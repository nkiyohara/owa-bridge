package browser

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// ResolveExecutable returns the exact Chromium-family executable Launch will
// use. An explicit configuration never silently falls back to another browser.
func ResolveExecutable(configured string) (string, error) {
	if strings.ContainsAny(configured, "\r\n\x00") {
		return "", errors.New("browser executable contains a forbidden character")
	}
	if configured != "" {
		resolved, err := exec.LookPath(configured)
		if err != nil {
			return "", errors.New("configured Chromium executable was not found or is not executable")
		}
		return absoluteExecutable(resolved)
	}

	for _, candidate := range executableCandidates() {
		if candidate == "" {
			continue
		}
		resolved, err := exec.LookPath(candidate)
		if err == nil {
			return absoluteExecutable(resolved)
		}
	}
	return "", errors.New("no supported Chromium executable found; set browser.executable in config.toml")
}

func absoluteExecutable(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", errors.New("resolve Chromium executable path")
	}
	return filepath.Clean(absolute), nil
}

func executableCandidates() []string {
	switch runtime.GOOS {
	case "darwin":
		return []string{
			"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			"/Applications/Chromium.app/Contents/MacOS/Chromium",
			"/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge",
			"google-chrome",
			"chromium",
			"microsoft-edge",
		}
	case "windows":
		programFiles := os.Getenv("ProgramFiles")
		programFilesX86 := os.Getenv("ProgramFiles(x86)")
		localAppData := os.Getenv("LOCALAPPDATA")
		return []string{
			"chrome.exe",
			"msedge.exe",
			joinIfSet(programFiles, `Google\Chrome\Application\chrome.exe`),
			joinIfSet(programFilesX86, `Google\Chrome\Application\chrome.exe`),
			joinIfSet(localAppData, `Google\Chrome\Application\chrome.exe`),
			joinIfSet(programFiles, `Microsoft\Edge\Application\msedge.exe`),
			joinIfSet(programFilesX86, `Microsoft\Edge\Application\msedge.exe`),
			joinIfSet(localAppData, `Microsoft\Edge\Application\msedge.exe`),
		}
	default:
		return []string{
			"google-chrome",
			"google-chrome-stable",
			"chromium",
			"chromium-browser",
			"microsoft-edge",
			"microsoft-edge-stable",
			"/usr/bin/google-chrome",
			"/usr/bin/chromium",
			"/snap/bin/chromium",
		}
	}
}

func joinIfSet(directory, name string) string {
	if directory == "" {
		return ""
	}
	return filepath.Join(directory, name)
}
