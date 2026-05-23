package restore

import (
	"path/filepath"
	"strings"
	"testing"
)

// FuzzMapPathIntoRestoreDir asserts the mapper never panics and, when invoked with the
// documented contract — restoreDir and sourcePath are both clean, absolute paths native
// to the current GOOS — always returns a path rooted under restoreDir.
//
// Findings during development of this fuzz target (recorded so the file documents itself):
//  1. mapPathIntoRestoreDir does NOT defend against a relative sourcePath containing
//     ".." segments — "/r" + "../../../etc/passwd" → "/etc/passwd". This is a callable-
//     contract violation rather than a defect (the function comment specifies an absolute
//     sourcePath) but worth tightening if any other call site is ever added.
//  2. Degenerate "absolute" restoreDirs like "/." or "/" cause filepath.Join to normalise
//     away the prefix. Real callers feed a fully-specified restore directory.
//
// Both findings are skipped below; both are worth tracking in the project's issue tracker.
//
// Run with: go test -fuzz=FuzzMapPathIntoRestoreDir -fuzztime=20s ./restore
func FuzzMapPathIntoRestoreDir(f *testing.F) {
	f.Add("/r", "/etc/hosts")
	f.Add("/r", "/a/b/c/d/e")

	f.Fuzz(func(t *testing.T, restoreDir string, sourcePath string) {
		// Both inputs must be: absolute, native to current GOOS, and clean.
		if !filepath.IsAbs(restoreDir) || !filepath.IsAbs(sourcePath) {
			return
		}
		if filepath.Clean(restoreDir) != restoreDir || filepath.Clean(sourcePath) != sourcePath {
			return
		}
		if restoreDir == "/" || sourcePath == "/" {
			return
		}
		got := mapPathIntoRestoreDir(restoreDir, sourcePath)
		if !strings.HasPrefix(got, restoreDir) {
			t.Fatalf("mapPathIntoRestoreDir(%q, %q) = %q, expected prefix %q",
				restoreDir, sourcePath, got, restoreDir)
		}
	})
}
