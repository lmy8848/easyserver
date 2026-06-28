package runtimeenv

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"easyserver/internal/infra/executor"
)

// PackageService manages packages installed under a runtime environment.
// Package state is sourced live from the underlying package manager (npm/pip/...);
// there is no DB cache.
type PackageService struct {
	executor executor.CommandExecutor
}

func NewPackageService(exec executor.CommandExecutor) *PackageService {
	return &PackageService{executor: exec}
}

// runManagerCmd 运行包管理器命令并按"非零 exit = 失败"的通用约定处理结果。
// 注意：底层 executor.RunCombined 把非零 exit 当作 err=nil + exitCode>0 返回
// （为兼容 firewall 等依赖该语义的模块），因此 install/uninstall/update 等
// "成功才算成功"的场景必须显式检查 exitCode。
//
// 针对 pnpm 还要显式注入 PNPM_HOME 与全局 bin 路径：`pnpm setup` 会把这两个
// 写入 ~/.bashrc，但 server 进程（尤其 systemd 启动时）不 source bashrc，
// 导致 `pnpm add -g` 校验 PATH 失败。这里把环境对齐到 SSH 登录后的状态。
func (s *PackageService) runManagerCmd(ctx context.Context, name string, args ...string) (string, error) {
	var output string
	var exitCode int
	var err error

	if name == "pnpm" {
		opts := executor.CommandOptions{Env: pnpmEnv()}
		output, exitCode, err = s.executor.RunWithOptions(ctx, opts, name, args...)
	} else {
		output, exitCode, err = s.executor.RunCombined(ctx, name, args...)
	}

	if err != nil {
		return output, err
	}
	if exitCode != 0 {
		return output, fmt.Errorf("exit code %d", exitCode)
	}
	return output, nil
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

// describeCmdErr 选择最有信息量的错误描述：output 非空用 output（含 stderr），
// 否则用 err.Error()（如 "executable file not found in $PATH"）。
func describeCmdErr(err error, output string) string {
	if strings.TrimSpace(output) != "" {
		return output
	}
	if err != nil {
		return err.Error()
	}
	return ""
}

// miseReshim 让 mise 重新生成 shims。
// 通过 corepack enable 或 npm install -g 装上的可执行文件，需要 reshim 才能
// 出现在 /var/lib/easyserver/mise/shims/ 下被 server 进程的 PATH 找到。
// 失败不阻断流程——如果 shim 仍能从 node bin 目录直接定位也算 OK。
func (s *PackageService) miseReshim(ctx context.Context) {
	output, _, err := s.executor.RunCombined(ctx, "mise", "reshim")
	if err != nil {
		log.Printf("package: mise reshim failed (continuing): err=%v, output=%s", err, output)
	}
}

// ListPackages returns installed packages for a runtime by scanning the system
// package manager directly. There is no DB cache — the package manager itself
// is the source of truth.
func (s *PackageService) ListPackages(ctx context.Context, runtimeID int64, runtimeName, runtimePath string) ([]Package, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	switch runtimeName {
	case "node":
		return s.scanNodePackages(ctx, runtimeID, runtimePath)
	case "python":
		return s.scanPipPackages(ctx, runtimeID, runtimePath)
	case "java":
		return s.scanMavenPackages(ctx, runtimeID, runtimePath)
	case "php":
		return s.scanComposerPackages(ctx, runtimeID, runtimePath)
	default:
		return []Package{}, nil
	}
}

// InstallPackage installs a package
func (s *PackageService) InstallPackage(ctx context.Context, req *PackageInstallRequest, runtimeName, runtimePath string) error {
	switch runtimeName {
	case "node":
		return s.installNpmPackage(ctx, req, runtimePath)
	case "python":
		return s.installPipPackage(ctx, req, runtimePath)
	case "java":
		return s.installMavenPackage(ctx, req, runtimePath)
	case "php":
		return s.installComposerPackage(ctx, req, runtimePath)
	default:
		return fmt.Errorf("package management not supported for %s", runtimeName)
	}
}

// UninstallPackage uninstalls a package
func (s *PackageService) UninstallPackage(ctx context.Context, req *PackageUninstallRequest, runtimeName, runtimePath string) error {
	switch runtimeName {
	case "node":
		return s.uninstallNpmPackage(ctx, req, runtimePath)
	case "python":
		return s.uninstallPipPackage(ctx, req, runtimePath)
	case "java":
		return s.uninstallMavenPackage(ctx, req, runtimePath)
	case "php":
		return s.uninstallComposerPackage(ctx, req, runtimePath)
	default:
		return fmt.Errorf("package management not supported for %s", runtimeName)
	}
}

// UpdatePackage updates a package
func (s *PackageService) UpdatePackage(ctx context.Context, req *PackageUpdateRequest, runtimeName, runtimePath string) error {
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

// npm package search
func (s *PackageService) searchNpmPackages(ctx context.Context, query string) ([]PackageInfo, error) {
	output, _, _, err := s.executor.Run(ctx, "npm", "search", query, "--json")
	if err != nil {
		log.Printf("package: npm search error: %v", err)
		return []PackageInfo{}, nil
	}

	var result []struct {
		Name        string `json:"name"`
		Version     string `json:"version"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal([]byte(output), &result); err != nil {
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
	output, _, _, _ := s.executor.Run(ctx, "npm", "view", packageName, "versions", "--json")

	trimmed := strings.TrimSpace(output)
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

	return []string{}, fmt.Errorf("无法解析 npm view 输出")
}

// pip package search
func (s *PackageService) searchPipPackages(ctx context.Context, query string) ([]PackageInfo, error) {
	return []PackageInfo{}, fmt.Errorf("pip search not supported, use pip install <package>")
}

// pip package versions
func (s *PackageService) getPipPackageVersions(ctx context.Context, packageName string) ([]string, error) {
	output, _, _, err := s.executor.Run(ctx, "pip", "index", "versions", packageName)
	if err != nil {
		log.Printf("package: pip index error: %v", err)
		return []string{}, nil
	}

	outputStr := output
	start := strings.Index(outputStr, "(")
	end := strings.Index(outputStr, ")")
	if start == -1 || end == -1 {
		return []string{}, nil
	}

	versionsStr := outputStr[start+1 : end]
	versions := strings.Split(versionsStr, ", ")

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
func (s *PackageService) scanNodePackages(ctx context.Context, runtimeID int64, runtimePath string) ([]Package, error) {
	packages, _ := s.scanNpmPackages(ctx, runtimeID, runtimePath)

	if _, err := s.executor.LookPath("pnpm"); err == nil {
		pnpmPkgs, _ := s.scanPnpmPackages(ctx, runtimeID)
		packages = append(packages, pnpmPkgs...)
	}

	return packages, nil
}

// npm package management
func (s *PackageService) scanNpmPackages(ctx context.Context, runtimeID int64, runtimePath string) ([]Package, error) {
	output, _, _, err := s.executor.Run(ctx, "npm", "list", "-g", "--json")
	if err != nil {
		log.Printf("package: npm list error: %v", err)
		return []Package{}, nil
	}

	var result struct {
		Dependencies map[string]struct {
			Version string `json:"version"`
		} `json:"dependencies"`
	}
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		return nil, err
	}

	var packages []Package
	for name, dep := range result.Dependencies {
		packages = append(packages, Package{
			RuntimeID: runtimeID,
			Name:      name,
			Version:   dep.Version,
			Scope:     "global",
			Source:    "npm",
		})
	}

	return packages, nil
}

// scanPnpmPackages 扫描 pnpm 全局包。
// pnpm list -g --json 输出结构与 npm 不同：
//
//	[{ "path": "...", "dependencies": { "<pkg>": { "version": "x.y.z" } } }]
func (s *PackageService) scanPnpmPackages(ctx context.Context, runtimeID int64) ([]Package, error) {
	// 注入 PNPM_HOME，确保 list 看到的全局目录与 install 时一致。
	opts := executor.CommandOptions{Env: pnpmEnv()}
	output, _, err := s.executor.RunWithOptions(ctx, opts, "pnpm", "list", "-g", "--json")
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
				RuntimeID: runtimeID,
				Name:      name,
				Version:   dep.Version,
				Scope:     "global",
				Source:    "pnpm",
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

	args := []string{}
	switch manager {
	case "pnpm":
		args = append(args, "add", "-g")
	default:
		args = append(args, "install", "-g")
	}

	if manager == "pnpm" {
		if _, lookErr := s.executor.LookPath(manager); lookErr != nil {
			log.Printf("package: %s not found, attempting to enable via corepack", manager)
			// corepack 在 mise 环境下偶尔会 exit 0 却不真正生成 shim（静默失败），
			// 因此 enable 之后跑 mise reshim 让新 shim 出现，再用 LookPath 二次校验。
			corepackOutput, corepackErr := s.runManagerCmd(ctx, "corepack", "enable", manager)
			s.miseReshim(ctx)
			if _, lookErr2 := s.executor.LookPath(manager); lookErr2 != nil {
				log.Printf("package: corepack did not produce a working %s shim (corepack: err=%v, output=%q), falling back to npm install -g", manager, corepackErr, corepackOutput)
				installOutput, installErr := s.runManagerCmd(ctx, "npm", "install", "-g", manager)
				if installErr != nil {
					return fmt.Errorf("failed to auto-install %s: %v (output: %s)", manager, installErr, installOutput)
				}
				s.miseReshim(ctx)
			}
			// pnpm 需要先 `pnpm setup` 才能进行全局安装（PNPM_HOME / 全局 bin 目录初始化）。
			// 失败不阻断流程——后续 pnpm 调用会显式注入 PNPM_HOME 兜底。
			setupOutput, _, setupErr := s.executor.RunCombined(ctx, "pnpm", "setup")
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

	output, err := s.runManagerCmd(ctx, manager, args...)
	if err != nil {
		return fmt.Errorf("%s install failed: %s", manager, describeCmdErr(err, output))
	}

	log.Printf("package: installed via %s %s", manager, strings.Join(args, " "))
	return nil
}

func (s *PackageService) uninstallNpmPackage(ctx context.Context, req *PackageUninstallRequest, runtimePath string) error {
	manager := req.Manager
	if manager == "" {
		manager = "npm"
	}

	args := []string{}
	switch manager {
	case "pnpm":
		args = append(args, "remove", "-g")
	default:
		args = append(args, "uninstall", "-g")
	}
	args = append(args, req.Name)

	output, err := s.runManagerCmd(ctx, manager, args...)
	if err != nil {
		return fmt.Errorf("%s uninstall failed: %s", manager, describeCmdErr(err, output))
	}

	log.Printf("package: uninstalled %s via %s", req.Name, manager)
	return nil
}

func (s *PackageService) updateNpmPackage(ctx context.Context, req *PackageUpdateRequest, runtimePath string) error {
	manager := req.Manager
	if manager == "" {
		manager = "npm"
	}

	// npm 与 pnpm 都用 `update -g <name>`
	args := []string{"update", "-g", req.Name}

	output, err := s.runManagerCmd(ctx, manager, args...)
	if err != nil {
		return fmt.Errorf("%s update failed: %s", manager, describeCmdErr(err, output))
	}

	log.Printf("package: updated %s via %s", req.Name, manager)
	return nil
}

// pip package management
func (s *PackageService) scanPipPackages(ctx context.Context, runtimeID int64, runtimePath string) ([]Package, error) {
	output, _, _, err := s.executor.Run(ctx, "pip", "list", "--format=json")
	if err != nil {
		log.Printf("package: pip list error: %v", err)
		return []Package{}, nil
	}

	var result []struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	}
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		return nil, err
	}

	var packages []Package
	for _, pkg := range result {
		packages = append(packages, Package{
			RuntimeID: runtimeID,
			Name:      pkg.Name,
			Version:   pkg.Version,
			Scope:     "global",
			Source:    "pip",
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

	output, err := s.runManagerCmd(ctx, "pip", args...)
	if err != nil {
		return fmt.Errorf("pip install failed: %s", describeCmdErr(err, output))
	}

	log.Printf("package: installed %s via pip", req.Name)
	return nil
}

func (s *PackageService) uninstallPipPackage(ctx context.Context, req *PackageUninstallRequest, runtimePath string) error {
	output, err := s.runManagerCmd(ctx, "pip", "uninstall", "-y", req.Name)
	if err != nil {
		return fmt.Errorf("pip uninstall failed: %s", describeCmdErr(err, output))
	}

	log.Printf("package: uninstalled %s via pip", req.Name)
	return nil
}

func (s *PackageService) updatePipPackage(ctx context.Context, req *PackageUpdateRequest, runtimePath string) error {
	output, err := s.runManagerCmd(ctx, "pip", "install", "--upgrade", req.Name)
	if err != nil {
		return fmt.Errorf("pip update failed: %s", describeCmdErr(err, output))
	}

	log.Printf("package: updated %s via pip", req.Name)
	return nil
}

// maven package management (placeholder)
func (s *PackageService) scanMavenPackages(ctx context.Context, runtimeID int64, runtimePath string) ([]Package, error) {
	return []Package{}, nil
}

func (s *PackageService) installMavenPackage(ctx context.Context, req *PackageInstallRequest, runtimePath string) error {
	return fmt.Errorf("maven package installation not yet supported")
}

func (s *PackageService) uninstallMavenPackage(ctx context.Context, req *PackageUninstallRequest, runtimePath string) error {
	return fmt.Errorf("maven package uninstallation not yet supported")
}

// composer package management (placeholder)
func (s *PackageService) scanComposerPackages(ctx context.Context, runtimeID int64, runtimePath string) ([]Package, error) {
	return []Package{}, nil
}

func (s *PackageService) installComposerPackage(ctx context.Context, req *PackageInstallRequest, runtimePath string) error {
	return fmt.Errorf("composer package installation not yet supported")
}

func (s *PackageService) uninstallComposerPackage(ctx context.Context, req *PackageUninstallRequest, runtimePath string) error {
	return fmt.Errorf("composer package uninstallation not yet supported")
}
