package avatar

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestResolveConfigSelectsNewestOneLevelCandidate(t *testing.T) {
	root := filepath.Join(t.TempDir(), "OSC")
	avatarID := "avtr_demo"
	older := writeAvatarConfig(t, root, "usr_a", avatarID)
	newer := writeAvatarConfig(t, root, "usr_b", avatarID)
	nested := filepath.Join(root, "usr_a", "nested", "Avatars", avatarID+".json")
	writeFile(t, nested)

	oldTime := time.Date(2026, time.August, 20, 12, 0, 0, 0, time.UTC)
	newTime := oldTime.Add(time.Minute)
	setModTime(t, older, oldTime)
	setModTime(t, newer, newTime)
	setModTime(t, nested, newTime.Add(time.Minute))

	resolved, err := resolveConfig(root, "", avatarID)
	if err != nil {
		t.Fatalf("resolveConfig() error = %v", err)
	}
	if want := mustAbsoluteCleanPath(t, newer); resolved.path != want {
		t.Fatalf("resolved path = %q, want newest one-level candidate %q", resolved.path, want)
	}
	if resolved.source != SourceAvatarConfig || !resolved.requireIDMatch {
		t.Fatalf("resolved metadata = %#v, want avatar config with ID match required", resolved)
	}

	setModTime(t, older, oldTime)
	setModTime(t, newer, oldTime)
	resolved, err = resolveConfig(root, "", avatarID)
	if err != nil {
		t.Fatalf("resolveConfig() equal-time error = %v", err)
	}
	want := mustAbsoluteCleanPath(t, older)
	if alternate := mustAbsoluteCleanPath(t, newer); alternate < want {
		want = alternate
	}
	if resolved.path != want {
		t.Fatalf("resolved equal-time path = %q, want lexically first %q", resolved.path, want)
	}
}

func TestValidateAvatarIDRejectsUnsafeValues(t *testing.T) {
	invalidIDs := []string{
		"",
		".",
		"..",
		"/",
		"\\",
		":",
		"\x00",
		strings.Repeat("a", maxAvatarIDBytes+1),
	}
	for _, avatarID := range invalidIDs {
		t.Run("invalid", func(t *testing.T) {
			if err := validateAvatarID(avatarID); !errors.Is(err, ErrInvalidAvatarID) {
				t.Fatalf("validateAvatarID(%q) error = %v, want ErrInvalidAvatarID", avatarID, err)
			}
		})
	}

	if err := validateAvatarID("local_test_avatar"); err != nil {
		t.Fatalf("validateAvatarID(local test ID) error = %v", err)
	}
}

func TestValidateAvatarIDRejectsWindowsReservedCharacters(t *testing.T) {
	for _, avatarID := range []string{"<", ">", ":", "\"", "/", "\\", "|", "?", "*"} {
		t.Run(avatarID, func(t *testing.T) {
			if err := validateAvatarID(avatarID); !errors.Is(err, ErrInvalidAvatarID) {
				t.Fatalf("validateAvatarID(%q) error = %v, want ErrInvalidAvatarID", avatarID, err)
			}
		})
	}

	if err := validateAvatarID("local_test_avatar"); err != nil {
		t.Fatalf("validateAvatarID(local test ID) error = %v", err)
	}
}

func TestResolveConfigUsesFallbackOnlyWhenAvatarMissing(t *testing.T) {
	avatarID := "avtr_demo"
	cases := []struct {
		name        string
		setup       func(t *testing.T, root string) string
		wantSource  Source
		wantIDMatch bool
		wantErr     error
		wantPath    bool
	}{
		{
			name: "current absent regular fallback",
			setup: func(t *testing.T, root string) string {
				return writeFile(t, filepath.Join(t.TempDir(), "fallback.json"))
			},
			wantSource: SourceFallback,
			wantPath:   true,
		},
		{
			name: "current absent empty fallback",
			setup: func(t *testing.T, root string) string {
				return ""
			},
			wantErr: ErrConfigNotFound,
		},
		{
			name: "current absent missing fallback",
			setup: func(t *testing.T, root string) string {
				return filepath.Join(t.TempDir(), "missing-fallback.json")
			},
			wantErr:  ErrConfigNotFound,
			wantPath: true,
		},
		{
			name: "current present regular fallback",
			setup: func(t *testing.T, root string) string {
				writeAvatarConfig(t, root, "usr_a", avatarID)
				return writeFile(t, filepath.Join(t.TempDir(), "fallback.json"))
			},
			wantSource:  SourceAvatarConfig,
			wantIDMatch: true,
		},
		{
			name: "current present directory fallback",
			setup: func(t *testing.T, root string) string {
				path := filepath.Join(root, "usr_a", "Avatars", avatarID+".json")
				if err := os.MkdirAll(path, 0o755); err != nil {
					t.Fatalf("MkdirAll(%q): %v", path, err)
				}
				return writeFile(t, filepath.Join(t.TempDir(), "fallback.json"))
			},
			wantErr: ErrInvalidConfigPath,
		},
		{
			name: "current present symlink fallback",
			setup: func(t *testing.T, root string) string {
				candidate := filepath.Join(root, "usr_a", "Avatars", avatarID+".json")
				if err := os.MkdirAll(filepath.Dir(candidate), 0o755); err != nil {
					t.Fatalf("MkdirAll(%q): %v", filepath.Dir(candidate), err)
				}
				target := writeFile(t, filepath.Join(t.TempDir(), "target.json"))
				if err := os.Symlink(target, candidate); err != nil {
					if isSymlinkPrivilegeError(err) {
						t.Skipf("symlink creation needs unavailable OS privilege: %v", err)
					}
					t.Fatalf("Symlink(%q, %q): %v", target, candidate, err)
				}
				return writeFile(t, filepath.Join(t.TempDir(), "fallback.json"))
			},
			wantErr: ErrInvalidConfigPath,
		},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "OSC")
			fallback := test.setup(t, root)

			resolved, err := resolveConfig(root, fallback, avatarID)
			if test.wantErr != nil {
				if !errors.Is(err, test.wantErr) {
					t.Fatalf("resolveConfig() error = %v, want errors.Is(_, %v)", err, test.wantErr)
				}
				if test.wantPath && !strings.Contains(err.Error(), fmt.Sprintf("%q", mustAbsoluteCleanPath(t, fallback))) {
					t.Fatalf("resolveConfig() error = %q, want fallback path context %q", err, fallback)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveConfig() error = %v", err)
			}
			if resolved.source != test.wantSource || resolved.requireIDMatch != test.wantIDMatch {
				t.Fatalf("resolved metadata = %#v, want source %v and requireIDMatch %t", resolved, test.wantSource, test.wantIDMatch)
			}
			if test.wantPath && resolved.path != mustAbsoluteCleanPath(t, fallback) {
				t.Fatalf("resolved path = %q, want %q", resolved.path, mustAbsoluteCleanPath(t, fallback))
			}
		})
	}
}

