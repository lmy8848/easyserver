package runtimeenv

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	targetMiseVersion   = "v2026.6.14"
	fallbackMiseVersion = "v2026.6.13"
	targetMiseSha256    = "96ae1ef7b00a6ebbbec23ba1016d6e722f5e904966272f621d15326429e90d53"
	fallbackMiseSha256  = "d80fa6b4be4d926e7a09fadab1ba777c9ce52f9c22de6d8f6e3cce8d66d281d8"
	miseBinPath         = "/usr/local/bin/mise"
	miseDataDir         = "/var/lib/easyserver/mise"
	miseProfilePath     = "/etc/profile.d/mise.sh"
)

// BootstrapMise ensures mise is installed and configured correctly
func BootstrapMise() error {
	// 1. Check if already installed and version matches
	if err := checkMiseVersion(); err == nil {
		return setupMiseEnv()
	}

	// 2. Download binary directly
	if err := downloadMise(targetMiseVersion, targetMiseSha256); err != nil {
		log.Printf("Failed to download target version %s: %v, falling back to %s", targetMiseVersion, err, fallbackMiseVersion)
		if errFallback := downloadMise(fallbackMiseVersion, fallbackMiseSha256); errFallback != nil {
			return fmt.Errorf("failed to download both target and fallback: %w", errFallback)
		}
	}

	// 3. Setup environment
	return setupMiseEnv()
}

func checkMiseVersion() error {
	cmd := exec.Command(miseBinPath, "--version")
	out, err := cmd.Output()
	if err != nil {
		return err
	}
	verStr := strings.TrimSpace(string(out))

	// mise --version output evolved over releases. Modern builds emit
	// "2026.6.13 linux-x64 (...)" with no leading "mise " — the old
	// " <ver> " space-padded substring check missed those because the
	// version sits at byte 0. Tokenize and compare exact fields instead.
	//
	// If the installed mise isn't on our pinned list we still accept it —
	// the user put it there; redownloading every boot is hostile. Pinning
	// only exists to keep the auto-download SHA verifiable.
	target := strings.TrimPrefix(targetMiseVersion, "v")
	fallback := strings.TrimPrefix(fallbackMiseVersion, "v")
	for _, f := range strings.Fields(verStr) {
		if f == target || f == fallback {
			return nil
		}
	}
	log.Printf("mise: existing binary at %s reports %q (expected %s or %s); using it as-is",
		miseBinPath, verStr, target, fallback)
	return nil
}

func downloadMise(version, expectedSha256 string) error {
	urls := []string{
		fmt.Sprintf("https://github.com/jdx/mise/releases/download/%s/mise-%s-linux-x64", version, version),
		fmt.Sprintf("https://mirror.ghproxy.com/https://github.com/jdx/mise/releases/download/%s/mise-%s-linux-x64", version, version),
	}

	var lastErr error
	for _, dlUrl := range urls {
		log.Printf("Downloading mise from %s", dlUrl)

		// Create tmp file in the SAME directory as the final target so the
		// atomic rename below stays within one filesystem. Defaulting to
		// /tmp blows up with "invalid cross-device link" on hosts where
		// /tmp is tmpfs and /usr/local/bin is on the root fs.
		tmpFile, err := os.CreateTemp(filepath.Dir(miseBinPath), "mise-download-*.tmp")
		if err != nil {
			return err
		}
		tmpPath := tmpFile.Name()
		tmpFile.Close()

		defer os.Remove(tmpPath)

		err = downloadFile(tmpPath, dlUrl, expectedSha256)
		if err == nil {
			if err := os.Chmod(tmpPath, 0755); err != nil {
				return err
			}
			return os.Rename(tmpPath, miseBinPath)
		}
		log.Printf("Download failed from %s: %v", dlUrl, err)
		lastErr = err
	}
	return lastErr
}

func downloadFile(filepath string, url string, expectedSha256 string) error {
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad status: %s", resp.Status)
	}

	hasher := sha256.New()
	out, err := os.Create(filepath)
	if err != nil {
		return err
	}
	defer out.Close()

	writer := io.MultiWriter(out, hasher)
	if _, err = io.Copy(writer, resp.Body); err != nil {
		return err
	}

	if hex.EncodeToString(hasher.Sum(nil)) != expectedSha256 {
		return fmt.Errorf("checksum mismatch")
	}
	return nil
}

func setupMiseEnv() error {
	if err := os.MkdirAll(miseDataDir, 0755); err != nil {
		return err
	}

	// Create the shims directory ahead of time if it doesn't exist to prevent PATH warnings
	shimsDir := filepath.Join(miseDataDir, "shims")
	if err := os.MkdirAll(shimsDir, 0755); err != nil {
		return err
	}

	content := fmt.Sprintf("export PATH=\"%s:$PATH\"\n", shimsDir)

	b, err := os.ReadFile(miseProfilePath)
	if err == nil && string(b) == content {
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(miseProfilePath), 0755); err != nil {
		return err
	}

	return os.WriteFile(miseProfilePath, []byte(content), 0644)
}
