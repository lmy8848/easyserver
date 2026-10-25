package security

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"easyserver/internal/infra/errx"
)

// Service provides security-audit operations (CVE scanning, kernel status,
// file integrity monitoring).
type Service struct {
	db       *sql.DB
	fimPaths []string
}

// NewService creates a security Service. configPath 是面板配置文件的实际路径
// （--config），FIM 用它替换默认监视列表中的硬编码项。
func NewService(db *sql.DB, configPath string) *Service {
	return &Service{db: db, fimPaths: defaultFIMPaths(configPath)}
}

// debianEcosystem 从 /etc/os-release 读取 VERSION_ID，返回如 "Debian:12"；
// 读取失败回退为 "Debian"（旧行为）。
var debianEcosystem = sync.OnceValue(func() string {
	data, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return "Debian"
	}
	for line := range strings.SplitSeq(string(data), "\n") {
		if v, ok := strings.CutPrefix(line, "VERSION_ID="); ok {
			return "Debian:" + strings.Trim(strings.TrimSpace(v), `"`)
		}
	}
	return "Debian"
})

// Vulnerability is one installed package with known CVEs.
// Upgradable 表示 apt 源中已有修复版本（Candidate 高于当前版本）；为 false
// 时该包的修复尚未进入当前 Debian 发行版源，点击升级不会有实际效果。
type Vulnerability struct {
	Package    string   `json:"package"`
	Version    string   `json:"version"`
	VulnIDs    []string `json:"vuln_ids"`
	Candidate  string   `json:"candidate,omitempty"`
	Upgradable bool     `json:"upgradable"`
}

type installedPackage struct {
	Name    string
	Version string
}

// Scan lists installed apt packages and queries osv.dev for known CVEs.
// Returns only packages that have at least one vulnerability.
func (s *Service) Scan(ctx context.Context) ([]Vulnerability, error) {
	pkgs, err := s.listInstalled(ctx)
	if err != nil {
		return nil, errx.Internal("读取已装包失败: %w", err)
	}
	var vulns []Vulnerability
	// osv.dev querybatch accepts up to 1000 queries per request.
	for i := 0; i < len(pkgs); i += 500 {
		end := min(i+500, len(pkgs))
		results, err := s.queryOSVBatch(ctx, pkgs[i:end])
		if err != nil {
			return nil, errx.Internal("查询 osv.dev 失败: %w", err)
		}
		for j, res := range results {
			if len(res.Vulns) == 0 {
				continue
			}
			ids := make([]string, 0, len(res.Vulns))
			for _, v := range res.Vulns {
				ids = append(ids, v.ID)
			}
			vulns = append(vulns, Vulnerability{
				Package: pkgs[i+j].Name,
				Version: pkgs[i+j].Version,
				VulnIDs: ids,
			})
		}
	}
	// 标注可修复状态：osv.dev 记录的是上游修复信息，修复是否已进入
	// 当前发行版源要问 apt，二者缺一不可，否则用户会点一个注定无效的升级
	names := make([]string, 0, len(vulns))
	for _, v := range vulns {
		names = append(names, v.Package)
	}
	installed, candidate := aptPolicy(ctx, names)
	for i := range vulns {
		vulns[i].Candidate = candidate[vulns[i].Package]
		vulns[i].Upgradable = vulns[i].Candidate != "" &&
			vulns[i].Candidate != "(none)" &&
			vulns[i].Candidate != installed[vulns[i].Package]
	}
	return vulns, nil
}

