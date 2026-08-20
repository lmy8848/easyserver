package runtimeenv

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"easyserver/internal/infra/errx"
	"easyserver/internal/infra/mise"
)

var (
	validPkgName    = regexp.MustCompile(`^(@[a-zA-Z0-9_.-]+/)?[a-zA-Z0-9_.][a-zA-Z0-9_.-]*$`)
	validPkgVersion = regexp.MustCompile(`^[a-zA-Z0-9_.+^~>=<*][a-zA-Z0-9_.\-+^~>=<*]*$`)
)

func validatePackageName(name string) error {
	if !validPkgName.MatchString(name) {
		return errx.BadRequest("invalid package name: %s", name)
	}
	return nil
}

func validatePackageVersion(version string) error {
	if version != "" && !validPkgVersion.MatchString(version) {
		return errx.BadRequest("invalid package version: %s", version)
	}
	return nil
}

func isAllowedManager(name string) bool {
	switch name {
	case "npm", "pnpm", "pip", "corepack":
		return true
	default:
		return false
	}
}

// PackageService manages packages installed under a runtime environment.
// Package state is sourced live from the underlying package manager (npm/pip/...);
// there is no DB cache.
type PackageService struct {
	provider mise.Provider
}

func NewPackageService(provider mise.Provider) *PackageService {
	return &PackageService{provider: provider}
}

// runCombinedEnv 执行命令并把 env 注入进程环境，返回合并输出。
// pnpm 需要 PNPM_HOME/PATH 注入（`pnpm setup` 写 ~/.bashrc，而 server 进程不
// source 它），所以这两处不走 executor.RunCombined 而直接 exec.CommandContext。
// 非零退出返回 *exec.ExitError，与 executor 语义一致。
func runCombinedEnv(ctx context.Context, env []string, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// runManagerCmd 运行包管理器命令。非零退出与启动失败均以 err 形式返回
// （os/exec 语义），install/uninstall/update 直接透传。
// runtimePath 是安装版本目录（installs/<tool>/<version>）：npm/pip 用其 bin/ 下
// 自带的可执行文件，而非系统 PATH 的同名工具；pnpm 走全局 PNPM_HOME；corepack
// 等系统工具传空 runtimePath 走 PATH。
func (s *PackageService) runManagerCmd(ctx context.Context, runtimePath, name string, args ...string) (string, error) {
	if !isAllowedManager(name) {
		return "", fmt.Errorf("unsupported package manager: %s", name)
	}
	if name == "pnpm" {
		return runCombinedEnv(ctx, pnpmEnv(), "pnpm", args...)
	}
	bin := name
	if runtimePath != "" {
		bin = managerBin(runtimePath, name)
	}
	output, err := exec.CommandContext(ctx, bin, args...).CombinedOutput()
	return string(output), err
}

// managerBin 返回安装版本内对应包管理器的可执行路径。
// node → <install>/bin/npm，python → <install>/bin/pip。让全局包操作走该版本
// 自带的包管理器，而非系统 PATH 的同名工具。
func managerBin(runtimePath, tool string) string {
	return filepath.Join(runtimePath, "bin", tool)
}

// pnpmEnv 返回 pnpm 全局安装所需的 PNPM_HOME 与 PATH 注入项。
// 同时覆盖新旧版本 pnpm 的 bin 目录约定（PNPM_HOME 与 PNPM_HOME/bin）。
func pnpmEnv() []string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		home = "/root"
	}
	pnpmHome := filepath.Join(home, ".local", "share", "pnpm")
	pnpmBin := filepath.Join(pnpmHome, "bin")
	sep := string(os.PathListSeparator)
	return []string{
		"PNPM_HOME=" + pnpmHome,
		"PATH=" + pnpmBin + sep + pnpmHome + sep + os.Getenv("PATH"),
	}
}

