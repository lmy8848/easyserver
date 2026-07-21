// Package launcher owns the TCP listener lifecycle and hot-restart logic.
// It replaces the former package-level globalListener in internal/api/response.go:
// the listener is held by a *Launcher value constructed in main.go and injected
// into the settings handler, instead of being stored in a hidden global.
package launcher

import (
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"strconv"
)

// AcquireListener returns a TCP listener on addr. When EASYSERVER_INHERIT_FD
// is set (hot restart), the parent's listener FD is inherited via
// net.FileListener; otherwise a fresh listener is bound. Called at process
// start in main.go.
func AcquireListener(addr string) (net.Listener, error) {
	if inheritFD := os.Getenv("EASYSERVER_INHERIT_FD"); inheritFD != "" {
		if fdNum, err := strconv.Atoi(inheritFD); err == nil {
			f := os.NewFile(uintptr(fdNum), "listener")
			if f != nil {
				ln, err := net.FileListener(f)
				f.Close()
				if err == nil {
					log.Printf("launcher: inherited listener from parent on %s", addr)
					return ln, nil
				}
			}
		}
	}
	return net.Listen("tcp", addr)
}

// Launcher holds the TCP listener and orchestrates hot restart via FD passing
// to a child process.
type Launcher struct {
	ln net.Listener
}

// New creates a Launcher holding the given listener.
func New(ln net.Listener) *Launcher {
	return &Launcher{ln: ln}
}

// RestartOpts configures a restart.
type RestartOpts struct {
	ConfigPath string
	DevMode    bool
	Force      bool
}

// Restart forks a child process and exits the parent.
//
// In force mode the listener is closed first so the child re-binds on a new
// address (e.g. after a port change); no FD is inherited. Otherwise the
// listener FD is passed to the child for zero-downtime restart.
//
// Known issues preserved as-is (not fixed in this refactor):
//   - force mode has a window between CloseListener and child bind where the
//     port can be grabbed by another process;
//   - graceful mode exits the parent immediately after fork, before the child
//     is confirmed ready (clients may see connection refused).
//
// TLS configuration is validated by the caller (settings handler), not here.
func (l *Launcher) Restart(opts RestartOpts) error {
	log.Printf("launcher: restarting panel (force=%v)...", opts.Force)

	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve executable path: %w", err)
	}
	execPath, err = filepath.EvalSymlinks(execPath)
	if err != nil {
		return fmt.Errorf("resolve symlink: %w", err)
	}

	args := []string{execPath, "-config", opts.ConfigPath}
	if opts.DevMode {
		args = append(args, "-dev")
	}

	if opts.Force {
		// Force mode: close the listener so the child re-binds on the
		// new port/host from config. No FD inheritance.
		l.closeListener()
		child, err := os.StartProcess(execPath, args, &os.ProcAttr{
			Files: []*os.File{os.Stdin, os.Stdout, os.Stderr},
			Env:   os.Environ(),
		})
		if err != nil {
			return fmt.Errorf("fork child (force): %w", err)
		}
		log.Printf("launcher: forked child PID %d (force restart, new listener), exiting parent", child.Pid)
		os.Exit(0)
		return nil
	}

	// Graceful mode: fork a child process that inherits our TCP listener FD.
	// Zero-downtime restart - no port contention, no polling.
	listenerFile := l.dupListenerFile()
	if listenerFile == nil {
		return fmt.Errorf("no listener available for restart")
	}
	childEnv := append(os.Environ(),
		"EASYSERVER_INHERIT_FD=3", // first FD after stdin(0)/stdout(1)/stderr(2)
	)
	child, err := os.StartProcess(execPath, args, &os.ProcAttr{
		Files: []*os.File{os.Stdin, os.Stdout, os.Stderr, listenerFile},
		Env:   childEnv,
	})
	listenerFile.Close() // close our copy; child has its own
	if err != nil {
		return fmt.Errorf("fork child: %w", err)
	}
	log.Printf("launcher: forked child PID %d, exiting parent", child.Pid)

	// Exit immediately - child already has the listener FD.
	os.Exit(0)
	return nil
}

// closeListener releases the listener so the child can bind a fresh one.
func (l *Launcher) closeListener() {
	l.ln.Close()
}

// dupListenerFile returns a dup'd *os.File for the listener, or nil. The
// caller must close the file when done.
func (l *Launcher) dupListenerFile() *os.File {
	tcpLn, ok := l.ln.(*net.TCPListener)
	if !ok {
		return nil
	}
	f, err := tcpLn.File()
	if err != nil {
		return nil
	}
	return f
}
