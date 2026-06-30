package runtimeenv

import (
	"path/filepath"
	"strings"
)

type Runtime struct {
	Lang               string   `json:"lang"`              // "node"
	Display            string   `json:"display"`           // "Node.js"
	MiseTool           string   `json:"mise_tool"`         // mise 插件名
	Majors             []string `json:"majors"`            // UI 可选的主版本列表
	MirrorEnvs         []string `json:"mirror_envs"`       // 该语言会用到的 env key（MirrorPanel 据此过滤 EnvConfig）
	MirrorCandidates   []string `json:"mirror_candidates"` // 候选镜像地址（不含 mise 默认源），供 MirrorPanel 新增时选择
	SupportsGlobalPkgs bool     `json:"supports_global_pkgs"`
}

var catalog = []Runtime{
	{
		Lang:               "node",
		Display:            "Node.js",
		MiseTool:           "node",
		Majors:             []string{"16", "18", "20", "22", "24"},
		MirrorEnvs:         []string{"MISE_NODE_MIRROR_URL"},
		MirrorCandidates:   []string{"https://npmmirror.com/mirrors/node", "https://mirrors.tuna.tsinghua.edu.cn/nodejs-release"},
		SupportsGlobalPkgs: true,
	},
	{
		Lang:               "python",
		Display:            "Python",
		MiseTool:           "python",
		Majors:             []string{"3.10", "3.11", "3.12", "3.13", "3.14"},
		MirrorEnvs:         []string{"PYTHON_BUILD_MIRROR_URL"},
		MirrorCandidates:   []string{"https://npmmirror.com/mirrors/python", "https://mirrors.aliyun.com/python"},
		SupportsGlobalPkgs: true,
	},
	{
		Lang:               "go",
		Display:            "Go",
		MiseTool:           "go",
		Majors:             []string{"1.22", "1.23", "1.24", "1.25", "1.26"},
		MirrorEnvs:         []string{"MISE_GO_DOWNLOAD_MIRROR"},
		MirrorCandidates:   []string{"https://mirrors.aliyun.com/golang", "https://mirrors.ustc.edu.cn/golang"},
		SupportsGlobalPkgs: false,
	},
	{
		Lang:               "java",
		Display:            "Java",
		MiseTool:           "vfox:version-fox/vfox-java",
		Majors:             []string{"8", "11", "17", "21", "25"},
		MirrorEnvs:         []string{},
		MirrorCandidates:   []string{},
		SupportsGlobalPkgs: false,
	},
	{
		Lang:    "php",
		Display: "PHP",
		// `mise registry php` 给出两条后端：asdf:mise-plugins/asdf-php 与
		// vfox:mise-plugins/vfox-php，按顺序匹配。直接用短名 "php" 让 mise
		// 走 registry——asdf-php 通过 git clone + tags 拿版本（稳定），而
		// vfox-php 的 Available hook 走 GitHub API，被限流/受网络影响时
		// 直接返回空，表现为前端版本下拉为空。
		MiseTool:           "php",
		Majors:             []string{"8.1", "8.2", "8.3", "8.4", "8.5"},
		MirrorEnvs:         []string{},
		MirrorCandidates:   []string{},
		SupportsGlobalPkgs: true,
	},
}

var supportedMap map[string]struct{}

func init() {
	supportedMap = make(map[string]struct{}, len(catalog))
	for _, r := range catalog {
		supportedMap[r.Lang] = struct{}{}
	}
}

// IsSupported checks if a given language is supported by the catalog.
func IsSupported(lang string) bool {
	_, ok := supportedMap[strings.ToLower(lang)]
	return ok
}

// GetCatalog returns a deep copy of the full catalog of runtimes.
// We append onto []string{} (not nil) so empty slices serialize to JSON [],
// not null — the frontend reads `c.mirror_envs.length` and would crash on
// null otherwise.
func GetCatalog() []Runtime {
	c := make([]Runtime, len(catalog))
	for i, r := range catalog {
		c[i] = Runtime{
			Lang:               r.Lang,
			Display:            r.Display,
			MiseTool:           r.MiseTool,
			Majors:             append([]string{}, r.Majors...),
			MirrorEnvs:         append([]string{}, r.MirrorEnvs...),
			MirrorCandidates:   append([]string{}, r.MirrorCandidates...),
			SupportsGlobalPkgs: r.SupportsGlobalPkgs,
		}
	}
	return c
}

// MiseToolFor returns the mise tool identifier for a given language.
// e.g. "node" → "node", "java" → "vfox:version-fox/vfox-java".
// The second return value is false when the lang is not in the catalog.
func MiseToolFor(lang string) (string, bool) {
	lang = strings.ToLower(lang)
	for _, r := range catalog {
		if r.Lang == lang {
			return r.MiseTool, true
		}
	}
	return "", false
}

// miseToolDirName normalizes a mise tool key into the filesystem name mise
// uses under installs/.  mise replaces ':' and '/' with '-' (see
// .mise-installs.toml: e.g. "vfox:version-fox/vfox-java" →
// "vfox-version-fox-vfox-java"). Core plugins like "node"/"go" pass through.
func miseToolDirName(miseTool string) string {
	return strings.NewReplacer(":", "-", "/", "-").Replace(miseTool)
}

// miseInstallPath returns the on-disk install dir mise will use for
// (lang, version), or "" when lang isn't in the catalog or version is blank.
// Pure function — does not stat the path, so it returns the *intended*
// location even when status != installed.
func miseInstallPath(lang, version string) string {
	if version == "" {
		return ""
	}
	tool, ok := MiseToolFor(lang)
	if !ok {
		return ""
	}
	return filepath.Join(miseDataDir, "installs", miseToolDirName(tool), version)
}

// SupportsGlobalPkgsFor returns true if the specified language supports global package management via the panel.
func SupportsGlobalPkgsFor(lang string) bool {
	lang = strings.ToLower(lang)
	for _, r := range catalog {
		if r.Lang == lang {
			return r.SupportsGlobalPkgs
		}
	}
	return false
}
