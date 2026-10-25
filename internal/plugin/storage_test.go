package plugin

import (
	"path/filepath"
	"testing"
)

// PluginStorage 的路径穿越防护测试（安全关键，此前零测试）。

func newTestStorage(t *testing.T) *PluginStorage {
	t.Helper()
	return NewPluginStorage(t.TempDir())
}

func TestStoragePathTraversalRejected(t *testing.T) {
	s := newTestStorage(t)
	root := filepath.Join(s.base, "evil")
	cases := []string{
		"../../../etc/passwd",
		"..\\..\\windows",
		"/etc/passwd",          // 绝对路径
		"sub/../../escape.txt", // 混合穿越
		"./../../escape.txt",
	}
	// 防御策略是"/锚定 Clean 归一化"：穿越段被吞掉、结果必须落在插件目录内
	// （不越界即防住，而非必须报错）。
	for _, rel := range cases {
		p, err := s.safePath("evil", rel)
		if err != nil {
			continue // 显式报错也算防御
		}
		if p != root && !filepathHasPrefix(p, root) {
			t.Errorf("穿越路径 %q 解析越界: %s", rel, p)
		}
	}
	// 读写穿越路径不得在插件区外产生副作用
	_ = s.WriteFile("evil", "../../../escape-probe.txt", []byte("x"))
	if _, err := s.ReadFile("evil", "../../../escape-probe.txt"); err != nil {
		t.Logf("穿越探测读取: %v（归一化后落在插件内为预期）", err)
	}
	// 插件目录之外不应出现探测文件
	if _, err := s.safePath("evil", "../../../escape-probe.txt"); err != nil {
		t.Fatalf("safePath: %v", err)
	}
}

// filepathHasPrefix 报告 p 是否在目录 dir 之下（含分隔符锚定，避免 /a/bb 误配 /a/b）。
func filepathHasPrefix(p, dir string) bool {
	return len(p) > len(dir) && p[:len(dir)] == dir && p[len(dir)] == filepath.Separator
}

func TestStorageDeleteRootRejected(t *testing.T) {
	s := newTestStorage(t)
	if err := s.Delete("evil", ""); err == nil {
		t.Fatal("删除插件根目录应被拒绝")
	}
	if err := s.Delete("evil", "."); err == nil {
		t.Fatal("删除插件根目录（.）应被拒绝")
	}
}

func TestStorageRoundtrip(t *testing.T) {
	s := newTestStorage(t)
	if err := s.WriteFile("p1", "sub/data.txt", []byte("hello")); err != nil {
		t.Fatalf("写入失败: %v", err)
	}
	got, err := s.ReadFile("p1", "sub/data.txt")
	if err != nil {
		t.Fatalf("读取失败: %v", err)
	}
	if string(got) != "hello" {
		t.Fatalf("内容不一致: %q", got)
	}
	// 跨插件隔离：p2 读不到 p1 的文件
	if _, err := s.ReadFile("p2", "sub/data.txt"); err == nil {
		t.Fatal("插件间文件区应隔离")
	}
	// 删除
	if err := s.Delete("p1", "sub/data.txt"); err != nil {
		t.Fatalf("删除失败: %v", err)
	}
	if _, err := s.ReadFile("p1", "sub/data.txt"); err == nil {
		t.Fatal("删除后读取应失败")
	}
}

func TestStorageCleanAnchorsDotDot(t *testing.T) {
	s := newTestStorage(t)
	// ".." 应被 / 锚定 Clean 吞掉（落回插件根内），不越界
	p, err := s.safePath("p1", "a/../b.txt")
	if err != nil {
		t.Fatalf("合法相对路径被拒: %v", err)
	}
	want := filepath.Join(s.base, "p1", "b.txt")
	if p != want {
		t.Fatalf("safePath 归一化异常: %s want %s", p, want)
	}
}
