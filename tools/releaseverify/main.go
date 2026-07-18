// Command releaseverify validates the complete local GoReleaser output.
package main

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	expectedArchives = 6
	expectedPackages = 6
	expectedSBOMs    = 24
	licensePrefix    = "third_party_licenses/"
	minimumLicenses  = 24
)

type artifact struct {
	Name   string         `json:"name"`
	Type   string         `json:"type"`
	GOOS   string         `json:"goos"`
	GOARCH string         `json:"goarch"`
	Extra  map[string]any `json:"extra"`
}

type scoopManifest struct {
	Architecture map[string]struct {
		URL  string `json:"url"`
		Hash string `json:"hash"`
	} `json:"architecture"`
}

func main() {
	dist := flag.String("dist", "dist", "GoReleaser output directory")
	flag.Parse()

	if err := verify(*dist); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "release verification failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf(
		"release verification passed: %d archives, %d packages, %d SBOMs\n",
		expectedArchives,
		expectedPackages,
		expectedSBOMs,
	)
}

func verify(dist string) error {
	if err := verifyLicenseBundle(filepath.Join(".release", "third_party_licenses")); err != nil {
		return err
	}
	artifacts, err := readArtifacts(filepath.Join(dist, "artifacts.json"))
	if err != nil {
		return err
	}
	hashes, err := verifyChecksums(dist)
	if err != nil {
		return err
	}
	if err := verifyInventory(dist, artifacts, hashes); err != nil {
		return err
	}
	if err := verifyCatalogs(dist, hashes); err != nil {
		return err
	}
	return nil
}

func readArtifacts(path string) ([]artifact, error) {
	data, err := readLocalFile(path)
	if err != nil {
		return nil, fmt.Errorf("read artifact inventory: %w", err)
	}
	var artifacts []artifact
	if err := json.Unmarshal(data, &artifacts); err != nil {
		return nil, fmt.Errorf("decode artifact inventory: %w", err)
	}
	return artifacts, nil
}

func verifyChecksums(dist string) (map[string]string, error) {
	manifest, err := readLocalFile(filepath.Join(dist, "checksums.txt"))
	if err != nil {
		return nil, fmt.Errorf("read checksum manifest: %w", err)
	}
	hashes := make(map[string]string)
	for lineNumber, line := range strings.Split(strings.TrimSpace(string(manifest)), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			return nil, fmt.Errorf("checksum line %d is malformed", lineNumber+1)
		}
		want, name := strings.ToLower(fields[0]), fields[1]
		if len(want) != sha256.Size*2 {
			return nil, fmt.Errorf("checksum for %q is not SHA-256", name)
		}
		if _, err := hex.DecodeString(want); err != nil {
			return nil, fmt.Errorf("checksum for %q is not hexadecimal: %w", name, err)
		}
		if filepath.Base(name) != name {
			return nil, fmt.Errorf("checksum path %q is not a release basename", name)
		}
		if err := validateGitHubAssetName(name); err != nil {
			return nil, err
		}
		got, err := hashFile(filepath.Join(dist, name))
		if err != nil {
			return nil, err
		}
		if got != want {
			return nil, fmt.Errorf("checksum mismatch for %q", name)
		}
		if _, exists := hashes[name]; exists {
			return nil, fmt.Errorf("duplicate checksum for %q", name)
		}
		hashes[name] = want
	}
	if len(hashes) != expectedArchives+expectedPackages+expectedSBOMs {
		return nil, fmt.Errorf("checksum count is %d, want %d", len(hashes), 36)
	}
	return hashes, nil
}

func validateGitHubAssetName(name string) error {
	if strings.Contains(name, "~") {
		return fmt.Errorf(
			"release asset %q contains '~', which GitHub rewrites and would invalidate checksums",
			name,
		)
	}
	return nil
}