func TestResolveConfigRejectsLinkedDirectoryComponents(t *testing.T) {
	const avatarID = "avtr_demo"
	cases := []struct {
		name  string
		setup func(t *testing.T, root string)
	}{
		{
			name: "linked user directory",
			setup: func(t *testing.T, root string) {
				target := filepath.Join(t.TempDir(), "external-user")
				writeFile(t, filepath.Join(target, "Avatars", avatarID+".json"))
				if err := os.MkdirAll(root, 0o755); err != nil {
					t.Fatalf("MkdirAll(%q): %v", root, err)
				}
				if err := os.Symlink(target, filepath.Join(root, "usr_a")); err != nil {
					if isSymlinkPrivilegeError(err) {
						t.Skipf("symlink creation needs unavailable OS privilege: %v", err)
					}
					t.Fatalf("Symlink(%q, user directory): %v", target, err)
				}
			},
		},
		{
			name: "linked Avatars directory",
			setup: func(t *testing.T, root string) {
				target := filepath.Join(t.TempDir(), "external-avatars")
				writeFile(t, filepath.Join(target, avatarID+".json"))
				userDir := filepath.Join(root, "usr_a")
				if err := os.MkdirAll(userDir, 0o755); err != nil {
					t.Fatalf("MkdirAll(%q): %v", userDir, err)
				}
				if err := os.Symlink(target, filepath.Join(userDir, "Avatars")); err != nil {
					if isSymlinkPrivilegeError(err) {
						t.Skipf("symlink creation needs unavailable OS privilege: %v", err)
					}
					t.Fatalf("Symlink(%q, Avatars directory): %v", target, err)
				}
			},
		},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "OSC")
			test.setup(t, root)
			fallback := writeFile(t, filepath.Join(t.TempDir(), "fallback.json"))

			_, err := resolveConfig(root, fallback, avatarID)
			if !errors.Is(err, ErrInvalidConfigPath) {
				t.Fatalf("resolveConfig() error = %v, want ErrInvalidConfigPath without fallback", err)
			}
		})
	}
}

func TestConfigCandidateOrderingPreservesTimeRange(t *testing.T) {
	latest := configCandidate{
		path:    "z.json",
		modTime: time.Date(9999, time.December, 31, 23, 59, 59, 0, time.UTC),
	}
	earliest := configCandidate{
		path:    "a.json",
		modTime: time.Date(1, time.January, 1, 0, 0, 0, 0, time.UTC),
	}
	if !configCandidateComesBefore(latest, earliest) {
		t.Fatal("latest representable time must sort before earliest representable time")
	}
	if configCandidateComesBefore(earliest, latest) {
		t.Fatal("earliest representable time must not sort before latest representable time")
	}

	tie := latest
	tie.path = "a.json"
	if !configCandidateComesBefore(tie, latest) {
		t.Fatal("equal timestamps must sort by ascending path")
	}
}

func writeAvatarConfig(t *testing.T, root, userID, avatarID string) string {
	t.Helper()
	return writeFile(t, filepath.Join(root, userID, "Avatars", avatarID+".json"))
}

func writeFile(t *testing.T, path string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q): %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte("{}"), 0o600); err != nil {
		t.Fatalf("WriteFile(%q): %v", path, err)
	}
	return path
}

func setModTime(t *testing.T, path string, value time.Time) {
	t.Helper()
	if err := os.Chtimes(path, value, value); err != nil {
		t.Fatalf("Chtimes(%q): %v", path, err)
	}
}

func mustAbsoluteCleanPath(t *testing.T, path string) string {
	t.Helper()
	abs, err := filepath.Abs(path)
	if err != nil {
		t.Fatalf("Abs(%q): %v", path, err)
	}
	return filepath.Clean(abs)
}

func isSymlinkPrivilegeError(err error) bool {
	return errors.Is(err, fs.ErrPermission) || errors.Is(err, syscall.Errno(1314))
}
