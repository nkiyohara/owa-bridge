package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNormalize(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	nested := filepath.Join(root, "nested")
	if err := os.Mkdir(nested, 0o750); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(nested, "LICENSE")
	if err := os.WriteFile(file, []byte("license\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, time.July, 17, 12, 34, 56, 0, time.UTC)
	if err := normalize(root, want); err != nil {
		t.Fatalf("normalize() error = %v", err)
	}
	for _, path := range []string{root, nested, file} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if !info.ModTime().Equal(want) {
			t.Errorf("%s mtime = %s, want %s", path, info.ModTime(), want)
		}
	}
}

func TestNormalizeRejectsSymlink(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	target := filepath.Join(root, "target")
	if err := os.WriteFile(target, []byte("license\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(root, "link")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	err := normalize(root, time.Now())
	if err == nil || !strings.Contains(err.Error(), "refusing symlink") {
		t.Fatalf("normalize() error = %v, want symlink rejection", err)
	}
}

func TestValidateRoot(t *testing.T) {
	t.Parallel()

	for _, root := range []string{"", ".", "..", "../licenses", filepath.Join(string(filepath.Separator), "tmp", "licenses")} {
		if err := validateRoot(root); err == nil {
			t.Errorf("validateRoot(%q) unexpectedly succeeded", root)
		}
	}
	if err := validateRoot(filepath.Join(".release", "third_party_licenses")); err != nil {
		t.Fatalf("validateRoot() rejected generated path: %v", err)
	}
}
