package main

import (
	"slices"
	"strings"
	"testing"
)

func TestValidateGitHubAssetName(t *testing.T) {
	t.Parallel()

	if err := validateGitHubAssetName("owa-bridge_0.1.0-rc.2_amd64.deb"); err != nil {
		t.Fatalf("validateGitHubAssetName() rejected a safe name: %v", err)
	}
	err := validateGitHubAssetName("owa-bridge_0.1.0~rc.2_amd64.deb")
	if err == nil || !strings.Contains(err.Error(), "GitHub rewrites") {
		t.Fatalf("validateGitHubAssetName() error = %v, want GitHub rewrite warning", err)
	}
}

func TestPackageInventoryRequiresPublicChangelog(t *testing.T) {
	t.Parallel()

	destinations := []string{
		"/usr/bin/owa",
		"/usr/share/bash-completion/completions/owa",
		"/usr/share/zsh/site-functions/_owa",
		"/usr/share/fish/vendor_completions.d/owa.fish",
		"/usr/share/man/man1/owa.1",
		"/usr/share/doc/owa-bridge/CHANGELOG.md",
		"/usr/share/doc/owa-bridge/third_party_licenses",
		"/usr/share/owa-bridge/plugins/owa-bridge",
		"/usr/share/owa-bridge/.agents/plugins/marketplace.json",
		"/usr/share/owa-bridge/.claude-plugin/marketplace.json",
	}
	files := make([]any, 0, len(destinations))
	for _, destination := range destinations {
		files = append(files, map[string]any{"dst": destination})
	}
	if missing := packageMissingFiles(map[string]any{"Files": files}); len(missing) != 0 {
		t.Fatalf("complete package inventory missing = %v", missing)
	}

	withoutChangelog := append([]any(nil), files[:5]...)
	withoutChangelog = append(withoutChangelog, files[6:]...)
	missing := packageMissingFiles(map[string]any{"Files": withoutChangelog})
	if !slices.Contains(missing, "/usr/share/doc/owa-bridge/CHANGELOG.md") {
		t.Fatalf("package inventory missing = %v, want changelog", missing)
	}
}

func TestArchiveInventoryAcceptsChangelogAndRejectsExtras(t *testing.T) {
	t.Parallel()

	want := []string{"CHANGELOG.md", "LICENSE", "README.md"}
	got := append([]string(nil), want...)
	for range minimumLicenses {
		got = append(got, licensePrefix+"example.invalid/dependency/LICENSE")
	}
	if err := requireReleaseFiles("synthetic.zip", got, want); err != nil {
		t.Fatalf("requireReleaseFiles() error = %v", err)
	}
	if err := requireReleaseFiles("synthetic.zip", append(got, "unexpected.txt"), want); err == nil {
		t.Fatal("requireReleaseFiles() accepted an unexpected file")
	}
}
