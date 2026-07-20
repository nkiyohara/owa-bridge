package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderBindsCatalogsToChecksums(t *testing.T) {
	t.Parallel()

	dist := t.TempDir()
	metadataJSON := `{
  "tag": "v1.2.3",
  "version": "1.2.3",
  "commit": "0123456789abcdef0123456789abcdef01234567",
  "date": "2026-07-20T10:30:00Z"
}`
	checksums := strings.Join([]string{
		strings.Repeat("a", 64) + "  owa-bridge_1.2.3_source.tar.gz",
		strings.Repeat("b", 64) + "  owa-bridge_1.2.3_windows_amd64.zip",
		strings.Repeat("c", 64) + "  owa-bridge_1.2.3_windows_arm64.zip",
	}, "\n") + "\n"
	writeTestFile(t, filepath.Join(dist, "metadata.json"), metadataJSON)
	writeTestFile(t, filepath.Join(dist, "checksums.txt"), checksums)

	if err := render(dist); err != nil {
		t.Fatalf("render() error = %v", err)
	}

	formula := readTestFile(t, filepath.Join(dist, "homebrew", "Formula", "owa-bridge.rb"))
	for _, want := range []string{
		"owa-bridge_1.2.3_source.tar.gz",
		strings.Repeat("a", 64),
		`depends_on "go" => :build`,
		`"go", "build", "-mod=vendor"`,
		`man1.install "manpages/owa.1"`,
	} {
		if !strings.Contains(formula, want) {
			t.Errorf("Homebrew Formula does not contain %q", want)
		}
	}

	scoop := readTestFile(t, filepath.Join(dist, "scoop", "owa-bridge.json"))
	for _, want := range []string{"windows_amd64.zip", strings.Repeat("b", 64), "windows_arm64.zip", strings.Repeat("c", 64)} {
		if !strings.Contains(scoop, want) {
			t.Errorf("Scoop manifest does not contain %q", want)
		}
	}

	wingetRoot := filepath.Join(dist, "winget", "manifests", "n", "nkiyohara", "OWABridge", "1.2.3")
	installer := readTestFile(t, filepath.Join(wingetRoot, "nkiyohara.OWABridge.installer.yaml"))
	for _, want := range []string{"PortableCommandAlias: owa", strings.Repeat("b", 64), strings.Repeat("c", 64)} {
		if !strings.Contains(installer, want) {
			t.Errorf("WinGet installer manifest does not contain %q", want)
		}
	}
}

func TestRenderRejectsMissingArtifact(t *testing.T) {
	t.Parallel()

	dist := t.TempDir()
	writeTestFile(t, filepath.Join(dist, "metadata.json"), `{
  "tag": "v1.2.3",
  "version": "1.2.3",
  "commit": "0123456789abcdef0123456789abcdef01234567",
  "date": "2026-07-20T10:30:00Z"
}`)
	writeTestFile(t, filepath.Join(dist, "checksums.txt"), strings.Repeat("a", 64)+"  owa-bridge_1.2.3_source.tar.gz\n")

	err := render(dist)
	if err == nil || !strings.Contains(err.Error(), "windows_") {
		t.Fatalf("render() error = %v, want missing Windows archive", err)
	}
}

func writeTestFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}

func readTestFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path) // #nosec G304 -- test path is below t.TempDir.
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
