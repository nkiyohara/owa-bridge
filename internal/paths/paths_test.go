package paths

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nkiyohara/owa-bridge/internal/domain"
)

func TestStateDirByPlatform(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		goos      string
		home      string
		config    string
		cache     string
		xdg       string
		wantParts []string
	}{
		{"linux default", "linux", "/home/test", "/config", "/cache", "", []string{"home", "test", ".local", "state", "owa-bridge"}},
		{"linux XDG", "linux", "/home/test", "/config", "/cache", "/state", []string{"state", "owa-bridge"}},
		{"macOS", "darwin", "/Users/test", "/Users/test/Library/Application Support", "/cache", "", []string{"Application Support", "owa-bridge"}},
		{"Windows", "windows", `C:\Users\test`, `C:\Config`, `C:\Local`, "", []string{"Local", "owa-bridge"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := stateDir(test.goos, test.home, test.config, test.cache, nil, nil, test.xdg)
			if err != nil {
				t.Fatalf("stateDir() error = %v", err)
			}
			for _, part := range test.wantParts {
				if !strings.Contains(got, part) {
					t.Fatalf("stateDir() = %q, want part %q", got, part)
				}
			}
		})
	}
}

func TestStateDirRejectsInvalidInputs(t *testing.T) {
	t.Parallel()

	if _, err := stateDir("linux", "/home/test", "", "", nil, nil, "relative"); err == nil {
		t.Fatal("stateDir() unexpectedly accepted relative XDG_STATE_HOME")
	}
	if _, err := stateDir("darwin", "/Users/test", "", "", errors.New("missing"), nil, ""); err == nil {
		t.Fatal("stateDir() unexpectedly ignored config directory error")
	}
	if _, err := stateDir("windows", "", "", "", nil, errors.New("missing"), ""); err == nil {
		t.Fatal("stateDir() unexpectedly ignored cache directory error")
	}
}

func TestUpdateCachePathUsesPrivateStateTree(t *testing.T) {
	t.Setenv("OWA_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	path, err := UpdateCachePath()
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(path) != "latest.json" || filepath.Base(filepath.Dir(path)) != "updates" {
		t.Fatalf("UpdateCachePath() = %q", path)
	}
}

func TestProfileDirDoesNotContainAccountAlias(t *testing.T) {
	t.Setenv("OWA_STATE_DIR", t.TempDir())
	alias := "work/team"
	path, err := ProfileDir(domain.AccountID(alias))
	if err != nil {
		t.Fatalf("ProfileDir() error = %v", err)
	}
	if strings.Contains(path, alias) || filepath.Base(path) == "team" {
		t.Fatalf("ProfileDir() exposed alias as path: %q", path)
	}
}
