package mise

import (
	"bytes"
	"fmt"
)

// BuildConfigContent renders the textual content of the panel-private mise
// config.toml (/opt/easyserver/mise/config.toml) from env vars. Pure function —
// no I/O — so its output can be asserted in tests.
//
// envs: 启用中的环境变量（如镜像 MISE_NODE_MIRROR_URL）。
//
// Layout:
//
//	[env]
//	MISE_NODE_MIRROR_URL = "..."
func BuildConfigContent(envs map[string]string) string {
	var buf bytes.Buffer

	buf.WriteString("[env]\n")
	for name, value := range envs {
		// %q on both key and value: TOML basic strings overlap with Go
		// quoted strings for the chars Name/Value can plausibly hold.
		// Belt-and-suspenders — even if an unsanitized key sneaks in, it
		// can't break out of its quoted form to forge a new section.
		fmt.Fprintf(&buf, "%q = %q\n", name, value)
	}

	return buf.String()
}
