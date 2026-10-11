package sdk

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Manifest 是插件清单（与面板 contract 对齐的精简版，供插件自校验与
// 展示元数据）。面板侧完整校验以面板实现为准。
type Manifest struct {
	APIVersion            int    `yaml:"api_version"`
	ManifestSchemaVersion int    `yaml:"manifest_schema_version"`
	Name                  string `yaml:"name"`
	Version               string `yaml:"version"`
	Author                string `yaml:"author"`
	Description           string `yaml:"description"`
	Runtime               struct {
		Exec    string `yaml:"exec"`
		Socket  string `yaml:"socket"`
	} `yaml:"runtime"`
	Permissions []string `yaml:"permissions"`
}

// LoadManifest 读取并校验插件自身的 plugin.yaml。
// path 通常为进程 CWD 下的 "plugin.yaml"（面板以插件目录为工作目录拉起）。
func LoadManifest(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取 manifest: %w", err)
	}
	var m Manifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("manifest YAML 无效: %w", err)
	}
	if m.Name == "" || m.Version == "" || m.Runtime.Exec == "" {
		return nil, fmt.Errorf("manifest 缺少必填字段（name/version/runtime.exec）")
	}
	return &m, nil
}

// HasPermission 报告 manifest 是否声明了某能力。
func (m *Manifest) HasPermission(perm string) bool {
	for _, p := range m.Permissions {
		if p == perm {
			return true
		}
	}
	return false
}
