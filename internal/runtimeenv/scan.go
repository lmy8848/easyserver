package runtimeenv

import (
	"context"
	"errors"
	"fmt"
	stdlog "log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"easyserver/internal/infra/mise"
)

// 完成标记：面板安装成功后在版本目录写入此文件，目录扫描只认带标记的安装。
// 防止面板崩溃在解压窗口时残留的半截目录被当成已安装（ADR-0009）。
// 手动 `mise install` 的版本不带标记，面板列表不显示（已知限制）。
const okMarker = ".easyserver-ok"

// markerPath 返回 (lang, exact) 的完成标记路径（纯计算，不 stat 磁盘）。
func markerPath(lang, exact string) string {
	return filepath.Join(mise.DataDir, "installs", miseToolDir(lang), exact, okMarker)
}

// miseToolDir 返回 lang 在 installs/ 下的目录名（与 provider 的 InstallPath 同构）。
// 计算失败（未知 lang）返回空串，调用方据此跳过。
func miseToolDir(lang string) string {
	return mise.InstallDir(lang)
}

// ScanInstalled 扫描 installs/ 目录，返回全部面板安装完成的 RuntimeEnvironment。
// 权威来源是文件系统（ADR-0009）：installs/<tool-dir>/<version> 存在且带完成标记
// 即视为已安装。排序：先 lang 后 version（语义版本）。
func ScanInstalled(ctx context.Context) ([]RuntimeEnvironment, error) {
	root := filepath.Join(mise.DataDir, "installs")
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read installs dir: %w", err)
	}

	// tool-dir → lang 反查表：只扫 catalog 支持的语言。
	langByTool := map[string]string{}
	for _, r := range catalog {
		langByTool[miseToolDir(r.Lang)] = r.Lang
	}

	var envs []RuntimeEnvironment
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		lang, ok := langByTool[e.Name()]
		if !ok {
			continue
		}
		versions, err := scanVersions(root, e.Name(), lang)
		if err != nil {
			stdlog.Printf("runtime: scan %s: %v", lang, err)
			continue
		}
		envs = append(envs, versions...)
	}

	sort.Slice(envs, func(i, j int) bool {
		if envs[i].Name != envs[j].Name {
			return envs[i].Name < envs[j].Name
		}
		return versionLess(envs[i].Version, envs[j].Version)
	})
	return envs, nil
}

// scanVersions 列出某 lang 目录下带完成标记的版本。
func scanVersions(root, toolDir, lang string) ([]RuntimeEnvironment, error) {
	dir := filepath.Join(root, toolDir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var envs []RuntimeEnvironment
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		version := e.Name()
		if _, err := os.Stat(filepath.Join(dir, version, okMarker)); err != nil {
			continue // 无标记：手动安装或崩溃残留
		}
		envs = append(envs, RuntimeEnvironment{
			Name:    lang,
			Version: version,
			Path:    filepath.Join(dir, version),
			Status:  "installed",
		})
	}
	return envs, nil
}

// versionLess 按语义版本号比较两个版本字符串（纯数字段比较，非字符串序）。
// "20.11.0" < "21.0.1"，"9" < "10"（区别于字典序）。
func versionLess(a, b string) bool {
	as := strings.Split(a, ".")
	bs := strings.Split(b, ".")
	for i := 0; i < len(as) && i < len(bs); i++ {
		ai, aerr := parseNumPrefix(as[i])
		bi, berr := parseNumPrefix(bs[i])
		switch {
		case aerr == nil && berr != nil:
			return true
		case aerr != nil && berr == nil:
			return false
		case aerr == nil && berr == nil && ai != bi:
			return ai < bi
		}
	}
	return len(as) < len(bs)
}

// parseNumPrefix 解析段开头的数字；非数字开头的段（如 beta）返回 0 + err。
func parseNumPrefix(s string) (int, error) {
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	if i == 0 {
		return 0, errors.New("no digit")
	}
	return atoi(s[:i]), nil
}

// atoi 简易 int 解析（段长有限，忽略错误）。
func atoi(s string) int {
	n := 0
	for i := range len(s) {
		n = n*10 + int(s[i]-'0')
	}
	return n
}
