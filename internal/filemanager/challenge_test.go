package filemanager

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// PoC-1: ReadContent has NO O_NOFOLLOW. Once an attacker plants a symlink to
// outside the sandbox INSIDE the sandbox, ValidatePath does block it via
// EvalSymlinks. But what if the symlink target is *itself a path inside
// basePath* via an absolute prefix that just happens to resolve outward
// through filesystem semantics? Try: symlink whose target is the basePath
// directory plus "/../../etc/passwd" — i.e. target string contains traversal.
// EvalSymlinks should still resolve it correctly. Check we actually block.
func TestChallenge_ReadContent_SymlinkWithTraversalTarget(t *testing.T) {
	base := t.TempDir()
	outside := filepath.Join(t.TempDir(), "loot.txt")
	if err := os.WriteFile(outside, []byte("LOOT"), 0644); err != nil {
		t.Fatal(err)
	}
	// Make target a path that goes via basePath then traverses out
	target := filepath.Join(base, "..", filepath.Base(filepath.Dir(outside)), "loot.txt")
	if err := os.Symlink(target, filepath.Join(base, "trick")); err != nil {
		t.Fatal(err)
	}
	m, _ := NewManager(base)
	c, err := m.ReadContent("/trick")
	if err == nil {
		t.Errorf("VULN: ReadContent leaked symlink-out content: %q", c.Content)
	}
}

