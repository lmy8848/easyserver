package runtimeenv

import "strings"

type Runtime struct {
	Lang       string   // "node"
	Display    string   // "Node.js"
	MiseTool   string   // mise 插件名
	Majors     []string // UI 可选的主版本列表
	MirrorEnvs []string // 该语言会用到的 env key（供镜像表 seed 使用）
}

var catalog = []Runtime{
	{
		Lang:       "node",
		Display:    "Node.js",
		MiseTool:   "node",
		Majors:     []string{"18", "20", "22"},
		MirrorEnvs: []string{"MISE_NODE_MIRROR_URL"},
	},
	{
		Lang:       "python",
		Display:    "Python",
		MiseTool:   "python",
		Majors:     []string{"3.10", "3.11", "3.12"},
		MirrorEnvs: []string{},
	},
	{
		Lang:       "go",
		Display:    "Go",
		MiseTool:   "go",
		Majors:     []string{"1.21", "1.22", "1.23"},
		MirrorEnvs: []string{"MISE_GO_DOWNLOAD_MIRROR"},
	},
	{
		Lang:       "java",
		Display:    "Java",
		MiseTool:   "vfox:version-fox/vfox-java",
		Majors:     []string{"11", "17", "21"},
		MirrorEnvs: []string{},
	},
	{
		Lang:       "php",
		Display:    "PHP",
		MiseTool:   "vfox:mise-plugins/vfox-php",
		Majors:     []string{"8.1", "8.2", "8.3"},
		MirrorEnvs: []string{},
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
func GetCatalog() []Runtime {
	c := make([]Runtime, len(catalog))
	for i, r := range catalog {
		c[i] = Runtime{
			Lang:       r.Lang,
			Display:    r.Display,
			MiseTool:   r.MiseTool,
			Majors:     append([]string(nil), r.Majors...),
			MirrorEnvs: append([]string(nil), r.MirrorEnvs...),
		}
	}
	return c
}