// describeCmdErr 返回给上游 error 用：包管理器出错时只截取输出末尾 10 行，
// 并把完整输出 log 到服务端便于调试。output 非空时用 output（含 stderr），
// 否则用 err.Error()（如 "executable file not found in $PATH"）。
//
// logTag 用于标识来源（如 "pip install" / "npm install"），方便日志检索。
//
// 上百行的 pip/npm 错误若整段塞进 error 会被 handler 当成 500 消息原样
// 回前端，既噪音又泄露环境信息。
func describeCmdErr(err error, output, logTag string) string {
	if strings.TrimSpace(output) != "" {
		log.Printf("package: %s full output:\n%s", logTag, output)
		return tailLines(output, 10)
	}
	if err != nil {
		return err.Error()
	}
	return ""
}

// tailLines 返回 s 末尾最多 n 行非空行，用 "\n" 拼接。
func tailLines(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}

// miseReshim 让 mise 重新生成 shims。
// 通过 corepack enable 或 npm install -g 装上的可执行文件，需要 reshim 才能
// 出现在 /opt/easyserver/mise/shims/ 下被 server 进程的 PATH 找到。
// 失败不阻断流程——如果 shim 仍能从 node bin 目录直接定位也算 OK。
//
// ponytail: 全局包管理是 mise 执行模型产物（reshim 不入通用 Provider 接口，
// 换容器/nix 底层时此模块随之更换），此处直接调用 mise 二进制是具体实现的
// 已知例外。
func (s *PackageService) miseReshim(ctx context.Context) {
	output, err := exec.CommandContext(ctx, filepath.Join(mise.DataDir, "mise"), "reshim").CombinedOutput()
	if err != nil {
		log.Printf("package: mise reshim failed (continuing): err=%v, output=%s", err, output)
	}
}

// ListPackages returns installed packages for a runtime by scanning the system
// package manager directly. There is no DB cache — the package manager itself
// is the source of truth.
func (s *PackageService) ListPackages(ctx context.Context, runtimeName, runtimePath string) ([]Package, error) {
	switch runtimeName {
	case "node":
		return s.scanNodePackages(ctx, runtimePath)
	case "python":
		return s.scanPipPackages(ctx, runtimePath)
	case "php":
		return s.scanComposerPackages(ctx, runtimePath)
	default:
		return []Package{}, nil
	}
}

// InstallPackage installs a package
func (s *PackageService) InstallPackage(ctx context.Context, req *PackageInstallRequest, runtimeName, runtimePath string) error {
	if err := validatePackageName(req.Name); err != nil {
		return err
	}
	if err := validatePackageVersion(req.Version); err != nil {
		return err
	}
	switch runtimeName {
	case "node":
		return s.installNpmPackage(ctx, req, runtimePath)
	case "python":
		return s.installPipPackage(ctx, req, runtimePath)
	case "php":
		return s.installComposerPackage(ctx, req, runtimePath)
	default:
		return fmt.Errorf("package management not supported for %s", runtimeName)
	}
}

// UninstallPackage uninstalls a package
func (s *PackageService) UninstallPackage(ctx context.Context, req *PackageUninstallRequest, runtimeName, runtimePath string) error {
	if err := validatePackageName(req.Name); err != nil {
		return err
	}
	switch runtimeName {
	case "node":
		return s.uninstallNpmPackage(ctx, req, runtimePath)
	case "python":
		return s.uninstallPipPackage(ctx, req, runtimePath)
	case "php":
		return s.uninstallComposerPackage(ctx, req, runtimePath)
	default:
		return fmt.Errorf("package management not supported for %s", runtimeName)
	}
}

// UpdatePackage updates a package
func (s *PackageService) UpdatePackage(ctx context.Context, req *PackageUpdateRequest, runtimeName, runtimePath string) error {
	if err := validatePackageName(req.Name); err != nil {
		return err
	}
	switch runtimeName {
	case "node":
		return s.updateNpmPackage(ctx, req, runtimePath)
	case "python":
		return s.updatePipPackage(ctx, req, runtimePath)
	default:
		return fmt.Errorf("package update not supported for %s", runtimeName)
	}
}

// SearchPackages searches for available packages
func (s *PackageService) SearchPackages(ctx context.Context, runtimeName, query string) ([]PackageInfo, error) {
	switch runtimeName {
	case "node":
		return s.searchNpmPackages(ctx, query)
	case "python":
		return s.searchPipPackages(ctx, query)
	default:
		return []PackageInfo{}, nil
	}
}

