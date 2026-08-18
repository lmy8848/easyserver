package logger

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestParseLevel(t *testing.T) {
	cases := map[string]slog.Level{
		"debug": slog.LevelDebug,
		"info":  slog.LevelInfo,
		"warn":  slog.LevelWarn,
		"error": slog.LevelError,
		"INFO":  slog.LevelInfo,
		"":      slog.LevelInfo,
	}
	for in, want := range cases {
		got, err := parseLevel(in)
		if err != nil {
			t.Fatalf("parseLevel(%q) unexpected err: %v", in, err)
		}
		if got != want {
			t.Fatalf("parseLevel(%q)=%v want %v", in, got, want)
		}
	}
	if _, err := parseLevel("verbose"); err == nil {
		t.Fatal("parseLevel(verbose) should error")
	}
}

func TestLevelFiltering(t *testing.T) {
	defer func() { _ = SetLevel("info") }()
	var buf bytes.Buffer
	h := makeHandler(&buf, false, "text")
	l := slog.New(h)

	if err := SetLevel("warn"); err != nil {
		t.Fatal(err)
	}
	l.Info("should-be-hidden")
	l.Warn("warn-shown")
	l.Error("error-shown")

	out := buf.String()
	if strings.Contains(out, "should-be-hidden") {
		t.Fatalf("INFO leaked at WARN level:\n%s", out)
	}
	if !strings.Contains(out, "warn-shown") || !strings.Contains(out, "error-shown") {
		t.Fatalf("WARN/ERROR not captured:\n%s", out)
	}
}

func TestSourceLocation(t *testing.T) {
	defer func() { _ = SetLevel("info") }()
	var buf bytes.Buffer
	h := makeHandler(&buf, true, "text")
	slog.New(h).Error("boom")
	out := buf.String()
	if !strings.Contains(out, "logger_test.go") || !strings.Contains(out, "@") {
		t.Fatalf("expected source location (func@file:line), got:\n%s", out)
	}
}

func TestJSONFormat(t *testing.T) {
	defer func() { _ = SetLevel("info") }()
	var buf bytes.Buffer
	h := makeHandler(&buf, true, "json")
	slog.New(h).Info("jsonok")
	if !strings.Contains(buf.String(), `"msg":"jsonok"`) || !strings.HasPrefix(buf.String(), "{") {
		t.Fatalf("expected JSON record, got:\n%s", buf.String())
	}
}

func TestRotation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.log")
	ft, err := newFileTarget(path, 100, 2)
	if err != nil {
		t.Fatal(err)
	}
	defer ft.Close()

	payload := []byte("0123456789abcdefghij") // 20 bytes
	for range 30 {                            // 600 bytes → 触发多次轮转
		if _, err := ft.Write(payload); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := os.Stat(path + ".1"); err != nil {
		t.Errorf("expected rotated %s.1: %v", path, err)
	}
	if _, err := os.Stat(path + ".2"); err != nil {
		t.Errorf("expected rotated %s.2: %v", path, err)
	}
	// 主文件被重开，仍有内容可写。
	if _, err := ft.Write(payload); err != nil {
		t.Fatal(err)
	}
}

func TestRotationRespectsMaxFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.log")
	ft, err := newFileTarget(path, 100, 2)
	if err != nil {
		t.Fatal(err)
	}
	defer ft.Close()
	payload := make([]byte, 100)
	for range 40 {
		_, _ = ft.Write(payload)
	}
	if _, err := os.Stat(path + ".3"); err == nil {
		t.Errorf(".3 should not exist when maxFiles=2")
	}
}

func TestHookFireAndRemove(t *testing.T) {
	defer func() { _ = SetLevel("info") }()
	var mu sync.Mutex
	var msgs []string
	remove := AddHook(func(_ context.Context, r slog.Record) {
		mu.Lock()
		msgs = append(msgs, r.Message)
		mu.Unlock()
	})

	var buf bytes.Buffer
	h := makeHandler(&buf, false, "text")
	l := slog.New(h)
	l.Info("hook-me")
	l.Warn("hook-me-too")

	mu.Lock()
	got := append([]string(nil), msgs...)
	mu.Unlock()
	if len(got) != 2 {
		t.Fatalf("expected 2 hook calls, got %d: %v", len(got), got)
	}

	remove()
	l.Error("after-remove")
	mu.Lock()
	got = append([]string(nil), msgs...)
	mu.Unlock()
	if len(got) != 2 {
		t.Fatalf("hook should be removed, got %d calls: %v", len(got), got)
	}
}

func TestSetLevelInvalidKeepsCurrent(t *testing.T) {
	defer func() { _ = SetLevel("info") }()
	_ = SetLevel("warn")
	if err := SetLevel("bogus"); err == nil {
		t.Fatal("bogus level should error")
	}
	if GetLevel() != "warn" {
		t.Fatalf("level should stay warn, got %q", GetLevel())
	}
}