// aptPolicy 批量查询包的 Installed/Candidate 版本；查询失败时返回空表
// （调用方据此退化为"全部视为不可升级"，不会误标可修复）。
func aptPolicy(ctx context.Context, packages []string) (installed, candidate map[string]string) {
	installed = map[string]string{}
	candidate = map[string]string{}
	if len(packages) == 0 {
		return installed, candidate
	}
	// 可变参数槽只能整体由展开填充，不能混用显式参数：
	// CommandContext(ctx, "apt-cache", "policy", pkgs...) 是编译错误
	args := append([]string{"policy"}, packages...)
	out, err := exec.CommandContext(ctx, "apt-cache", args...).CombinedOutput()
	if err != nil {
		return installed, candidate
	}
	cur := ""
	for line := range strings.SplitSeq(string(out), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if !strings.HasPrefix(trimmed, " ") && strings.HasSuffix(trimmed, ":") {
			cur = strings.TrimSuffix(trimmed, ":")
			continue
		}
		if cur == "" {
			continue
		}
		if v, ok := strings.CutPrefix(trimmed, "Installed: "); ok {
			installed[cur] = strings.TrimSpace(v)
		} else if v, ok := strings.CutPrefix(trimmed, "Candidate: "); ok {
			candidate[cur] = strings.TrimSpace(v)
		}
	}
	return installed, candidate
}

// listInstalled parses `apt list --installed` output.
func (s *Service) listInstalled(ctx context.Context) ([]installedPackage, error) {
	out, err := exec.CommandContext(ctx, "apt", "list", "--installed").CombinedOutput()
	if err != nil {
		return nil, err
	}
	var pkgs []installedPackage
	for line := range strings.SplitSeq(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "Listing") {
			continue
		}
		// format: name/archive[,archive...] version arch [status]
		// 斜杠后第一段是归档标识（如 oldstable,now / stable,now），版本在其后。
		parts := strings.SplitN(line, "/", 2)
		if len(parts) < 2 {
			continue
		}
		name := parts[0]
		fields := strings.Fields(parts[1])
		if len(fields) < 2 {
			continue
		}
		pkgs = append(pkgs, installedPackage{Name: name, Version: fields[1]})
	}
	return pkgs, nil
}

type osvQuery struct {
	Package struct {
		Name      string `json:"name"`
		Ecosystem string `json:"ecosystem"`
	} `json:"package"`
	Version string `json:"version"`
}

type osvVuln struct {
	ID string `json:"id"`
}

type osvResult struct {
	Vulns []osvVuln `json:"vulns"`
}

type osvBatchResp struct {
	Results []osvResult `json:"results"`
}

// queryOSVBatch queries osv.dev for a batch of packages (Debian ecosystem).
func (s *Service) queryOSVBatch(ctx context.Context, pkgs []installedPackage) ([]osvResult, error) {
	queries := make([]osvQuery, len(pkgs))
	for i, p := range pkgs {
		queries[i].Package.Name = p.Name
		// 必须按本机实际发行版版本查询（Debian:12），裸 "Debian" 指向最新
		// 稳定版的数据——拿 13 的漏洞记录对照 12 的包会大量误报"有漏洞"
		// 且源里根本不存在修复版本
		queries[i].Package.Ecosystem = debianEcosystem()
		queries[i].Version = p.Version
	}
	body, _ := json.Marshal(map[string]any{"queries": queries})
	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.osv.dev/v1/querybatch", strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var r osvBatchResp
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, err
	}
	return r.Results, nil
}

