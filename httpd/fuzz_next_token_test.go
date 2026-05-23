package httpd

import "testing"

// FuzzDecodeNextTokenOfReportBackupList asserts the decoder never panics on
// arbitrary input. Round-trip behaviour is covered by the deterministic tests
// in api_rest_report_backup_test.go.
//
// Run with: go test -fuzz=FuzzDecodeNextTokenOfReportBackupList -fuzztime=20s ./httpd
func FuzzDecodeNextTokenOfReportBackupList(f *testing.F) {
	f.Add(buildNextTokenOfReportBackupList(10, 5, 100, 200))
	f.Add("not-base64")
	f.Add("")
	f.Add("!@#$%^&*()")

	f.Fuzz(func(t *testing.T, in string) {
		// Must not panic on any input — the decoder's contract is "return an error or
		// a parsed result." A panic would crash the daemon for an attacker-controlled
		// 'next' query parameter.
		_, _, _, _, _ = decodeNextTokenOfReportBackupList(in)
	})
}

// FuzzDecodeNextTokenOfReportBackupFileList mirrors FuzzDecodeNextTokenOfReportBackupList
// for the file-list variant (which has five colon-separated parts including a boolean).
//
// Run with: go test -fuzz=FuzzDecodeNextTokenOfReportBackupFileList -fuzztime=20s ./httpd
func FuzzDecodeNextTokenOfReportBackupFileList(f *testing.F) {
	f.Add(buildNextTokenOfReportBackupFileList(10, 5, "j", "/p", true))
	f.Add("not-base64")
	f.Add("")
	f.Add("!@#$%")

	f.Fuzz(func(t *testing.T, in string) {
		_, _, _, _, _, _ = decodeNextTokenOfReportBackupFileList(in)
	})
}
