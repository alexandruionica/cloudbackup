package restore

import (
	"cloudbackup/shared"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

func TestPickTarget(t *testing.T) {
	cfg := shared.ConfigBackup{
		Name: "demo",
		Target: []shared.ConfigBackupTarget{
			{Name: "primary"},
			{Name: "secondary"},
		},
	}

	if got, err := pickTarget(cfg, ""); err != nil || got.Name != "primary" {
		t.Errorf("empty target name should return first target; got=%q err=%v", got.Name, err)
	}
	if got, err := pickTarget(cfg, "secondary"); err != nil || got.Name != "secondary" {
		t.Errorf("named target lookup failed; got=%q err=%v", got.Name, err)
	}
	if _, err := pickTarget(cfg, "nope"); err == nil {
		t.Error("unknown target name should return error")
	}
	if _, err := pickTarget(shared.ConfigBackup{Name: "demo"}, ""); err == nil {
		t.Error("backup with no targets should return error")
	}
}

func TestResolveRestoreDir(t *testing.T) {
	mu := &sync.RWMutex{}

	// 1. explicit per-request override always wins
	override := filepath.Join("tmp", "override")
	cfg := shared.CfgTemplate{DataDir: "/var/data", RestoreDir: "/var/restores", Mutex: mu}
	got, err := resolveRestoreDir(Request{RestoreJobId: "rj", RestoreDirOverride: override}, cfg, "demo")
	if err != nil || got != filepath.Clean(override) {
		t.Errorf("override path expected %q, got %q (err=%v)", filepath.Clean(override), got, err)
	}

	// 2. configured RestoreDir is used as base
	cfg2 := shared.CfgTemplate{DataDir: "/var/data", RestoreDir: "/srv/restores", Mutex: mu}
	got, err = resolveRestoreDir(Request{RestoreJobId: "rj"}, cfg2, "demo")
	want := filepath.Join("/srv/restores", "demo", "rj")
	if err != nil || got != want {
		t.Errorf("configured base expected %q, got %q (err=%v)", want, got, err)
	}

	// 3. empty RestoreDir falls back to <DataDir>/restores
	cfg3 := shared.CfgTemplate{DataDir: "/var/data", Mutex: mu}
	got, err = resolveRestoreDir(Request{RestoreJobId: "rj"}, cfg3, "demo")
	want = filepath.Join("/var/data", "restores", "demo", "rj")
	if err != nil || got != want {
		t.Errorf("default base expected %q, got %q (err=%v)", want, got, err)
	}
}

func TestMapPathIntoRestoreDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix-only path mapping check")
	}
	cases := []struct {
		restoreDir string
		source     string
		want       string
	}{
		{"/r", "/etc/hosts", "/r/etc/hosts"},
		{"/r", "/var/log/", "/r/var/log"},
		{"/r", "relative/file", "/r/relative/file"},
	}
	for _, c := range cases {
		got := mapPathIntoRestoreDir(c.restoreDir, c.source)
		if got != c.want {
			t.Errorf("mapPathIntoRestoreDir(%q,%q) = %q, want %q", c.restoreDir, c.source, got, c.want)
		}
	}
}

func TestMapPathIntoRestoreDirNoEscape(t *testing.T) {
	// Sanity: result should always be under restoreDir, never escape via the leading slash.
	got := mapPathIntoRestoreDir("/r", "/etc/hosts")
	if !strings.HasPrefix(got, "/r/") {
		t.Errorf("mapped path %q should remain under /r/", got)
	}
}
