package cron

import (
	"strings"
	"testing"
)

func TestWrapWithMiseExec_BareCommand(t *testing.T) {
	got, err := wrapWithMiseExec("node", "20.11.0", "node app.js")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := `MISE_DATA_DIR=/var/lib/easyserver/mise /usr/local/bin/mise exec node@20.11.0 -- node app.js`
	if got != want {
		t.Fatalf("\nwant: %q\n got: %q", want, got)
	}
}

func TestWrapWithMiseExec_VfoxTool(t *testing.T) {
	// Java goes through vfox; tool name has ':' and '/'. cron lines are shell-
	// parsed, so a bare token with these chars is one argv element — fine.
	got, err := wrapWithMiseExec("java", "21.0.0", "java -jar app.jar")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(got, "vfox:version-fox/vfox-java@21.0.0") {
		t.Fatalf("expected vfox tool spec in output, got: %s", got)
	}
	if !strings.HasSuffix(got, " -- java -jar app.jar") {
		t.Fatalf("expected trailing user command, got: %s", got)
	}
}

func TestWrapWithMiseExec_UnsupportedLang(t *testing.T) {
	_, err := wrapWithMiseExec("rust", "1.80.0", "cargo run")
	if err == nil {
		t.Fatal("expected error for unsupported lang, got nil")
	}
}

// AC2 regression: must NOT prepend `bash -lc` or `~/.bashrc` to the line.
// User commands containing those substrings are user choice; we only check
// that wrapWithMiseExec itself does not inject them.
func TestWrapWithMiseExec_NoLoginShell(t *testing.T) {
	got, _ := wrapWithMiseExec("node", "20.11.0", "node app.js")
	for _, badPrefix := range []string{"bash -lc", "bash -l ", "sh -l"} {
		if strings.HasPrefix(strings.TrimSpace(strings.SplitN(got, " -- ", 2)[0]), badPrefix) {
			t.Fatalf("wrap injected forbidden login shell %q: %s", badPrefix, got)
		}
	}
}

// cron(8) `%` regression: user commands like `date +%Y%m%d` must survive into
// the crontab line as `\%Y\%m\%d`, otherwise cron truncates the command and
// feeds the tail as stdin.
func TestWrapWithMiseExec_EscapesPercent(t *testing.T) {
	got, err := wrapWithMiseExec("node", "20.11.0", `echo $(date +%Y%m%d) >> /tmp/r.log`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(got, "+%Y") {
		t.Fatalf("bare %% survived into crontab line, cron would truncate:\n%s", got)
	}
	if !strings.Contains(got, `+\%Y\%m\%d`) {
		t.Fatalf("expected escaped \\%% sequences, got:\n%s", got)
	}
}