func verifyInventory(dist string, artifacts []artifact, hashes map[string]string) error {
	counts := make(map[string]int)
	targets := make(map[string]bool)
	packageFormats := make(map[string]int)
	sbomFormats := make(map[string]int)
	for _, item := range artifacts {
		counts[item.Type]++
		switch item.Type {
		case "Archive":
			if filepath.Base(item.Name) != item.Name {
				return fmt.Errorf("archive name %q is not a basename", item.Name)
			}
			targets[item.GOOS+"/"+item.GOARCH] = true
			if hashes[item.Name] == "" {
				return fmt.Errorf("archive %q is absent from checksums", item.Name)
			}
			if err := verifyArchive(filepath.Join(dist, item.Name), item.GOOS); err != nil {
				return err
			}
		case "Linux Package":
			extension := filepath.Ext(item.Name)
			packageFormats[extension]++
			if hashes[item.Name] == "" {
				return fmt.Errorf("package %q is absent from checksums", item.Name)
			}
			if missing := packageMissingFiles(item.Extra); len(missing) > 0 {
				return fmt.Errorf("package %q does not declare required files %q", item.Name, missing)
			}
		case "SBOM":
			if hashes[item.Name] == "" {
				return fmt.Errorf("SBOM %q is absent from checksums", item.Name)
			}
			format, err := verifySBOM(filepath.Join(dist, item.Name))
			if err != nil {
				return err
			}
			sbomFormats[format]++
		}
	}

	wantCounts := map[string]int{
		"Archive":       expectedArchives,
		"Binary":        expectedArchives,
		"Checksum":      1,
		"Linux Package": expectedPackages,
		"Metadata":      1,
		"SBOM":          expectedSBOMs,
	}
	for kind, want := range wantCounts {
		if counts[kind] != want {
			return fmt.Errorf("%s count is %d, want %d", kind, counts[kind], want)
		}
	}
	for _, goos := range []string{"darwin", "linux", "windows"} {
		for _, goarch := range []string{"amd64", "arm64"} {
			target := goos + "/" + goarch
			if !targets[target] {
				return fmt.Errorf("release target %s is missing", target)
			}
		}
	}
	for _, extension := range []string{".apk", ".deb", ".rpm"} {
		if packageFormats[extension] != 2 {
			return fmt.Errorf("%s package count is %d, want 2", extension, packageFormats[extension])
		}
	}
	if sbomFormats["CycloneDX"] != 12 || sbomFormats["SPDX"] != 12 {
		return fmt.Errorf("SBOM formats are %#v, want 12 CycloneDX and 12 SPDX", sbomFormats)
	}
	return nil
}

func packageMissingFiles(extra map[string]any) []string {
	required := map[string]bool{
		"/usr/bin/owa": false,
		"/usr/share/bash-completion/completions/owa":     false,
		"/usr/share/zsh/site-functions/_owa":             false,
		"/usr/share/fish/vendor_completions.d/owa.fish":  false,
		"/usr/share/man/man1/owa.1":                      false,
		"/usr/share/doc/owa-bridge/CHANGELOG.md":         false,
		"/usr/share/doc/owa-bridge/third_party_licenses": false,
	}
	files, ok := extra["Files"].([]any)
	if !ok {
		return sortedMissingFiles(required)
	}
	for _, value := range files {
		file, ok := value.(map[string]any)
		if !ok {
			continue
		}
		if destination, ok := file["dst"].(string); ok {
			if _, requiredDestination := required[destination]; requiredDestination {
				required[destination] = true
			}
		}
	}
	return sortedMissingFiles(required)
}

func verifyLicenseBundle(root string) error {
	required := map[string]bool{
		"github.com/alecthomas/kong/COPYING":               false,
		"github.com/hashicorp/go-multierror/LICENSE":       false,
		"github.com/hashicorp/go-multierror/multierror.go": false,
		"github.com/modelcontextprotocol/go-sdk/LICENSE":   false,
		"golang.org/x/sys/LICENSE":                         false,
	}
	files := 0
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("license bundle contains symlink %q", path)
		}
		if entry.IsDir() {
			return nil
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("license bundle contains non-regular file %q", path)
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		files++
		relative = filepath.ToSlash(relative)
		if _, expected := required[relative]; expected {
			required[relative] = true
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("inspect third-party license bundle: %w", err)
	}
	if missing := sortedMissingFiles(required); len(missing) > 0 {
		return fmt.Errorf("third-party license bundle is missing %q", missing)
	}
	if files < minimumLicenses {
		return fmt.Errorf(
			"third-party license bundle contains %d files, want at least %d",
			files, minimumLicenses,
		)
	}
	return nil
}

func sortedMissingFiles(required map[string]bool) []string {
	missing := make([]string, 0, len(required))
	for path, found := range required {
		if !found {
			missing = append(missing, path)
		}
	}
	sort.Strings(missing)
	return missing
}