// PoC-2: Copy(src,...) uses os.Open(validSrc) — no O_NOFOLLOW. Same shape as
// ReadContent: ValidatePath resolves symlinks and should block beforehand.
// Sanity-check the block still holds and there's no leak path.
func TestChallenge_Copy_SrcSymlinkOut(t *testing.T) {
	base := t.TempDir()
	outside := filepath.Join(t.TempDir(), "secret.txt")
	if err := os.WriteFile(outside, []byte("SECRET-XYZ"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(base, "src")); err != nil {
		t.Fatal(err)
	}
	m, _ := NewManager(base)
	err := m.Copy("/src", "/dst")
	if err == nil {
		// Check destination doesn't contain the secret
		data, _ := os.ReadFile(filepath.Join(base, "dst"))
		if strings.Contains(string(data), "SECRET-XYZ") {
			t.Errorf("VULN: Copy followed symlink and leaked: %q", data)
		} else {
			t.Errorf("VULN-soft: Copy succeeded with symlink src (should reject), no leak: %q", data)
		}
	}
}

// PoC-3: Rename source path is a symlink that points outside. ValidatePath on
// /src resolves and rejects -> good. Confirm.
func TestChallenge_Rename_SymlinkSourceOut(t *testing.T) {
	base := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.txt")
	os.WriteFile(outside, []byte("OUTSIDE"), 0644)
	os.Symlink(outside, filepath.Join(base, "link"))

	m, _ := NewManager(base)
	if err := m.Rename("/link", "/renamed"); err == nil {
		// Did we move the OUTSIDE file?
		if _, err := os.Stat(outside); os.IsNotExist(err) {
			t.Errorf("VULN: Rename followed symlink and moved an outside file!")
		}
	}
}

// PoC-4: Compress with a self-referential destination — sources=["/"]
// destPath="/out". The zip file is being written into the sandbox while Walk
// is scanning the same sandbox. Does it recursively include itself and
// explode disk? Bound = limitWriter for extract but Compress has NO size cap.
func TestChallenge_Compress_SelfInclusion(t *testing.T) {
	base := t.TempDir()
	// Drop a 1 MB file so the zip has nontrivial content
	os.WriteFile(filepath.Join(base, "big.bin"), bytes.Repeat([]byte{0}, 1<<20), 0644)

	m, _ := NewManager(base)
	if err := m.Compress([]string{"/"}, "/snapshot"); err != nil {
		t.Logf("Compress err (might be expected): %v", err)
		return
	}
	info, _ := os.Stat(filepath.Join(base, "snapshot.zip"))
	if info != nil && info.Size() > 100*1024*1024 {
		t.Errorf("VULN: Compress self-included and produced an outsized archive: %d bytes", info.Size())
	}
	// Also: was the zip itself walked and included as an entry?
	zr, err := zip.OpenReader(filepath.Join(base, "snapshot.zip"))
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	for _, f := range zr.File {
		if strings.Contains(f.Name, "snapshot.zip") {
			t.Logf("Self-inclusion: %s (%d bytes compressed, %d bytes uncompressed)",
				f.Name, f.CompressedSize64, f.UncompressedSize64)
		}
	}
}

// PoC-5: ValidatePath with pathological inputs.
func TestChallenge_ValidatePath_Pathological(t *testing.T) {
	base := t.TempDir()
	m, _ := NewManager(base)

	// 5a: null byte
	if _, err := m.ValidatePath("\x00"); err == nil {
		t.Error("VULN: null byte accepted")
	}
	// 5b: huge / chain
	if _, err := m.ValidatePath(strings.Repeat("/", 4096)); err != nil {
		t.Logf("4096 slashes: %v", err)
	}
	// 5c: absolute /etc/passwd
	v, err := m.ValidatePath("/etc/passwd")
	if err != nil {
		t.Errorf("/etc/passwd should map to <base>/etc/passwd, got err: %v", err)
	} else if !strings.HasPrefix(v, base) {
		t.Errorf("VULN: /etc/passwd escaped: %s", v)
	}
	// 5d: lots of ../
	v, err = m.ValidatePath(strings.Repeat("../", 50) + "etc/passwd")
	if err != nil {
		t.Logf("../*50/etc/passwd → %v", err)
	} else if !strings.HasPrefix(v, base) {
		t.Errorf("VULN: traversal escaped: %s", v)
	}
}

// PoC-6: Extract with archive containing absolute-symlink-ish names.
// "Symlinks not allowed" check uses file.Mode()&os.ModeSymlink. What if the
// zip header lies (regular file Mode) but contents reference a symlink-like
// path "..". The string check `strings.Contains(file.Name, "..")` should
// catch any name with literal "..". But what about Unicode confusables like
// "．．" (fullwidth)? They don't expand via filepath.Clean. Probe a different
// vector: a name that's just "." -> filepath.Join(dest, ".") = dest. That
// would attempt to create dest as a file (since IsDir() depends on header).
func TestChallenge_Extract_DotEntry(t *testing.T) {
	base := t.TempDir()
	zipPath := filepath.Join(base, "weird.zip")
	zf, _ := os.Create(zipPath)
	zw := zip.NewWriter(zf)
	w, _ := zw.Create(".") // single-dot entry, NOT a dir (no trailing /)
	w.Write([]byte("PWN"))
	zw.Close()
	zf.Close()

	m, _ := NewManager(base)
	err := m.Extract("/weird.zip", "/")
	t.Logf("Extract single-dot entry err: %v", err)
	// The check `isSubPath(destPath, validPath)` should pass since dest==validPath.
	// We'd then OpenFile on destPath itself — which is a directory → fail.
	// Verify destination wasn't replaced/corrupted.
	if info, err := os.Stat(base); err != nil || !info.IsDir() {
		t.Errorf("VULN: extracting '.' entry damaged sandbox root")
	}
}

// PoC-7: Extract with an entry whose name is just ".." (parent), no slash.
// Walk's strings.Contains check should catch any "..". Verify.
func TestChallenge_Extract_BareDotDot(t *testing.T) {
	base := t.TempDir()
	zipPath := filepath.Join(base, "dotdot.zip")
	zf, _ := os.Create(zipPath)
	zw := zip.NewWriter(zf)
	w, _ := zw.Create("..")
	w.Write([]byte("ESCAPE"))
	zw.Close()
	zf.Close()

	m, _ := NewManager(base)
	err := m.Extract("/dotdot.zip", "/")
	if err == nil {
		t.Errorf("VULN: bare '..' entry extracted without error")
	}
}

// PoC-8: extractGzip's basename. gzPath is validated, but if a user puts a
// gzip file named "foo.gz" inside sandbox at sub/foo.gz and extracts to "/",
// outPath = base/foo (basename strip). Now: what if the gzPath validated to
// a path that ends in something like "tricky"? filepath.Base of that yields
// "tricky" (no .gz suffix), TrimSuffix is a no-op, then the fallback kicks
// in: baseName = "tricky.extracted". Verify that no path manipulation
// escapes. Probe gzip with no .gz suffix.
func TestChallenge_ExtractGzip_NoGzSuffix(t *testing.T) {
	base := t.TempDir()
	// Make a "raw" gzip file with no .gz suffix
	gzPath := filepath.Join(base, "blob")
	// We can't even reach extractGzip without .gz/.tgz/.zip routing — Extract
	// switches on the extension. So this path is unreachable. Document it.
	_ = gzPath
	m, _ := NewManager(base)
	os.WriteFile(filepath.Join(base, "blob"), []byte("not gzip"), 0644)
	err := m.Extract("/blob", "/")
	if err == nil {
		t.Error("VULN: Extract accepted blob with no archive extension")
	}
}

// PoC-9: extractZip's per-entry NOFOLLOW. If basePath has a pre-existing
// symlink at <dest>/<entryDir> pointing OUTSIDE, mkdirAllWithRecord calls
// os.Stat (follows symlink) — if outside dir already exists, the Stat
// succeeds, treats it as an existing dir, and proceeds to OpenFile under it
// with NOFOLLOW. NOFOLLOW only protects the *last* component. The
// intermediate symlinked dir lets writes go OUTSIDE the sandbox!
func TestChallenge_Extract_IntermediateSymlinkEscape(t *testing.T) {
	base := t.TempDir()
	outside := t.TempDir() // attacker-controlled writable dir outside sandbox
	// Plant a symlink: <base>/escape -> <outside>
	if err := os.Symlink(outside, filepath.Join(base, "escape")); err != nil {
		t.Fatal(err)
	}

	// Build a zip whose entry name is "escape/pwned"
	zipPath := filepath.Join(base, "evil.zip")
	zf, _ := os.Create(zipPath)
	zw := zip.NewWriter(zf)
	w, _ := zw.Create("escape/pwned")
	io.Copy(w, strings.NewReader("OWNED-VIA-INTERMEDIATE-SYMLINK"))
	zw.Close()
	zf.Close()

	m, _ := NewManager(base)
	err := m.Extract("/evil.zip", "/")
	t.Logf("Extract err: %v", err)

	// Check whether "OWNED-..." landed in <outside>/pwned
	want := filepath.Join(outside, "pwned")
	if data, err := os.ReadFile(want); err == nil {
		t.Errorf("VULN: extractZip wrote through intermediate symlink to %s: %q", want, data)
	}
}

// PoC-10: Same shape for tar.gz (extractTarGz).
func TestChallenge_ExtractTarGz_IntermediateSymlinkEscape(t *testing.T) {
	base := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(base, "escape")); err != nil {
		t.Fatal(err)
	}

	// Build tar.gz with entry "escape/pwned"
	tgz := createTarGz(t,
		map[string]string{"escape/pwned": "OWNED-VIA-TAR-INTERMEDIATE"},
		nil, nil)
	dst := filepath.Join(base, "evil.tar.gz")
	if data, err := os.ReadFile(tgz); err == nil {
		os.WriteFile(dst, data, 0644)
	}

	m, _ := NewManager(base)
	err := m.Extract("/evil.tar.gz", "/")
	t.Logf("Extract tar.gz err: %v", err)

	want := filepath.Join(outside, "pwned")
	if data, err := os.ReadFile(want); err == nil {
		t.Errorf("VULN: extractTarGz wrote through intermediate symlink: %q", data)
	}
}

