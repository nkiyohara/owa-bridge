package browser

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateOptions(t *testing.T) {
	t.Parallel()

	valid := Options{
		Origin:     "https://outlook.cloud.microsoft",
		ProfileDir: filepath.Join(t.TempDir(), "profile"),
	}
	if err := validateOptions(valid); err != nil {
		t.Fatalf("validateOptions() error = %v", err)
	}

	tests := []Options{
		{},
		{Origin: "http://outlook.example", ProfileDir: valid.ProfileDir},
		{Origin: valid.Origin, ProfileDir: "relative/profile"},
		{Origin: valid.Origin, ProfileDir: valid.ProfileDir, Executable: "chrome\n--flag"},
	}
	for _, options := range tests {
		if err := validateOptions(options); err == nil {
			t.Fatalf("validateOptions(%+v) unexpectedly succeeded", options)
		}
	}
}

func TestResolveExecutableUsesExactConfiguredPath(t *testing.T) {
	t.Parallel()

	executable := filepath.Join(t.TempDir(), "test-chromium")
	// #nosec G306 -- the owner-only test fixture must be executable.
	if err := os.WriteFile(executable, []byte("synthetic executable"), 0o700); err != nil {
		t.Fatal(err)
	}
	resolved, err := ResolveExecutable(executable)
	if err != nil {
		t.Fatalf("ResolveExecutable() error = %v", err)
	}
	if resolved != executable {
		t.Fatalf("ResolveExecutable() = %q, want %q", resolved, executable)
	}
}

func TestResolveExecutableDoesNotFallbackFromExplicitPath(t *testing.T) {
	t.Parallel()

	_, err := ResolveExecutable(filepath.Join(t.TempDir(), "missing-chromium"))
	if err == nil {
		t.Fatal("ResolveExecutable() unexpectedly accepted a missing explicit path")
	}
}
