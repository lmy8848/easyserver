package deploy

import (
	"bytes"
	"encoding/pem"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"crypto/x509"

	"golang.org/x/crypto/ssh"
)

// SSH 超时常量
const (
	SSHConnectTimeout = 10 * time.Second
	SSHCommandTimeout = 5 * time.Minute
	SSHMkdirTimeout   = 10 * time.Second
)

// SSHClient wraps an SSH connection to a remote server.
type SSHClient struct {
	conn     *ssh.Client
	host     string
	port     int
	username string
}

// NewSSHClient creates an SSHClient connected to the given Server.
// authData is the decrypted password or private key content.
func NewSSHClient(srv *Server, authData string) (*SSHClient, error) {
	if srv == nil {
		return nil, fmt.Errorf("ssh: deploy server is nil")
	}

	authMethod, err := parseAuthMethod(srv.AuthType, authData)
	if err != nil {
		return nil, fmt.Errorf("ssh: auth error: %w", err)
	}

	// TODO: Replace InsecureIgnoreHostKey with known-hosts verification for production use.
	// Current implementation is vulnerable to MITM attacks.
	log.Printf("ssh: WARNING - host key verification disabled for %s:%d (MITM risk)", srv.Host, srv.Port)
	config := &ssh.ClientConfig{
		User:            srv.Username,
		Auth:            []ssh.AuthMethod{authMethod},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         SSHConnectTimeout,
	}

	addr := net.JoinHostPort(srv.Host, strconv.Itoa(srv.Port))
	conn, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		return nil, fmt.Errorf("ssh: failed to connect to %s: %w", addr, err)
	}

	return &SSHClient{
		conn:     conn,
		host:     srv.Host,
		port:     srv.Port,
		username: srv.Username,
	}, nil
}

// Close closes the underlying SSH connection.
func (c *SSHClient) Close() error {
	if c == nil || c.conn == nil {
		return nil
	}
	return c.conn.Close()
}

// RunCommand executes cmd on the remote server and returns stdout, stderr,
// exit code, and any error. The command is killed after the given timeout.
func (c *SSHClient) RunCommand(cmd string, timeout time.Duration) (stdout, stderr string, exitCode int, err error) {
	if c == nil || c.conn == nil {
		return "", "", -1, fmt.Errorf("ssh: client is not connected")
	}

	session, err := c.conn.NewSession()
	if err != nil {
		return "", "", -1, fmt.Errorf("ssh: failed to create session: %w", err)
	}
	defer session.Close()

	var outBuf, errBuf bytes.Buffer
	session.Stdout = &outBuf
	session.Stderr = &errBuf

	// Run command directly - timeout is handled by goroutine + select pattern
	wrappedCmd := cmd

	// Use a channel to detect when the command finishes.
	done := make(chan error, 1)
	go func() {
		done <- session.Run(wrappedCmd)
	}()

	select {
	case <-time.After(timeout):
		// Attempt to close the session to kill the remote process.
		session.Close()
		return outBuf.String(), errBuf.String(), -1, fmt.Errorf("ssh: command timed out after %s", timeout)
	case runErr := <-done:
		stdout = outBuf.String()
		stderr = errBuf.String()
		if runErr != nil {
			if exitErr, ok := runErr.(*ssh.ExitError); ok {
				return stdout, stderr, exitErr.ExitStatus(), nil
			}
			return stdout, stderr, -1, fmt.Errorf("ssh: command failed: %w", runErr)
		}
		return stdout, stderr, 0, nil
	}
}

// UploadFile uploads a local file to the remote server using the SCP protocol.
func (c *SSHClient) UploadFile(localPath, remotePath string) error {
	if c == nil || c.conn == nil {
		return fmt.Errorf("ssh: client is not connected")
	}

	// Read local file.
	info, err := os.Stat(localPath)
	if err != nil {
		return fmt.Errorf("ssh: failed to stat local file: %w", err)
	}
	data, err := os.ReadFile(localPath)
	if err != nil {
		return fmt.Errorf("ssh: failed to read local file: %w", err)
	}

	session, err := c.conn.NewSession()
	if err != nil {
		return fmt.Errorf("ssh: failed to create session: %w", err)
	}
	defer session.Close()

	stdin, err := session.StdinPipe()
	if err != nil {
		return fmt.Errorf("ssh: failed to open stdin pipe: %w", err)
	}

	stdout, err := session.StdoutPipe()
	if err != nil {
		return fmt.Errorf("ssh: failed to open stdout pipe: %w", err)
	}

	// Start scp in sink mode (-t = to remote).
	if err := session.Start(fmt.Sprintf("scp -t %q", remotePath)); err != nil {
		return fmt.Errorf("ssh: failed to start scp: %w", err)
	}

	// Helper: read a single byte from stdout and check it is a zero acknowledgment.
	ack := func() error {
		buf := make([]byte, 1)
		if _, err := io.ReadFull(stdout, buf); err != nil {
			return fmt.Errorf("ssh: failed to read scp ack: %w", err)
		}
		if buf[0] != 0 {
			return fmt.Errorf("ssh: scp remote error: %s", readErrorMessage(stdout, buf[0]))
		}
		return nil
	}

	// Wait for initial ack.
	if err := ack(); err != nil {
		return err
	}

	// Send file mode, size, and name.
	fileName := filepath.Base(remotePath)
	header := fmt.Sprintf("C%04o %d %s\n", info.Mode().Perm(), len(data), fileName)
	if _, err := stdin.Write([]byte(header)); err != nil {
		return fmt.Errorf("ssh: failed to send scp header: %w", err)
	}

	// Wait for ack after header.
	if err := ack(); err != nil {
		return err
	}

	// Send file content.
	if _, err := stdin.Write(data); err != nil {
		return fmt.Errorf("ssh: failed to send file content: %w", err)
	}

	// Send end-of-file marker.
	if _, err := stdin.Write([]byte{0}); err != nil {
		return fmt.Errorf("ssh: failed to send scp eof: %w", err)
	}

	// Wait for final ack.
	if err := ack(); err != nil {
		return err
	}

	// Close stdin to signal we are done.
	stdin.Close()

	if err := session.Wait(); err != nil {
		return fmt.Errorf("ssh: scp upload failed: %w", err)
	}

	return nil
}