// Upgrade runs apt-get install --only-upgrade for the given packages.
// Upgrade refreshes the apt index before upgrading, then installs the given
// packages with --only-upgrade. 返回值第二个 string 是面向用户的结果消息：
// apt 在索引过期时会以退出码 0 结束且什么都不升（"0 upgraded, 0 newly
// installed"），必须如实区分"升级成功"与"源里没有更新"。
// Upgrade 先用 apt-cache policy 把请求的包分成"源里有修复版本"与"暂无
// 修复"两类，只对前者执行升级；返回面向用户的消息、暂无修复包清单和
// 原始输出。此前对含无修复版本的整批包执行 --only-upgrade 会以
// "0 upgraded" 退出码 0 结束，前端却提示升级成功。
func (s *Service) Upgrade(ctx context.Context, packages []string) (string, []string, string, error) {
	if len(packages) == 0 {
		return "", nil, "", errx.BadRequest("未指定升级的包")
	}

	// apt 为分钟级操作：内部超时上限 5 分钟，防 debconf 交互/慢源把请求
	// 无限挂起（外层路由 WriteTimeout 10 分钟兜底，但挂满对用户体验是灾难）。
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	upgradable, noFix := classifyUpgradable(ctx, packages)
	if len(upgradable) == 0 {
		return "软件源中没有可用的更新版本（索引已刷新）", noFix, "", nil
	}

	// 非交互环境：防 debconf 等待 stdin 输入把升级卡死为僵尸请求。
	env := append(os.Environ(),
		"DEBIAN_FRONTEND=noninteractive",
		"APT_LISTCHANGES_FRONTEND=none",
	)

	updCmd := exec.CommandContext(ctx, "apt-get", "update", "-q") //nolint:gosec // 固定二进制 apt-get
	updCmd.Env = env
	if out, err := updCmd.CombinedOutput(); err != nil {
		return "", noFix, string(out), errx.Internal("刷新软件源失败: %w", err)
	}

	args := append([]string{"install", "--only-upgrade", "-y"}, upgradable...)
	cmd := exec.CommandContext(ctx, "apt-get", args...) //nolint:gosec // 包名经包管理器白名单处理
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", noFix, string(out), errx.Internal("升级失败: %w", err)
	}

	upgraded := 0
	if m := upgradeCountRe.FindStringSubmatch(string(out)); m != nil {
		upgraded, _ = strconv.Atoi(m[1])
	}
	var msg string
	switch {
	case len(noFix) == 0:
		msg = fmt.Sprintf("已升级 %d 个包", upgraded)
	case upgraded == 0:
		msg = "软件源中没有可用的更新版本（索引已刷新）"
	default:
		msg = fmt.Sprintf("已升级 %d 个包；%d 个包 Debian 源中暂无修复版本", upgraded, len(noFix))
	}
	return msg, noFix, string(out), nil
}

// upgradeCountRe 匹配 apt-get 汇总行的升级数量。
var upgradeCountRe = regexp.MustCompile(`(\d+) upgraded`)

// classifyUpgradable 按 apt-cache policy 区分有修复版本的包与暂无修复的
// 包；查询失败时退回旧行为（全部尝试升级）。
func classifyUpgradable(ctx context.Context, packages []string) (upgradable, noFix []string) {
	installed, candidate := aptPolicy(ctx, packages)
	if len(candidate) == 0 {
		return packages, nil
	}
	for _, p := range packages {
		c := candidate[p]
		if c != "" && c != "(none)" && c != installed[p] {
			upgradable = append(upgradable, p)
		} else {
			noFix = append(noFix, p)
		}
	}
	return upgradable, noFix
}

// KernelStatus reports the running kernel vs the latest installed kernel.
type KernelStatus struct {
	Current     string `json:"current"`
	Latest      string `json:"latest"`
	NeedsReboot bool   `json:"needs_reboot"`
}

// KernelStatus returns running kernel and latest installed kernel.
func (s *Service) KernelStatus(ctx context.Context) (*KernelStatus, error) {
	out, _ := exec.CommandContext(ctx, "uname", "-r").CombinedOutput()
	current := strings.TrimSpace(string(out))
	out, _ = exec.CommandContext(ctx, "bash", "-c", "dpkg -l 'linux-image-*' 2>/dev/null | awk '/^ii/{print $2}' | sed 's/linux-image-//' | sort -V | tail -1").CombinedOutput()
	latest := strings.TrimSpace(string(out))
	return &KernelStatus{
		Current:     current,
		Latest:      latest,
		NeedsReboot: current != "" && latest != "" && current != latest,
	}, nil
}

// PackageUpdateCount returns pending apt upgrades (apt list --upgradable).
func (s *Service) PackageUpdateCount(ctx context.Context) (int, error) {
	out, err := exec.CommandContext(ctx, "apt", "list", "--upgradable").CombinedOutput()
	if err != nil {
		return 0, fmt.Errorf("apt list --upgradable: %w", err)
	}
	count := 0
	for line := range strings.SplitSeq(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "Listing") {
			continue
		}
		count++
	}
	return count, nil
}
