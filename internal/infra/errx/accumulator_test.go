package errx

import (
	"bytes"
	"errors"
	"testing"
)

type failWriter struct {
	limit int
	wrote int
}

func (f *failWriter) Write(p []byte) (int, error) {
	if f.wrote+len(p) > f.limit {
		return 0, errors.New("write limit exceeded")
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
		_, _ = ew.WriteString("hello ")
		_, _ = ew.Write([]byte("world"))
		if err := ew.Err(); err != nil {
			t.Fatalf("expected nil, got %v", err)
		}
		if buf.String() != "hello world" {
			t.Errorf("got %q, want 'hello world'", buf.String())
		}
	})

	t.Run("write error accumulates", func(t *testing.T) {
		fw := &failWriter{limit: 5}
		ew := NewErrWriter(fw)
		_, _ = ew.WriteString("12345")
		_, _ = ew.WriteString("67890") // should fail and accumulate
		_, _ = ew.WriteString("abcde") // should be no-op

		if err := ew.Err(); err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}