// DownloadFile downloads a file from the remote server to a local path using
// the SCP protocol.
func (c *SSHClient) DownloadFile(remotePath, localPath string) error {
	if c == nil || c.conn == nil {
		return fmt.Errorf("ssh: client is not connected")
	}

	session, err := c.conn.NewSession()
	if err != nil {
		return fmt.Errorf("ssh: failed to create session: %w", err)
	}
	defer session.Close()

	stdin, err := session.StdinPipe()
	if err != nil {
		return fmt.Errorf("ssh: failed to open stdin pipe: %w", err)
	}

	stdout, err := session.StdoutPipe()
	if err != nil {
		return fmt.Errorf("ssh: failed to open stdout pipe: %w", err)
	}

	// Start scp in source mode (-f = from remote).
	if err := session.Start(fmt.Sprintf("scp -f %q", remotePath)); err != nil {
		return fmt.Errorf("ssh: failed to start scp: %w", err)
	}

	// Send initial ack to indicate we are ready.
	if _, err := stdin.Write([]byte{0}); err != nil {
		return fmt.Errorf("ssh: failed to send initial ack: %w", err)
	}

	// Read the file metadata header from the remote side.
	// Format: C<mode> <size> <filename>\n
	headerBuf := make([]byte, 0, 128)
	for {
		b := make([]byte, 1)
		if _, err := io.ReadFull(stdout, b); err != nil {
			return fmt.Errorf("ssh: failed to read scp header: %w", err)
		}
		if b[0] == '\n' {
			break
		}
		headerBuf = append(headerBuf, b[0])
	}

	header := string(headerBuf)
	if len(header) < 1 || header[0] != 'C' {
		return fmt.Errorf("ssh: unexpected scp response: %q", header)
	}

	// Parse: C<mode> <size> <filename>
	parts := strings.SplitN(header[1:], " ", 3)
	if len(parts) < 2 {
		return fmt.Errorf("ssh: malformed scp header: %q", header)
	}

	fileSize, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return fmt.Errorf("ssh: malformed scp file size %q: %w", parts[1], err)
	}

	// Acknowledge the header.
	if _, err := stdin.Write([]byte{0}); err != nil {
		return fmt.Errorf("ssh: failed to ack scp header: %w", err)
	}

	// Read exactly fileSize bytes of content.
	fileData := make([]byte, fileSize)
	if _, err := io.ReadFull(stdout, fileData); err != nil {
		return fmt.Errorf("ssh: failed to read scp file content: %w", err)
	}

	// Read end-of-file marker (single byte: 0 = success).
	eofBuf := make([]byte, 1)
	if _, err := io.ReadFull(stdout, eofBuf); err != nil {
		return fmt.Errorf("ssh: failed to read scp eof: %w", err)
	}
	if eofBuf[0] != 0 {
		return fmt.Errorf("ssh: scp remote sent error byte: 0x%02x", eofBuf[0])
	}

	// Acknowledge eof.
	if _, err := stdin.Write([]byte{0}); err != nil {
		return fmt.Errorf("ssh: failed to ack scp eof: %w", err)
	}

	// Ensure the local directory exists.
	dir := filepath.Dir(localPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("ssh: failed to create local directory: %w", err)
	}

	// Write to local file.
	if err := os.WriteFile(localPath, fileData, 0o644); err != nil {
		return fmt.Errorf("ssh: failed to write local file: %w", err)
	}

	stdin.Close()
	if err := session.Wait(); err != nil {
		return fmt.Errorf("ssh: scp download failed: %w", err)
	}

	return nil
}

// parseAuthMethod returns an ssh.AuthMethod for the given auth type and data.
func parseAuthMethod(authType, authData string) (ssh.AuthMethod, error) {
	switch authType {
	case "password":
		return ssh.Password(authData), nil
	case "key":
		signer, err := ssh.ParsePrivateKey([]byte(authData))
		if err != nil {
			// Try parsing as PKCS8 or other format via x509.
			block, _ := pem.Decode([]byte(authData))
			if block == nil {
				return nil, fmt.Errorf("failed to parse private key: %w", err)
			}
			key, x509Err := x509.ParsePKCS8PrivateKey(block.Bytes)
			if x509Err != nil {
				// Return the original ParsePrivateKey error as it is more relevant.
				return nil, fmt.Errorf("failed to parse private key: %w", err)
			}
			signer, err = ssh.NewSignerFromKey(key)
			if err != nil {
				return nil, fmt.Errorf("failed to create signer from key: %w", err)
			}
		}
		return ssh.PublicKeys(signer), nil
	default:
		return nil, fmt.Errorf("unsupported auth type: %q (expected 'password' or 'key')", authType)
	}
}

// readErrorMessage reads an error message string from the SCP stream.
// It consumes bytes until a newline is encountered.
func readErrorMessage(r io.Reader, firstByte byte) string {
	var buf []byte
	buf = append(buf, firstByte)
	tmp := make([]byte, 1)
	for {
		if _, err := io.ReadFull(r, tmp); err != nil {
			break
		}
		if tmp[0] == '\n' {
			break
		}
		buf = append(buf, tmp[0])
	}
	return strings.TrimSpace(string(buf))
}
