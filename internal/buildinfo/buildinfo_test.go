package buildinfo

import "testing"

func TestCurrentIsComplete(t *testing.T) {
	t.Parallel()

	info := Current()
	if info.Version == "" || info.Commit == "" || info.BuildDate == "" {
		t.Fatalf("Current() returned incomplete version metadata: %+v", info)
	}
	if info.GoVersion == "" || info.OS == "" || info.Arch == "" {
		t.Fatalf("Current() returned incomplete runtime metadata: %+v", info)
	}
}
