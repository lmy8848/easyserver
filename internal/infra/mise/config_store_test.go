package mise

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newTestStore 创建一个指向临时目录的 EnvStore（避免触碰 /opt 面板私有路径）。
func newTestStore(t *testing.T) (*EnvStore, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	return &EnvStore{path: path}, path
}

func TestEnvStore_ListMissingFile(t *testing.T) {
	s, _ := newTestStore(t)
	envs, err := s.ListEnv()
	if err != nil {
		t.Fatalf("ListEnv on missing file: %v", err)
	}
	if len(envs) != 0 {
		t.Fatalf("expected empty envs, got %v", envs)
	}
}

func TestEnvStore_UpsertRoundTrip(t *testing.T) {
	s, path := newTestStore(t)
	if err := s.UpsertEnv("MISE_NODE_MIRROR_URL", "https://npmmirror.com/mirrors/node/"); err != nil {
		t.Fatalf("UpsertEnv: %v", err)
	}
	if err := s.UpsertEnv("MISE_GO_DOWNLOAD_MIRROR", "https://mirrors.aliyun.com/golang/"); err != nil {
		t.Fatalf("UpsertEnv: %v", err)
	}

	envs, err := s.ListEnv()
	if err != nil {
		t.Fatalf("ListEnv: %v", err)
	}
	if envs["MISE_NODE_MIRROR_URL"] != "https://npmmirror.com/mirrors/node/" {
		t.Fatalf("unexpected value: %v", envs)
	}
	if envs["MISE_GO_DOWNLOAD_MIRROR"] != "https://mirrors.aliyun.com/golang/" {
		t.Fatalf("unexpected value: %v", envs)
	}

	// 覆盖更新
	if err := s.UpsertEnv("MISE_NODE_MIRROR_URL", "https://npmmirror.com/mirrors/node"); err != nil {
		t.Fatalf("UpsertEnv overwrite: %v", err)
	}
	envs, _ = s.ListEnv()
	if envs["MISE_NODE_MIRROR_URL"] != "https://npmmirror.com/mirrors/node" {
		t.Fatalf("overwrite failed: %v", envs)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if !strings.Contains(string(data), "[env]") {
		t.Fatalf("missing [env] section:\n%s", data)
	}
}

func TestEnvStore_PreservesUnknownSections(t *testing.T) {
	s, path := newTestStore(t)
	// 预置含未知段的文件（模拟用户手写 / 未来 [tools] 段）
	orig := "[env]\n\"MISE_NODE_MIRROR_URL\" = \"https://old.example.com/\"\n\n[tools]\nnode = \"20.11.0\"\n"
	if err := os.WriteFile(path, []byte(orig), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	if err := s.UpsertEnv("MISE_NODE_MIRROR_URL", "https://new.example.com/"); err != nil {
		t.Fatalf("UpsertEnv: %v", err)
	}
	if err := s.DeleteEnv("MISE_GO_DOWNLOAD_MIRROR"); err != nil {
		t.Fatalf("DeleteEnv non-existing: %v", err)
	}

	envs, err := s.ListEnv()
	if err != nil {
		t.Fatalf("ListEnv: %v", err)
	}
	if envs["MISE_NODE_MIRROR_URL"] != "https://new.example.com/" {
		t.Fatalf("update lost: %v", envs)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if !strings.Contains(string(data), "[tools]") || !strings.Contains(string(data), "20.11.0") {
		t.Fatalf("unknown section not preserved:\n%s", data)
	}
}

func TestEnvStore_Delete(t *testing.T) {
	s, _ := newTestStore(t)
	if err := s.UpsertEnv("A", "1"); err != nil {
		t.Fatalf("UpsertEnv: %v", err)
	}
	if err := s.UpsertEnv("B", "2"); err != nil {
		t.Fatalf("UpsertEnv: %v", err)
	}
	if err := s.DeleteEnv("A"); err != nil {
		t.Fatalf("DeleteEnv: %v", err)
	}
	envs, err := s.ListEnv()
	if err != nil {
		t.Fatalf("ListEnv: %v", err)
	}
	if _, ok := envs["A"]; ok {
		t.Fatalf("A should be gone: %v", envs)
	}
	if envs["B"] != "2" {
		t.Fatalf("B should remain: %v", envs)
	}
}

func TestEnvStore_InvalidFileNotOverwritten(t *testing.T) {
	s, path := newTestStore(t)
	broken := "this is [not valid toml = = =\n"
	if err := os.WriteFile(path, []byte(broken), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	if _, err := s.ListEnv(); err == nil {
		t.Fatal("expected error for invalid TOML")
	}
	if err := s.UpsertEnv("A", "1"); err == nil {
		t.Fatal("expected UpsertEnv to fail on invalid TOML")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if string(data) != broken {
		t.Fatalf("invalid file must not be overwritten, got:\n%s", data)
	}
}

func TestEnvStore_NonStringEnvValuesSkipped(t *testing.T) {
	s, path := newTestStore(t)
	// mise 的 [env] 支持 path 列表等非字符串语法，读取时应跳过不报错。
	orig := "[env]\n\"PLAIN\" = \"value\"\nPATH = { path = [\"/opt/x/bin\", \"/usr/bin\"] }\n"
	if err := os.WriteFile(path, []byte(orig), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	envs, err := s.ListEnv()
	if err != nil {
		t.Fatalf("ListEnv: %v", err)
	}
	if envs["PLAIN"] != "value" {
		t.Fatalf("plain value missing: %v", envs)
	}
	if _, ok := envs["PATH"]; ok {
		t.Fatalf("non-string env value must be skipped: %v", envs)
	}
}
