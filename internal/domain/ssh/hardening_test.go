package ssh

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"
)

func TestGenerateKeyPairEd25519(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)

	s := NewService()
	privPEM, err := s.GenerateKeyPair(context.Background(), "test-ed25519", "ed25519")
	if err != nil {
		t.Fatalf("GenerateKeyPair ed25519 failed: %v", err)
	}

	// Verify private key is valid OpenSSH PEM
	signer, err := ssh.ParsePrivateKey([]byte(privPEM))
	if err != nil {
		t.Fatalf("Failed to parse generated private key: %v", err)
	}
	if signer.PublicKey().Type() != "ssh-ed25519" {
		t.Fatalf("Expected ssh-ed25519 key type, got %s", signer.PublicKey().Type())
	}

	// Verify public key was added to authorized_keys
	keys, err := s.ListAuthorizedKeys()
	if err != nil {
		t.Fatalf("ListAuthorizedKeys failed: %v", err)
	}
	if len(keys) != 1 {
		t.Fatalf("Expected 1 key, got %d", len(keys))
	}
	if keys[0].Type != "ssh-ed25519" {
		t.Fatalf("Expected type ssh-ed25519, got %s", keys[0].Type)
	}
	if keys[0].Comment != "test-ed25519@easyserver" {
		t.Fatalf("Expected comment test-ed25519@easyserver, got %s", keys[0].Comment)
	}
	if !strings.HasPrefix(keys[0].Fingerprint, "SHA256:") {
		t.Fatalf("Expected SHA256 fingerprint, got %s", keys[0].Fingerprint)
	}
}

func TestGenerateKeyPairRSA(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)

	s := NewService()
	privPEM, err := s.GenerateKeyPair(context.Background(), "test-rsa", "rsa")
	if err != nil {
		t.Fatalf("GenerateKeyPair rsa failed: %v", err)
	}

	signer, err := ssh.ParsePrivateKey([]byte(privPEM))
	if err != nil {
		t.Fatalf("Failed to parse RSA private key: %v", err)
	}
	if signer.PublicKey().Type() != "ssh-rsa" {
		t.Fatalf("Expected ssh-rsa key type, got %s", signer.PublicKey().Type())
	}

	keys, err := s.ListAuthorizedKeys()
	if err != nil {
		t.Fatalf("ListAuthorizedKeys failed: %v", err)
	}
	if len(keys) != 1 {
		t.Fatalf("Expected 1 key, got %d", len(keys))
	}
	if keys[0].Type != "ssh-rsa" {
		t.Fatalf("Expected type ssh-rsa, got %s", keys[0].Type)
	}
}

func TestGenerateKeyPairUnsupportedECDSA(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)

	s := NewService()
	_, err := s.GenerateKeyPair(context.Background(), "test-ecdsa", "ecdsa")
	if err == nil {
		t.Fatal("Expected error for unsupported key type ecdsa")
	}
}

func TestAddAndRemoveAuthorizedKey(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)

	s := NewService()

	// 1. Invalid keys should be rejected
	if err := s.AddAuthorizedKey(""); err == nil {
		t.Fatal("Expected error for empty key")
	}
	if err := s.AddAuthorizedKey("invalid-ssh-key-data"); err == nil {
		t.Fatal("Expected error for invalid key")
	}

	// 2. Add valid key
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate ed25519: %v", err)
	}
	sshPub, err := ssh.NewPublicKey(pub)
	if err != nil {
		t.Fatalf("new public key: %v", err)
	}
	pubLine := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(sshPub))) + " user1@example.com"
	if err := s.AddAuthorizedKey(pubLine); err != nil {
		t.Fatalf("AddAuthorizedKey failed: %v", err)
	}

	// Add second key
	pub2, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate ed25519 2: %v", err)
	}
	sshPub2, err := ssh.NewPublicKey(pub2)
	if err != nil {
		t.Fatalf("new public key 2: %v", err)
	}
	pubLine2 := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(sshPub2))) + " user2@example.com"
	if err := s.AddAuthorizedKey(pubLine2); err != nil {
		t.Fatalf("AddAuthorizedKey 2 failed: %v", err)
	}

	keys, err := s.ListAuthorizedKeys()
	if err != nil {
		t.Fatalf("ListAuthorizedKeys failed: %v", err)
	}
	if len(keys) != 2 {
		t.Fatalf("Expected 2 keys, got %d", len(keys))
	}

	// 3. Remove key by comment
	if err := s.RemoveAuthorizedKey("user1@example.com"); err != nil {
		t.Fatalf("RemoveAuthorizedKey failed: %v", err)
	}

	keysAfter, err := s.ListAuthorizedKeys()
	if err != nil {
		t.Fatalf("ListAuthorizedKeys after removal failed: %v", err)
	}
	if len(keysAfter) != 1 {
		t.Fatalf("Expected 1 key after removal, got %d", len(keysAfter))
	}
	if keysAfter[0].Comment != "user2@example.com" {
		t.Fatalf("Expected remaining key comment user2@example.com, got %s", keysAfter[0].Comment)
	}

	// 4. File permissions check
	authPath := filepath.Join(tempHome, ".ssh", "authorized_keys")
	info, err := os.Stat(authPath)
	if err != nil {
		t.Fatalf("Stat authorized_keys: %v", err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("Expected 0600 permissions, got %o", info.Mode().Perm())
	}
}

func TestKillSessionValidation(t *testing.T) {
	s := NewService()
	ctx := context.Background()

	// PID <= 1
	if err := s.KillSession(ctx, 0); err == nil {
		t.Fatal("Expected error for PID 0")
	}
	if err := s.KillSession(ctx, 1); err == nil {
		t.Fatal("Expected error for PID 1")
	}
	// Self PID
	if err := s.KillSession(ctx, os.Getpid()); err == nil {
		t.Fatal("Expected error for self PID")
	}
	// Random non-SSH PID
	if err := s.KillSession(ctx, 999999); err == nil {
		t.Fatal("Expected error for nonexistent/non-SSH PID")
	}
}
