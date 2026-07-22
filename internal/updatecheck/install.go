package updatecheck

import (
	"context"
	"io"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// InstallMethod identifies the owner of the running executable when it can be
// determined without modifying the installation.
type InstallMethod string

const (
	InstallHomebrew InstallMethod = "homebrew"
	InstallWinGet   InstallMethod = "winget"
	InstallScoop    InstallMethod = "scoop"
	InstallDeb      InstallMethod = "deb"
	InstallRPM      InstallMethod = "rpm"
	InstallAPK      InstallMethod = "apk"
	InstallDirect   InstallMethod = "direct"
)

// DetectInstallation uses path conventions first and read-only Linux package
// ownership queries second. Unknown installations remain direct downloads.
func DetectInstallation(executable string) InstallMethod {
	if resolved, err := filepath.EvalSymlinks(executable); err == nil {
		executable = resolved
	}
	executable = strings.ReplaceAll(executable, `\`, "/")
	cleaned := filepath.ToSlash(strings.ToLower(filepath.Clean(executable)))
	switch {
	case strings.Contains(cleaned, "/cellar/owa-bridge/"),
		strings.Contains(cleaned, "/homebrew/owa-bridge/"):
		return InstallHomebrew
	case strings.Contains(cleaned, "/scoop/apps/owa-bridge/"):
		return InstallScoop
	case strings.Contains(cleaned, "/winget/packages/nkiyohara.owabridge_"):
		return InstallWinGet
	}
	if runtime.GOOS != "linux" || cleaned != "/usr/bin/owa" {
		return InstallDirect
	}
	queries := []struct {
		method  InstallMethod
		command string
		args    []string
	}{
		{InstallDeb, "dpkg-query", []string{"-S", executable}},
		{InstallRPM, "rpm", []string{"-qf", executable}},
		{InstallAPK, "apk", []string{"info", "--who-owns", executable}},
	}
	for _, query := range queries {
		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		command := exec.CommandContext(ctx, query.command, query.args...) // #nosec G204 -- fixed read-only package query with the running executable path.
		command.Stdout = io.Discard
		command.Stderr = io.Discard
		err := command.Run()
		cancel()
		if err == nil {
			return query.method
		}
	}
	return InstallDirect
}

// UpgradeAdvice returns an actionable command without replacing a
// package-manager-owned binary.
func UpgradeAdvice(method InstallMethod, version string) string {
	switch method {
	case InstallHomebrew:
		return "brew upgrade owa-bridge"
	case InstallWinGet:
		return "winget upgrade --id nkiyohara.OWABridge --exact"
	case InstallScoop:
		return "scoop update owa-bridge"
	case InstallDeb:
		return "download and verify the new .deb, then run: sudo apt install ./owa-bridge_" + version + "_*.deb"
	case InstallRPM:
		return "download and verify the new .rpm, then run: sudo dnf install ./owa-bridge-" + version + "-*.rpm"
	case InstallAPK:
		return "download and verify the new .apk, then run: sudo apk add ./owa-bridge_" + version + "_*.apk"
	case InstallDirect:
		return "download and verify the new archive from the release page"
	}
	return "review the verified release and use the original installation surface"
}
