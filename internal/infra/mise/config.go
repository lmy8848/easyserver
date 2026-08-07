package mise

import (
	"bytes"
	"fmt"
	"strings"
)

// BuildConfigContent renders the textual content of the panel-private mise
// config.toml (/opt/easyserver/mise/config.toml) from env vars and default
// versions. Pure function — no I/O — so its output can be asserted in tests.
//
// envs:    启用中的环境变量（如镜像 MISE_NODE_MIRROR_URL），key 已按底层标识
//
//	（mise tool 等）规整，交由底层写入。
//
// defaults:lang 的默认版本，key 为底层标识（如 "node" / "vfox:version-fox/vfox-java"）。
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
func BuildConfigContent(envs, defaults map[string]string) string {
	var buf bytes.Buffer

	buf.WriteString("[env]\n")
	for name, value := range envs {
		// %q on both key and value: TOML basic strings overlap with Go
		// quoted strings for the chars Name/Value can plausibly hold.
		// Belt-and-suspenders — even if an unsanitized key sneaks in, it
		// can't break out of its quoted form to forge a new section.
		buf.WriteString(fmt.Sprintf("%q = %q\n", name, value))
	}

	if len(defaults) > 0 {
		buf.WriteString("\n[tools]\n")
		for tool, exact := range defaults {
			if strings.ContainsAny(tool, ":/") {
				buf.WriteString(fmt.Sprintf("%q = %q\n", tool, exact))
			} else {
				buf.WriteString(fmt.Sprintf("%s = %q\n", tool, exact))
			}
		}
	}

	return buf.String()
}
