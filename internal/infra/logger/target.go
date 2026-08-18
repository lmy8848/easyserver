package logger

import (
	"fmt"
	"io"
	"os"
	"sync"
)

// sinkSet 是运行时可变的输出目标组合（file + stderr，可 AddTarget 追加新目标）。
// 读写分离：写路径只取缓存的 multi writer，Add/reset 才加锁重建——热路径无锁竞争。
type sinkSet struct {
	mu      sync.RWMutex
	writers []writerSink
	multi   io.Writer // 惰性重建的 MultiWriter
}

func newSinkSet() *sinkSet { return &sinkSet{} }

func (s *sinkSet) Write(p []byte) (int, error) {
	s.mu.RLock()
	m := s.multi
	s.mu.RUnlock()
	if m == nil {
		return len(p), nil // 尚无目标：静默丢弃，不影响调用方
	}
	return m.Write(p)
}

// reset 用给定目标重建组合（Init 时：stderr 就位 → 文件成功后置前）。
func (s *sinkSet) reset(ws ...writerSink) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.writers = append(s.writers[:0], ws...)
	s.rebuildLocked()
}

// Add 运行时追加输出目标（扩展点：面板流式广播、远程聚合等）。
func (s *sinkSet) Add(w writerSink) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.writers = append(s.writers, w)
	s.rebuildLocked()
}

func (s *sinkSet) rebuildLocked() {
	switch len(s.writers) {
	case 0:
		s.multi = nil
	case 1:
		s.multi = s.writers[0]
	default:
		arr := make([]io.Writer, len(s.writers))
		for i, w := range s.writers {
			arr[i] = w
		}
		s.multi = io.MultiWriter(arr...)
	}
}

// fileTarget 是旋转文件的 io.Writer 实现：O_APPEND 追加、按大小轮转、
// 高可用降级（写失败不 panic、不丢日志、降级到 stderr 提示）。
type fileTarget struct {
	mu       sync.Mutex
	path     string
	maxSize  int64 // 单文件轮转阈值（字节）
	maxFiles int   // 保留轮转文件数 .1 .2 …
	f        *os.File
	size     int64
	failures int
}

func newFileTarget(path string, maxSize int64, maxFiles int) (*fileTarget, error) {
	dir := dirOf(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("logger: 创建日志目录 %s: %w", dir, err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("logger: 打开日志文件 %s: %w", path, err)
	}
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("logger: 读取日志文件 %s: %w", path, err)
	}
	return &fileTarget{
		path:     path,
		maxSize:  maxSize,
		maxFiles: maxFiles,
		f:        f,
		size:     info.Size(),
	}, nil
}

// Write 追加一行。写文件失败时：降级 stderr 提示、尝试重开，但仍按成功返回
// （让 sink 的 MultiWriter 继续把该行写给其他目标，如 stderr），保证不丢、不阻塞。
func (t *fileTarget) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.f == nil {
		if rerr := t.reopen(); rerr != nil {
			fmt.Fprintf(os.Stderr, "logger: 重开 %s 失败，丢弃本次输出: %v\n", t.path, rerr)
			return len(p), nil
		}
	}

	if t.size+int64(len(p)) > t.maxSize {
		t.rotate()
	}

	n, err := t.f.Write(p)
	if err != nil {
		t.failures++
		// 首次 / 每百次打一次提示，避免刷屏。
		if t.failures <= 3 || t.failures%100 == 0 {
			fmt.Fprintf(os.Stderr, "logger: 写入 %s 失败(第%d次): %v\n", t.path, t.failures, err)
		}
		if rerr := t.reopen(); rerr != nil {
			fmt.Fprintf(os.Stderr, "logger: 重开 %s 失败: %v\n", t.path, rerr)
		}
		return len(p), nil // 吞掉错误，让 MultiWriter 继续写给其他目标
	}
	t.size += int64(n)
	return n, nil
}

// Close 关闭当前文件句柄。
func (t *fileTarget) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.f == nil {
		return nil
	}
	err := t.f.Close()
	t.f = nil
	return err
}

// rotate 触发大小轮转：先关闭当前句柄（Windows 下重命名打开的文件会被拒绝），
// path.N-1→path.N … path→path.1 移位，再重开当前文件。轮转失败不 panic：
// 若重开失败则下次写入会尝试重开，日志不丢（最多短暂保持旧句柄/旧大小计数）。
func (t *fileTarget) rotate() {
	if t.f != nil {
		_ = t.f.Close()
		t.f = nil
	}
	for i := t.maxFiles - 1; i >= 1; i-- {
		_ = os.Rename(rotateName(t.path, i), rotateName(t.path, i+1)) // 忽略缺文件错误
	}
	if err := os.Rename(t.path, rotateName(t.path, 1)); err != nil {
		fmt.Fprintf(os.Stderr, "logger: 轮转重命名 %s→.1 失败: %v\n", t.path, err)
	}
	if f, err := os.OpenFile(t.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644); err == nil {
		t.f = f
		t.size = 0
		return
	}
	fmt.Fprintf(os.Stderr, "logger: 轮转重开 %s 失败，等待下次写入重试\n", t.path)
}

// reopen 写失败后尝试重开文件（如磁盘恢复、文件被外部删除），并重算实际大小。
func (t *fileTarget) reopen() error {
	f, err := os.OpenFile(t.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if t.f != nil {
		_ = t.f.Close()
	}
	t.f = f
	if fi, err := f.Stat(); err == nil {
		t.size = fi.Size()
	} else {
		t.size = 0
	}
	t.failures = 0
	return nil
}

func rotateName(path string, i int) string {
	return fmt.Sprintf("%s.%d", path, i)
}

func dirOf(p string) string {
	for i := len(p) - 1; i >= 0; i-- {
		if p[i] == '/' || p[i] == '\\' {
			if i == 0 {
				return p[:1]
			}
			return p[:i]
		}
	}
	return "."
}
