package updatecheck

import "testing"

func TestUpgradeAdviceCoversEveryInstallationMethod(t *testing.T) {
	tests := []struct {
		method InstallMethod
		want   string
	}{
		{InstallHomebrew, "brew upgrade owa-bridge"},
		{InstallWinGet, "winget upgrade --id nkiyohara.OWABridge --exact"},
		{InstallScoop, "scoop update owa-bridge"},
		{InstallDeb, "sudo apt install"},
		{InstallRPM, "sudo dnf install"},
		{InstallAPK, "sudo apk add"},
		{InstallDirect, "release page"},
	}
	for _, test := range tests {
		if got := UpgradeAdvice(test.method, "v0.4.0"); !contains(got, test.want) {
			t.Errorf("UpgradeAdvice(%q) = %q, want %q", test.method, got, test.want)
		}
	}
}

func TestDetectInstallationRecognizesCatalogPaths(t *testing.T) {
	tests := []struct {
		path string
		want InstallMethod
	}{
		{"/opt/homebrew/Cellar/owa-bridge/0.4.0/bin/owa", InstallHomebrew},
		{`C:\Users\reader\scoop\apps\owa-bridge\current\owa.exe`, InstallScoop},
		{`C:\Users\reader\AppData\Local\Microsoft\WinGet\Packages\nkiyohara.OWABridge_Microsoft.Winget.Source_8wekyb3d8bbwe\owa.exe`, InstallWinGet},
		{"/tmp/owa", InstallDirect},
	}
	for _, test := range tests {
		if got := DetectInstallation(test.path); got != test.want {
			t.Errorf("DetectInstallation(%q) = %q, want %q", test.path, got, test.want)
		}
	}
}

func contains(value, fragment string) bool {
	for index := 0; index+len(fragment) <= len(value); index++ {
		if value[index:index+len(fragment)] == fragment {
			return true
		}
	}
	return false
}