// GetPackageVersions returns available versions for a package
func (s *PackageService) GetPackageVersions(ctx context.Context, runtimeName, packageName string) ([]string, error) {
	switch runtimeName {
	case "node":
		return s.getNpmPackageVersions(ctx, packageName)
	case "python":
		return s.getPipPackageVersions(ctx, packageName)
	default:
		return []string{}, nil
	}
}

// GetRegistry returns the current registry URL for a package manager
func (s *PackageService) GetRegistry(ctx context.Context, runtimeName, runtimePath, manager string) (string, error) {
	switch runtimeName {
	case "node":
		if manager == "" {
			manager = "npm"
		}
		if manager != "npm" && manager != "pnpm" {
			return "", fmt.Errorf("invalid package manager: %s", manager)
		}
		output, err := s.runManagerCmd(ctx, runtimePath, manager, "config", "get", "registry")
		if err != nil {
			return "", fmt.Errorf("%s config get registry failed: %w", manager, err)
		}
		return strings.TrimSpace(output), nil
	case "python":
		output, err := s.runManagerCmd(ctx, runtimePath, "pip", "config", "get", "global.index-url")
		if err != nil {
			// pip config get returns error if not set
			return "", nil //nolint:nilerr // pip 未配置 index-url 时返回空串
		}
		return strings.TrimSpace(output), nil
	default:
		return "", fmt.Errorf("registry configuration not supported for %s", runtimeName)
	}
}

// SetRegistry sets the registry URL for a package manager
func (s *PackageService) SetRegistry(ctx context.Context, runtimeName, runtimePath, manager, registry string) error {
	switch runtimeName {
	case "node":
		if manager == "" {
			manager = "npm"
		}
		if manager != "npm" && manager != "pnpm" {
			return fmt.Errorf("invalid package manager: %s", manager)
		}
		var output string
		var err error
		if registry == "" {
			output, err = s.runManagerCmd(ctx, runtimePath, manager, "config", "delete", "registry")
		} else {
			output, err = s.runManagerCmd(ctx, runtimePath, manager, "config", "set", "registry", registry)
		}
		if err != nil {
			return fmt.Errorf("%s config set registry failed: %s", manager, describeCmdErr(err, output, manager+" config set registry"))
		}
		return nil
	case "python":
		var output string
		var err error
		if registry == "" {
			output, err = s.runManagerCmd(ctx, runtimePath, "pip", "config", "unset", "global.index-url")
		} else {
			output, err = s.runManagerCmd(ctx, runtimePath, "pip", "config", "set", "global.index-url", registry)
		}
		if err != nil {
			// Ignore unset errors if it was not set
			if registry == "" && strings.Contains(err.Error(), "exit code 1") {
				return nil
			}
			return errx.BadRequest("pip config set failed: %s", describeCmdErr(err, output, "pip config set global.index-url"))
		}
		return nil
	default:
		return fmt.Errorf("registry configuration not supported for %s", runtimeName)
	}
}

