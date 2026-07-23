package projectstatus

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSourceFingerprintIgnoresGeneratedStatus(t *testing.T) {
	root := t.TempDir()
	writeFingerprintFile(t, root, "main.go", "package main\n")
	writeFingerprintFile(t, root, "docs/project/status.md", "first\n")
	first, err := SourceFingerprint(root)
	if err != nil {
		t.Fatal(err)
	}
	writeFingerprintFile(t, root, "docs/project/status.md", "second\n")
	second, err := SourceFingerprint(root)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("status file changed fingerprint: %s != %s", first, second)
	}
	writeFingerprintFile(t, root, "main.go", "package changed\n")
	third, err := SourceFingerprint(root)
	if err != nil {
		t.Fatal(err)
	}
	if third == second {
		t.Fatal("source change did not alter fingerprint")
	}
}

func TestSourceFingerprintIgnoresSuperpowersScratchData(t *testing.T) {
	for _, scratch := range []string{
		".superpowers/sdd/progress.md",
		".task-gocache/cache-entry",
	} {
		t.Run(scratch, func(t *testing.T) {
			root := t.TempDir()
			writeFingerprintFile(t, root, "main.go", "package main\n")
			writeFingerprintFile(t, root, scratch, "first\n")

			first, err := SourceFingerprint(root)
			if err != nil {
				t.Fatal(err)
			}
			writeFingerprintFile(t, root, scratch, "second\n")
			second, err := SourceFingerprint(root)
			if err != nil {
				t.Fatal(err)
			}
			if first != second {
				t.Fatalf("scratch data changed fingerprint: %s != %s", first, second)
			}
		})
	}
}

func writeFingerprintFile(t *testing.T, root, relative, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