func verifySBOM(path string) (string, error) {
	data, err := readLocalFile(path)
	if err != nil {
		return "", fmt.Errorf("read SBOM %q: %w", filepath.Base(path), err)
	}
	var document map[string]any
	if err := json.Unmarshal(data, &document); err != nil {
		return "", fmt.Errorf("decode SBOM %q: %w", filepath.Base(path), err)
	}
	if strings.Contains(string(data), "syft-archive-contents-") {
		return "", fmt.Errorf("SBOM %q exposes a generated Syft temp path", filepath.Base(path))
	}
	if document["bomFormat"] == "CycloneDX" {
		metadata, ok := document["metadata"].(map[string]any)
		if !ok {
			return "", fmt.Errorf("CycloneDX SBOM %q has no metadata", filepath.Base(path))
		}
		if err := verifyCanonicalTimestamp(metadata["timestamp"]); err != nil {
			return "", fmt.Errorf("CycloneDX SBOM %q: %w", filepath.Base(path), err)
		}
		serial, _ := document["serialNumber"].(string)
		if !strings.HasPrefix(serial, "urn:uuid:") || len(serial) != len("urn:uuid:")+36 {
			return "", fmt.Errorf("CycloneDX SBOM %q has a non-canonical serial number", filepath.Base(path))
		}
		return "CycloneDX", nil
	}
	if version, ok := document["spdxVersion"].(string); ok && strings.HasPrefix(version, "SPDX-") {
		creation, ok := document["creationInfo"].(map[string]any)
		if !ok {
			return "", fmt.Errorf("SPDX SBOM %q has no creation info", filepath.Base(path))
		}
		if err := verifyCanonicalTimestamp(creation["created"]); err != nil {
			return "", fmt.Errorf("SPDX SBOM %q: %w", filepath.Base(path), err)
		}
		namespace, _ := document["documentNamespace"].(string)
		if !strings.HasPrefix(namespace, "https://github.com/nkiyohara/owa-bridge/sbom/spdx/") {
			return "", fmt.Errorf("SPDX SBOM %q has a non-canonical namespace", filepath.Base(path))
		}
		return "SPDX", nil
	}
	return "", fmt.Errorf("SBOM %q has an unknown format", filepath.Base(path))
}

func verifyCanonicalTimestamp(value any) error {
	timestamp, ok := value.(string)
	if !ok {
		return errors.New("timestamp is missing")
	}
	parsed, err := time.Parse(time.RFC3339, timestamp)
	if err != nil {
		return fmt.Errorf("timestamp is not RFC3339: %w", err)
	}
	if timestamp != parsed.UTC().Format(time.RFC3339) {
		return errors.New("timestamp is not canonical UTC")
	}
	return nil
}

func verifyArchive(path, goos string) error {
	want := []string{
		"CHANGELOG.md",
		"LICENSE",
		"README.md",
		"SECURITY.md",
		"completions/_owa",
		"completions/owa.bash",
		"completions/owa.fish",
		"docs/install.md",
		"docs/mcp.md",
		"manpages/owa.1",
		licensePrefix + "github.com/alecthomas/kong/COPYING",
		licensePrefix + "github.com/hashicorp/go-multierror/LICENSE",
		licensePrefix + "github.com/hashicorp/go-multierror/multierror.go",
	}
	if goos == "windows" {
		want = append(want, "owa.exe")
		return verifyZip(path, want)
	}
	want = append(want, "owa")
	return verifyTarGzip(path, want)
}

func verifyZip(path string, want []string) error {
	archive, err := zip.OpenReader(path)
	if err != nil {
		return fmt.Errorf("open zip %q: %w", filepath.Base(path), err)
	}
	defer func() { _ = archive.Close() }()
	names := make([]string, 0, len(archive.File))
	for _, file := range archive.File {
		names = append(names, file.Name)
	}
	return requireReleaseFiles(filepath.Base(path), names, want)
}

func verifyTarGzip(path string, want []string) error {
	file, err := openLocalFile(path)
	if err != nil {
		return fmt.Errorf("open tarball %q: %w", filepath.Base(path), err)
	}
	defer func() { _ = file.Close() }()
	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		return fmt.Errorf("open gzip stream %q: %w", filepath.Base(path), err)
	}
	defer func() { _ = gzipReader.Close() }()
	tarReader := tar.NewReader(gzipReader)
	var names []string
	for {
		header, err := tarReader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("read tarball %q: %w", filepath.Base(path), err)
		}
		names = append(names, header.Name)
	}
	return requireReleaseFiles(filepath.Base(path), names, want)
}

