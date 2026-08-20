package errx

import (
	"bytes"
	"errors"
	"testing"
)

type failWriter struct {
	limit int
	wrote int
	calls int
	err   error
}

func (f *failWriter) Write(p []byte) (int, error) {
	f.calls++
	if f.wrote+len(p) > f.limit {
		return 0, f.err
	}
	f.wrote += len(p)
	return len(p), nil
}

func TestStepRunner(t *testing.T) {
	t.Run("all steps succeed", func(t *testing.T) {
		var runner StepRunner
		count := 0
		runner.Do(func() error { count++; return nil })
		runner.Do(func() error { count++; return nil })
		if err := runner.Err(); err != nil {
			t.Fatalf("expected nil, got %v", err)
		}
		if count != 2 {
			t.Errorf("expected count 2, got %d", count)
		}
	})

	t.Run("first failure halts execution", func(t *testing.T) {
		var runner StepRunner
		errTarget := errors.New("step 1 failed")
		count := 0
		runner.Do(func() error { return errTarget })
		runner.Do(func() error { count++; return nil })
		if !errors.Is(runner.Err(), errTarget) {
			t.Fatalf("expected %v, got %v", errTarget, runner.Err())
		}
		if count != 0 {
			t.Errorf("step 2 should not have run, count=%d", count)
		}
	})

	t.Run("Run helper", func(t *testing.T) {
		errTarget := errors.New("step 2 failed")
		err := Run(
			func() error { return nil },
			func() error { return errTarget },
			func() error { return errors.New("step 3") },
		)
		if !errors.Is(err, errTarget) {
			t.Fatalf("expected %v, got %v", errTarget, err)
		}
	})
}

func TestErrWriter(t *testing.T) {
	t.Run("successful writes", func(t *testing.T) {
		var buf bytes.Buffer
		ew := NewErrWriter(&buf)
		if _, err := ew.WriteString("hello "); err != nil {
			t.Fatalf("WriteString() error = %v", err)
		}
		if _, err := ew.Write([]byte("world")); err != nil {
			t.Fatalf("Write() error = %v", err)
		}
		if err := ew.Err(); err != nil {
			t.Fatalf("expected nil, got %v", err)
		}
		if buf.String() != "hello world" {
			t.Errorf("got %q, want 'hello world'", buf.String())
		}
	})

	t.Run("write error accumulates and subsequent writes become no-ops", func(t *testing.T) {
		errTarget := errors.New("write limit exceeded")
		fw := &failWriter{limit: 5, err: errTarget}
		ew := NewErrWriter(fw)
		if _, err := ew.WriteString("12345"); err != nil {
			t.Fatalf("first write error = %v", err)
		}
		if _, err := ew.WriteString("67890"); !errors.Is(err, errTarget) {
			t.Fatalf("second write error = %v, want %v", err, errTarget)
		}
		if _, err := ew.WriteString("abcde"); !errors.Is(err, errTarget) {
			t.Fatalf("third write error = %v, want %v", err, errTarget)
		}
		if fw.calls != 2 {
			t.Fatalf("underlying writer calls = %d, want 2", fw.calls)
		}
		if !errors.Is(ew.Err(), errTarget) {
			t.Fatalf("ew.Err() = %v, want %v", ew.Err(), errTarget)
		}
	})
}
