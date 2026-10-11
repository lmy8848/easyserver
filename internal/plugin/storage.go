package plugin

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"easyserver/internal/infra/errx"
)

// PluginStorage 插件私有文件区：/var/lib/easyserver/plugins/<name>/，
// 所有操作做路径规范化 + 前缀校验，防穿越（文档 §13/§17）。
type PluginStorage struct {
	base string
}

// NewPluginStorage 创建存储访问器。
func NewPluginStorage(base string) *PluginStorage {
	return &PluginStorage{base: base}
}

// safePath 解析插件视角的相对路径为绝对路径并确保落在插件目录内。
func (s *PluginStorage) safePath(plugin, rel string) (string, error) {
	rel = strings.TrimPrefix(filepath.ToSlash(rel), "/")
	clean := filepath.Clean("/" + rel) // 以 / 锚定后 Clean，吞掉所有 ..
	full := filepath.Join(s.base, plugin, clean)
	root := filepath.Join(s.base, plugin)
	if !strings.HasPrefix(full, root+string(filepath.Separator)) && full != root {
		return "", errx.Forbidden("路径越界")
	}
	return full, nil
}

// ReadFile 读取插件文件区内的文件。
func (s *PluginStorage) ReadFile(plugin, rel string) ([]byte, error) {
	p, err := s.safePath(plugin, rel)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(p)
}

// WriteFile 写入（MkdirAll 父目录，0644）。
func (s *PluginStorage) WriteFile(plugin, rel string, data []byte) error {
	p, err := s.safePath(plugin, rel)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		return err
	}
	return os.WriteFile(p, data, 0644)
}

// Delete 删除文件或目录。
func (s *PluginStorage) Delete(plugin, rel string) error {
	p, err := s.safePath(plugin, rel)
	if err != nil {
		return err
	}
	if p == filepath.Join(s.base, plugin) {
		return errx.Forbidden("拒绝删除插件根目录")
	}
	return os.RemoveAll(p)
}

// List 列出目录内容。
func (s *PluginStorage) List(plugin, rel string) ([]fs.FileInfo, error) {
	p, err := s.safePath(plugin, rel)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(p)
	if err != nil {
		return nil, err
	}
	out := make([]fs.FileInfo, 0, len(entries))
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue
		}
		out = append(out, info)
	}
	return out, nil
}

// String 调试用。
func (s *PluginStorage) String() string {
	return fmt.Sprintf("plugin-storage(%s)", s.base)
}