// npm package search
func (s *PackageService) searchNpmPackages(ctx context.Context, query string) ([]PackageInfo, error) {
	output, err := exec.CommandContext(ctx, "npm", "search", query, "--json").Output()
	if err != nil {
		log.Printf("package: npm search error: %v", err)
		return []PackageInfo{}, nil
	}

	var result []struct {
		Name        string `json:"name"`
		Version     string `json:"version"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(output, &result); err != nil {
		return nil, err
	}

	var packages []PackageInfo
	for _, pkg := range result {
		packages = append(packages, PackageInfo{
			Name:        pkg.Name,
			Version:     pkg.Version,
			Description: pkg.Description,
			Source:      "npm",
		})
	}

	return packages, nil
}

// npm package versions
func (s *PackageService) getNpmPackageVersions(ctx context.Context, packageName string) ([]string, error) {
	// 失败时 npm 仍会将结构化错误 JSON 写到 stdout（exit code 非零），
	// 因此忽略 err、统一按 stdout 内容分类处理。
	output, _ := exec.CommandContext(ctx, "npm", "view", packageName, "versions", "--json").Output()

	trimmed := strings.TrimSpace(string(output))
	if trimmed == "" {
		return []string{}, nil
	}

	// 1) 正常情况：版本数组
	var versions []string
	if err := json.Unmarshal([]byte(trimmed), &versions); err == nil {
		if len(versions) > 20 {
			versions = versions[len(versions)-20:]
		}
		for i, j := 0, len(versions)-1; i < j; i, j = i+1, j-1 {
			versions[i], versions[j] = versions[j], versions[i]
		}
		return versions, nil
	}

	// 2) 单版本：字符串
	var single string
	if err := json.Unmarshal([]byte(trimmed), &single); err == nil {
		return []string{single}, nil
	}

	// 3) 错误对象，如 {"error":{"code":"E404","summary":"Not Found ..."}}。
	//    这种情况一般是包名拼错或不存在，返回空列表给前端显示"无版本"即可，不抛 500。
	var errObj struct {
		Error struct {
			Code    string `json:"code"`
			Summary string `json:"summary"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(trimmed), &errObj); err == nil && errObj.Error.Code != "" {
		log.Printf("package: npm view %s returned error: %s %s", packageName, errObj.Error.Code, errObj.Error.Summary)
		return []string{}, nil
	}

	return []string{}, errors.New("无法解析 npm view 输出")
}

// pip package search
//
// PyPI 已于 2021 年关闭 `pip search` 接口；继续返回错误会让前端的实时搜索
// 下拉框 500，而非简单的"无建议"。这里改为返回空列表，与 maven/composer
// 等"不支持搜索"的语言行为一致——用户直接输入完整包名即可，pip 工作流
// 本来也是先知道名字再 install。
func (s *PackageService) searchPipPackages(ctx context.Context, query string) ([]PackageInfo, error) {
	return []PackageInfo{}, nil
}

// pip package versions
func (s *PackageService) getPipPackageVersions(ctx context.Context, packageName string) ([]string, error) {
	output, err := exec.CommandContext(ctx, "pip", "index", "versions", packageName).Output()
	if err != nil {
		log.Printf("package: pip index error: %v", err)
		return []string{}, nil
	}

	outputStr := output
	start := strings.Index(string(outputStr), "(")
	end := strings.Index(string(outputStr), ")")
	if start == -1 || end == -1 {
		return []string{}, nil
	}

	versionsStr := outputStr[start+1 : end]
	versions := strings.Split(string(versionsStr), ", ")

	if len(versions) > 20 {
		versions = versions[:20]
	}

	// Reverse the array so latest versions are first
	for i, j := 0, len(versions)-1; i < j; i, j = i+1, j-1 {
		versions[i], versions[j] = versions[j], versions[i]
	}

	return versions, nil
}

// scanNodePackages 合并扫描 npm 与 pnpm 全局包。
// node runtime 下两种包管理器装的包放在不同位置（npm prefix vs PNPM_HOME），
// 必须各自查询再合并，否则 pnpm 装的包在 npm list 里看不到。
func (s *PackageService) scanNodePackages(ctx context.Context, runtimePath string) ([]Package, error) {
	packages, _ := s.scanNpmPackages(ctx, runtimePath)

	if _, err := exec.LookPath("pnpm"); err == nil {
		pnpmPkgs, _ := s.scanPnpmPackages(ctx)
		packages = append(packages, pnpmPkgs...)
	}

	return packages, nil
}

// npm package management
func (s *PackageService) scanNpmPackages(ctx context.Context, runtimePath string) ([]Package, error) {
	output, err := exec.CommandContext(ctx, managerBin(runtimePath, "npm"), "list", "-g", "--json").Output()
	if err != nil {
		log.Printf("package: npm list error: %v", err)
		return []Package{}, nil
	}

	var result struct {
		Dependencies map[string]struct {
			Version string `json:"version"`
		} `json:"dependencies"`
	}
	if err := json.Unmarshal(output, &result); err != nil {
		return nil, err
	}

	var packages []Package
	for name, dep := range result.Dependencies {
		packages = append(packages, Package{
			Name:    name,
			Version: dep.Version,
			Scope:   "global",
			Source:  "npm",
		})
	}

	return packages, nil
}

