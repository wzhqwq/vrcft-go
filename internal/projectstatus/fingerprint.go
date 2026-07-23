package projectstatus

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func SourceFingerprint(root string) (string, error) {
	var files []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		if info.IsDir() && relative != "." && ignoredFingerprintDir(relative) {
			return filepath.SkipDir
		}
		if info.IsDir() || relative == "docs/project/status.md" {
			return nil
		}
		files = append(files, relative)
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(files)
	hash := sha256.New()
	for _, relative := range files {
		_, _ = io.WriteString(hash, relative+"\x00")
		file, err := os.Open(filepath.Join(root, filepath.FromSlash(relative)))
		if err != nil {
			return "", err
		}
		_, copyErr := io.Copy(hash, file)
		closeErr := file.Close()
		if copyErr != nil {
			return "", copyErr
		}
		if closeErr != nil {
			return "", closeErr
		}
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func ignoredFingerprintDir(relative string) bool {
	parts := strings.Split(relative, "/")
	for _, part := range parts {
		if strings.HasSuffix(part, "-gocache") {
			return true
		}
		switch part {
		case ".git", ".worktrees", ".superpowers", "node_modules", "dist", ".cache":
			return true
		}
	}
	return false
}