func requireReleaseFiles(archive string, got, want []string) error {
	required := make(map[string]bool, len(want))
	for _, name := range want {
		required[name] = false
	}
	licenseFiles := 0
	var unexpected []string
	for _, name := range got {
		if _, exists := required[name]; exists {
			required[name] = true
		}
		if strings.HasPrefix(name, licensePrefix) {
			if !strings.HasSuffix(name, "/") {
				licenseFiles++
			}
			continue
		}
		if _, exists := required[name]; !exists {
			unexpected = append(unexpected, name)
		}
	}
	if missing := sortedMissingFiles(required); len(missing) > 0 {
		return fmt.Errorf("archive %q is missing %q", archive, missing)
	}
	if len(unexpected) > 0 {
		sort.Strings(unexpected)
		return fmt.Errorf("archive %q contains unexpected files %q", archive, unexpected)
	}
	if licenseFiles < minimumLicenses {
		return fmt.Errorf(
			"archive %q contains %d third-party license files, want at least %d",
			archive, licenseFiles, minimumLicenses,
		)
	}
	return nil
}

func verifyCatalogs(dist string, hashes map[string]string) error {
	cask, err := readLocalFile(filepath.Join(dist, "homebrew", "Casks", "owa-bridge.rb"))
	if err != nil {
		return fmt.Errorf("read Homebrew Cask: %w", err)
	}
	if !strings.Contains(string(cask), `binary "owa"`) {
		return errors.New("homebrew Cask does not install owa")
	}
	for _, snippet := range []string{
		`bash_completion "completions/owa.bash"`,
		`zsh_completion "completions/_owa"`,
		`fish_completion "completions/owa.fish"`,
		`manpage "manpages/owa.1"`,
	} {
		if !strings.Contains(string(cask), snippet) {
			return fmt.Errorf("homebrew Cask is missing %q", snippet)
		}
	}
	for name, hash := range hashes {
		if (strings.Contains(name, "_darwin_") || strings.Contains(name, "_linux_")) &&
			(strings.HasSuffix(name, ".tar.gz")) && !strings.Contains(string(cask), hash) {
			return fmt.Errorf("homebrew Cask is missing the hash for %q", name)
		}
	}

	manifestData, err := readLocalFile(filepath.Join(dist, "scoop", "owa-bridge.json"))
	if err != nil {
		return fmt.Errorf("read Scoop manifest: %w", err)
	}
	var manifest scoopManifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		return fmt.Errorf("decode Scoop manifest: %w", err)
	}
	if len(manifest.Architecture) != 2 {
		return fmt.Errorf("scoop architecture count is %d, want 2", len(manifest.Architecture))
	}
	for architecture, item := range manifest.Architecture {
		name := filepath.Base(item.URL)
		if hashes[name] != item.Hash {
			return fmt.Errorf("scoop %s hash does not match %q", architecture, name)
		}
	}

	wingetFiles, err := filepath.Glob(filepath.Join(dist, "winget", "manifests", "*", "*", "*", "*", "*"))
	if err != nil {
		return fmt.Errorf("find WinGet manifests: %w", err)
	}
	if len(wingetFiles) != 3 {
		return fmt.Errorf("WinGet manifest count is %d, want 3", len(wingetFiles))
	}
	var installer string
	for _, path := range wingetFiles {
		if strings.HasSuffix(path, ".installer.yaml") {
			data, err := readLocalFile(path)
			if err != nil {
				return fmt.Errorf("read WinGet installer manifest: %w", err)
			}
			installer = string(data)
		}
	}
	if !strings.Contains(installer, "PortableCommandAlias: owa") {
		return errors.New("WinGet manifest does not install the owa command")
	}
	for name, hash := range hashes {
		if strings.Contains(name, "_windows_") && strings.HasSuffix(name, ".zip") {
			if !strings.Contains(installer, name) || !strings.Contains(installer, hash) {
				return fmt.Errorf("WinGet manifest does not bind %q to its hash", name)
			}
		}
	}
	return nil
}

func hashFile(path string) (string, error) {
	file, err := openLocalFile(path)
	if err != nil {
		return "", fmt.Errorf("open %q: %w", filepath.Base(path), err)
	}
	defer func() { _ = file.Close() }()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", fmt.Errorf("hash %q: %w", filepath.Base(path), err)
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

// The verifier is a local release-engineering command. Every caller either
// constructs a fixed path below --dist or first requires an artifact basename.
func readLocalFile(path string) ([]byte, error) {
	// #nosec G304 -- constrained local release output, never a network path.
	return os.ReadFile(path)
}

func openLocalFile(path string) (*os.File, error) {
	// #nosec G304 -- constrained local release output, never a network path.
	return os.Open(path)
}