// scanPnpmPackages 扫描 pnpm 全局包。
// pnpm list -g --json 输出结构与 npm 不同：
//
//	[{ "path": "...", "dependencies": { "<pkg>": { "version": "x.y.z" } } }]
func (s *PackageService) scanPnpmPackages(ctx context.Context) ([]Package, error) {
	// 注入 PNPM_HOME，确保 list 看到的全局目录与 install 时一致。
	output, err := runCombinedEnv(ctx, pnpmEnv(), "pnpm", "list", "-g", "--json")
	if err != nil {
		log.Printf("package: pnpm list error: %v", err)
		return []Package{}, nil
	}

	var result []struct {
		Dependencies map[string]struct {
			Version string `json:"version"`
		} `json:"dependencies"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &result); err != nil {
		log.Printf("package: pnpm list json parse error: %v, output=%s", err, output)
		return []Package{}, nil
	}

	var packages []Package
	for _, entry := range result {
		for name, dep := range entry.Dependencies {
			packages = append(packages, Package{
				Name:    name,
				Version: dep.Version,
				Scope:   "global",
				Source:  "pnpm",
			})
		}
	}

	return packages, nil
}

func (s *PackageService) installNpmPackage(ctx context.Context, req *PackageInstallRequest, runtimePath string) error {
	manager := req.Manager
	if manager == "" {
		manager = "npm"
	}
	if manager != "npm" && manager != "pnpm" {
		return fmt.Errorf("invalid package manager: %s", manager)
	}

	args := []string{}
	switch manager {
	case "pnpm":
		args = append(args, "add", "-g")
	default:
		args = append(args, "install", "-g")
	}

	if manager == "pnpm" {
		if _, lookErr := exec.LookPath(manager); lookErr != nil {
			log.Printf("package: %s not found, attempting to enable via corepack", manager)
			// corepack 在 mise 环境下偶尔会 exit 0 却不真正生成 shim（静默失败），
			// 因此 enable 之后跑 mise reshim 让新 shim 出现，再用 LookPath 二次校验。
			corepackOutput, corepackErr := s.runManagerCmd(ctx, "", "corepack", "enable", manager)
			s.miseReshim(ctx)
			if _, lookErr2 := exec.LookPath(manager); lookErr2 != nil {
				log.Printf("package: corepack did not produce a working %s shim (corepack: err=%v, output=%q), falling back to npm install -g", manager, corepackErr, corepackOutput)
				installOutput, installErr := s.runManagerCmd(ctx, "", "npm", "install", "-g", manager)
				if installErr != nil {
					return errx.BadRequest("failed to auto-install %s: %w (output: %s)", manager, installErr, installOutput)
				}
				s.miseReshim(ctx)
			}
			// pnpm 需要先 `pnpm setup` 才能进行全局安装（PNPM_HOME / 全局 bin 目录初始化）。
			// 失败不阻断流程——后续 pnpm 调用会显式注入 PNPM_HOME 兜底。
			setupOutput, setupErr := exec.CommandContext(ctx, "pnpm", "setup").CombinedOutput()
			if setupErr != nil {
				log.Printf("package: pnpm setup failed (continuing): err=%v, output=%s", setupErr, setupOutput)
			}
		}
	}

	if req.Version != "" {
		args = append(args, fmt.Sprintf("%s@%s", req.Name, req.Version))
	} else {
		args = append(args, req.Name)
	}

	output, err := s.runManagerCmd(ctx, runtimePath, manager, args...)
	if err != nil {
		return errx.BadRequest("%s install failed: %s", manager, describeCmdErr(err, output, manager+" install "+req.Name))
	}

	log.Printf("package: installed via %s %s", manager, strings.Join(args, " "))
	return nil
}

func (s *PackageService) uninstallNpmPackage(ctx context.Context, req *PackageUninstallRequest, runtimePath string) error {
	manager := req.Manager
	if manager == "" {
		manager = "npm"
	}
	if manager != "npm" && manager != "pnpm" {
		return fmt.Errorf("invalid package manager: %s", manager)
	}

	args := []string{}
	switch manager {
	case "pnpm":
		args = append(args, "remove", "-g")
	default:
		args = append(args, "uninstall", "-g")
	}
	args = append(args, req.Name)

	output, err := s.runManagerCmd(ctx, runtimePath, manager, args...)
	if err != nil {
		return fmt.Errorf("%s uninstall failed: %s", manager, describeCmdErr(err, output, manager+" uninstall "+req.Name))
	}

	log.Printf("package: uninstalled %s via %s", req.Name, manager)
	return nil
}

func (s *PackageService) updateNpmPackage(ctx context.Context, req *PackageUpdateRequest, runtimePath string) error {
	manager := req.Manager
	if manager == "" {
		manager = "npm"
	}
	if manager != "npm" && manager != "pnpm" {
		return fmt.Errorf("invalid package manager: %s", manager)
	}

	// npm 与 pnpm 都用 `update -g <name>`
	args := []string{"update", "-g", req.Name}

	output, err := s.runManagerCmd(ctx, runtimePath, manager, args...)
	if err != nil {
		return fmt.Errorf("%s update failed: %s", manager, describeCmdErr(err, output, manager+" update "+req.Name))
	}

	log.Printf("package: updated %s via %s", req.Name, manager)
	return nil
}

// pip package management
func (s *PackageService) scanPipPackages(ctx context.Context, runtimePath string) ([]Package, error) {
	output, err := exec.CommandContext(ctx, managerBin(runtimePath, "pip"), "list", "--format=json").Output()
	if err != nil {
		log.Printf("package: pip list error: %v", err)
		return []Package{}, nil
	}

	var result []struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	}
	if err := json.Unmarshal(output, &result); err != nil {
		return nil, err
	}

	var packages []Package
	for _, pkg := range result {
		packages = append(packages, Package{
			Name:    pkg.Name,
			Version: pkg.Version,
			Scope:   "global",
			Source:  "pip",
		})
	}

	return packages, nil
}

func (s *PackageService) installPipPackage(ctx context.Context, req *PackageInstallRequest, runtimePath string) error {
	args := []string{"install"}
	if req.Version != "" {
		args = append(args, fmt.Sprintf("%s==%s", req.Name, req.Version))
	} else {
		args = append(args, req.Name)
	}

	output, err := s.runManagerCmd(ctx, runtimePath, "pip", args...)
	if err != nil {
		return errx.BadRequest("pip install failed: %s", describeCmdErr(err, output, "pip install "+req.Name))
	}

	log.Printf("package: installed %s via pip", req.Name)
	return nil
}

func (s *PackageService) uninstallPipPackage(ctx context.Context, req *PackageUninstallRequest, runtimePath string) error {
	output, err := s.runManagerCmd(ctx, runtimePath, "pip", "uninstall", "-y", req.Name)
	if err != nil {
		return errx.BadRequest("pip uninstall failed: %s", describeCmdErr(err, output, "pip uninstall "+req.Name))
	}

	log.Printf("package: uninstalled %s via pip", req.Name)
	return nil
}

func (s *PackageService) updatePipPackage(ctx context.Context, req *PackageUpdateRequest, runtimePath string) error {
	output, err := s.runManagerCmd(ctx, runtimePath, "pip", "install", "--upgrade", req.Name)
	if err != nil {
		return errx.BadRequest("pip update failed: %s", describeCmdErr(err, output, "pip update "+req.Name))
	}

	log.Printf("package: updated %s via pip", req.Name)
	return nil
}

// composer package management (placeholder)
func (s *PackageService) scanComposerPackages(ctx context.Context, runtimePath string) ([]Package, error) {
	return []Package{}, nil
}

func (s *PackageService) installComposerPackage(ctx context.Context, req *PackageInstallRequest, runtimePath string) error {
	return errors.New("composer package installation not yet supported")
}

func (s *PackageService) uninstallComposerPackage(ctx context.Context, req *PackageUninstallRequest, runtimePath string) error {
	return errors.New("composer package uninstallation not yet supported")
}
