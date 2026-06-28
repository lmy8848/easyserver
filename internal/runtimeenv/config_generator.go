package runtimeenv

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
)

const miseConfigPath = "/etc/mise/config.toml"

// GenerateMiseConfig generates the /etc/mise/config.toml based on mirrors and global defaults
func (s *Service) GenerateMiseConfig(ctx context.Context) error {
	mirrors, err := s.repo.ListMirrors(ctx)
	if err != nil {
		return err
	}

	var buf bytes.Buffer
	buf.WriteString("[env]\n")

	for _, m := range mirrors {
		if m.Enabled == 1 {
			buf.WriteString(fmt.Sprintf("%s = \"%s\"\n", m.EnvKey, m.EnvValue))
		}
	}

	// Make sure the directory exists
	dir := filepath.Dir(miseConfigPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	// Write atomically using CreateTemp
	tmpFile, err := os.CreateTemp(dir, "config.toml.*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmpFile.Name()
	defer os.Remove(tmpName)

	if _, err := tmpFile.Write(buf.Bytes()); err != nil {
		tmpFile.Close()
		return err
	}
	if err := tmpFile.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, 0644); err != nil {
		return err
	}

	if err := os.Rename(tmpName, miseConfigPath); err != nil {
		return err
	}

	return nil
}
