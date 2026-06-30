package runtimeenv

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"easyserver/internal/envconfig"
)

const miseConfigPath = "/etc/mise/config.toml"

// buildMiseConfigContent renders the textual content of /etc/mise/config.toml
// from the given enabled env configs and global defaults. Pure function — no
// I/O — so its output can be asserted in tests without filesystem fakes.
//
// Layout:
//
//	[env]
//	MISE_NODE_MIRROR_URL = "..."
//	JAVA_HOME = "..."
//
//	[tools]
//	node = "20.11.0"
//	"vfox:version-fox/vfox-java" = "21.0.0"
//
// Tool keys that contain ':' or '/' are TOML-quoted; bare identifiers are not,
// to keep the file readable for the common node/python/go cases.
func buildMiseConfigContent(envConfigs []envconfig.EnvConfig, defaults []GlobalDefaultEntry) string {
	var buf bytes.Buffer

	buf.WriteString("[env]\n")
	for _, c := range envConfigs {
		if !c.Enabled {
			continue
		}
		// %q on both key and value: TOML basic strings overlap with Go
		// quoted strings for the chars Name/Value can plausibly hold.
		// Belt-and-suspenders with isValidEnvName — even if an unsanitized
		// key sneaks in (e.g. via direct DB write), it can't break out of
		// its quoted form to forge a new section.
		buf.WriteString(fmt.Sprintf("%q = %q\n", c.Name, c.Value))
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

// GenerateMiseConfig regenerates /etc/mise/config.toml from the current DB
// state of env configs and global defaults. Atomic write: temp file + rename.
func (s *Service) GenerateMiseConfig(ctx context.Context) error {
	envConfigs, err := s.envConfigs.ListEnvConfigs(ctx)
	if err != nil {
		return err
	}
	defaults, err := s.repo.ListDefaults(ctx)
	if err != nil {
		return err
	}

	content := buildMiseConfigContent(envConfigs, defaults)

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
