package cliargs

import (
	"os"
	"os/exec"
	"testing"
)

// --- ArgsCommandClientRestoreReportList ---

func TestRestoreReportListMissingConfig(t *testing.T) {
	testobj := &ArgsCommandClientRestoreReportList{}
	testobj.ConfigFile = "a/file/which/does/not/exist"
	testobj.Job.Name = "some_job"

	if os.Getenv("TEST_RUNNING") == "1" {
		_ = testobj.Execute(make([]string, 0))
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestRestoreReportListMissingConfig") // #nosec
	cmd.Env = append(os.Environ(), "TEST_RUNNING=1")
	err := cmd.Run()
	if err == nil {
		t.Fatal("process ran without error, want exit status 1 for missing config")
	}
}

func TestRestoreReportListInvalidFromStartTime(t *testing.T) {
	testobj := &ArgsCommandClientRestoreReportList{}
	testobj.ConfigFile = "a/file/which/does/not/exist"
	testobj.Job.Name = "some_job"
	testobj.StartTime = "not-a-date"

	if os.Getenv("TEST_RUNNING") == "1" {
		_ = testobj.Execute(make([]string, 0))
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestRestoreReportListInvalidFromStartTime") // #nosec
	cmd.Env = append(os.Environ(), "TEST_RUNNING=1")
	err := cmd.Run()
	if err == nil {
		t.Fatal("process ran without error, want exit status 1 for invalid from-start-time")
	}
}

func TestRestoreReportListInvalidUntilStartTime(t *testing.T) {
	testobj := &ArgsCommandClientRestoreReportList{}
	testobj.ConfigFile = "a/file/which/does/not/exist"
	testobj.Job.Name = "some_job"
	testobj.EndTime = "not-a-date"

	if os.Getenv("TEST_RUNNING") == "1" {
		_ = testobj.Execute(make([]string, 0))
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestRestoreReportListInvalidUntilStartTime") // #nosec
	cmd.Env = append(os.Environ(), "TEST_RUNNING=1")
	err := cmd.Run()
	if err == nil {
		t.Fatal("process ran without error, want exit status 1 for invalid until-start-time")
	}
}

func TestRestoreReportListUntilBeforeFrom(t *testing.T) {
	testobj := &ArgsCommandClientRestoreReportList{}
	testobj.ConfigFile = "a/file/which/does/not/exist"
	testobj.Job.Name = "some_job"
	testobj.StartTime = "2026-04-10T00:00:00Z"
	testobj.EndTime = "2026-04-01T00:00:00Z"

	if os.Getenv("TEST_RUNNING") == "1" {
		_ = testobj.Execute(make([]string, 0))
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestRestoreReportListUntilBeforeFrom") // #nosec
	cmd.Env = append(os.Environ(), "TEST_RUNNING=1")
	err := cmd.Run()
	if err == nil {
		t.Fatal("process ran without error, want exit status 1 for until before from")
	}
}

// --- ArgsCommandClientRestoreReportShow ---

func TestRestoreReportShowMissingConfig(t *testing.T) {
	testobj := &ArgsCommandClientRestoreReportShow{}
	testobj.ConfigFile = "a/file/which/does/not/exist"
	testobj.Job.Name = "some_job"
	testobj.JobId = "some-id"

	if os.Getenv("TEST_RUNNING") == "1" {
		_ = testobj.Execute(make([]string, 0))
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestRestoreReportShowMissingConfig") // #nosec
	cmd.Env = append(os.Environ(), "TEST_RUNNING=1")
	err := cmd.Run()
	if err == nil {
		t.Fatal("process ran without error, want exit status 1 for missing config")
	}
}
