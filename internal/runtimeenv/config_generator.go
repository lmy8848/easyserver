package runtimeenv

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const miseConfigPath = "/etc/mise/config.toml"

// buildMiseConfigContent renders the textual content of /etc/mise/config.toml
// from the given enabled mirrors and global defaults. Pure function — no I/O —
// so its output can be asserted in tests without filesystem fakes.
//
// Layout:
//
//	[env]
//	MISE_NODE_MIRROR_URL = "..."
//
//	[tools]
//	node = "20.11.0"
//	"vfox:version-fox/vfox-java" = "21.0.0"
//
// Tool keys that contain ':' or '/' are TOML-quoted; bare identifiers are not,
// to keep the file readable for the common node/python/go cases.
func buildMiseConfigContent(mirrors []RuntimeMirror, defaults []GlobalDefaultEntry) string {
	var buf bytes.Buffer

	buf.WriteString("[env]\n")
	for _, m := range mirrors {
		if m.Enabled == 1 {
			// %q on both key and value: TOML basic strings overlap with Go
			// quoted strings for the chars EnvKey/EnvValue can plausibly hold.
			// Belt-and-suspenders with the CreateMirror regex check — even if
			// an unsanitized key sneaks in (e.g. via direct DB write), it
			// can't break out of its quoted form to forge a new section.
			buf.WriteString(fmt.Sprintf("%q = %q\n", m.EnvKey, m.EnvValue))
		}
	}

	if len(defaults) > 0 {
		buf.WriteString("\n[tools]\n")
		for _, d := range defaults {
			tool, ok := MiseToolFor(d.Lang)
			if !ok {
				// Skip langs that aren't in the catalog; FK + CHECK should make
				// this unreachable, but stay defensive.
				continue
			}
			if strings.ContainsAny(tool, ":/") {
				buf.WriteString(fmt.Sprintf("%q = %q\n", tool, d.Exact))
			} else {
				buf.WriteString(fmt.Sprintf("%s = %q\n", tool, d.Exact))
			}
		}
	}

	return buf.String()
}

// GenerateMiseConfig regenerates /etc/mise/config.toml from the current DB state
// of mirrors and global defaults. Atomic write: temp file + rename.
func (s *Service) GenerateMiseConfig(ctx context.Context) error {
	mirrors, err := s.repo.ListMirrors(ctx)
	if err != nil {
		return err
	}
	defaults, err := s.repo.ListDefaults(ctx)
	if err != nil {
		return err
	}

	content := buildMiseConfigContent(mirrors, defaults)

	dir := filepath.Dir(miseConfigPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	tmpFile, err := os.CreateTemp(dir, "config.toml.*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmpFile.Name()
	defer os.Remove(tmpName)

	if _, err := tmpFile.Write([]byte(content)); err != nil {
		tmpFile.Close()
		return err
	}
	if err := tmpFile.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, 0644); err != nil {
		return err
	}
	return os.Rename(tmpName, miseConfigPath)
}