// PoC-11: Upload with a path whose parent component is a pre-existing
// symlink-out. Manager.Upload calls ValidatePath which uses EvalSymlinks.
// EvalSymlinks resolves the symlink first → escapes basePath → reject. Good.
// But what if only a *leaf parent* of a longer path is the symlink and the
// final component doesn't exist yet (so EvalSymlinks climbs to the existing
// parent)? validatePath's loop:
//
//	absPath = base/escape/newfile
//	checkPath = absPath; EvalSymlinks(absPath) -> ENOENT (file doesn't exist)
//	climb to base/escape -> EvalSymlinks resolves to <outside>
//	rel = "newfile"
//	resolvedPath = <outside>/newfile
//	isSubPath(base, <outside>/newfile) → false → REJECT
//
// So far so good. But: write a regression that confirms.
func TestChallenge_Upload_IntermediateSymlinkRejected(t *testing.T) {
	base := t.TempDir()
	outside := t.TempDir()
	os.Symlink(outside, filepath.Join(base, "escape"))

	m, _ := NewManager(base)
	_, err := m.Upload(strings.NewReader("payload"), "/escape/newfile")
	if err == nil {
		// Did anything land in outside?
		if data, err := os.ReadFile(filepath.Join(outside, "newfile")); err == nil {
			t.Errorf("VULN: Upload via intermediate symlink leaked: %q", data)
		}
	}
}
