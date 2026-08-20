package errx

import (
	"io"
)

// StepRunner accumulates errors in sequential multi-step operations.
// Once an error occurs, subsequent Do() calls are skipped and the first error is preserved.
type StepRunner struct {
	err error
}

// Do executes fn only if no previous step has failed.
func (r *StepRunner) Do(fn func() error) {
	if r.err != nil || fn == nil {
		return
	}
	r.err = fn()
}

// Err returns the first error encountered during execution, or nil if all succeeded.
func (r *StepRunner) Err() error {
	return r.err
}

// Run executes a list of sequential steps, halting at the first failure.
func Run(steps ...func() error) error {
	var runner StepRunner
	for _, step := range steps {
		runner.Do(step)
		if runner.err != nil {
			break
		}
	}
	return runner.Err()
}

// ErrWriter wraps an io.Writer and accumulates the first write error encountered.
// Once an error occurs, subsequent Write calls become no-ops.
type ErrWriter struct {
	w   io.Writer
	err error
}

// NewErrWriter creates an ErrWriter wrapping w.
func NewErrWriter(w io.Writer) *ErrWriter {
	return &ErrWriter{w: w}
}

// Write writes bytes to the underlying writer if no error has occurred yet.
func (ew *ErrWriter) Write(p []byte) (int, error) {
	if ew.err != nil {
		return 0, ew.err
	}
	n, err := ew.w.Write(p)
	if err != nil {
		ew.err = err
	}
	return n, err
}

// WriteString writes a string to the underlying writer if no error has occurred yet.
func (ew *ErrWriter) WriteString(s string) (int, error) {
	if ew.err != nil {
		return 0, ew.err
	}
	n, err := io.WriteString(ew.w, s)
	if err != nil {
		ew.err = err
	}
	return n, err
}

// Err returns the accumulated write error, if any.
func (ew *ErrWriter) Err() error {
	return ew.err
}
