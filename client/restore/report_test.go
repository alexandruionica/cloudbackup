package restore

import (
	"cloudbackup/httpd"
	"cloudbackup/shared"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// -----------------------------------------------------------------------------
// doReportList
// -----------------------------------------------------------------------------

func TestDoReportList_ParsesResponse(t *testing.T) {
	start := time.Now().UTC().Truncate(time.Second)
	end := start.Add(5 * time.Minute)
	srv, rs := newRecorder(t, 200, ReportListResponse{
		HttpStatusReply: httpd.HttpStatusReply{Code: "success", Message: "success"},
		Result: []httpd.ReportBackupListDbResults{
			{Name: "job1", JobId: "RID-1", StartTime: start.Format(time.RFC3339Nano), EndTime: end.Format(time.RFC3339Nano), State: "finished"},
			{Name: "job1", JobId: "RID-2", StartTime: start.Format(time.RFC3339Nano), EndTime: end.Format(time.RFC3339Nano), State: "finished"},
		},
	})
	defer srv.Close()

	decoded, body, err := doReportList(cfg(srv.URL), "job1", start, end, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(body) == 0 {
		t.Error("expected raw body to be populated")
	}
	if len(decoded.Result) != 2 {
		t.Fatalf("expected 2 results, got %d", len(decoded.Result))
	}
	if decoded.Result[0].JobId != "RID-1" || decoded.Result[1].JobId != "RID-2" {
		t.Errorf("unexpected job ids: %+v", decoded.Result)
	}
	if rs.lastMethod != "POST" || rs.lastPath != "/api/v1/report/restore/list" {
		t.Errorf("unexpected request method/path: %s %s", rs.lastMethod, rs.lastPath)
	}
	var sent httpd.ReportRestoreList
	if err := json.Unmarshal(rs.lastBody, &sent); err != nil {
		t.Fatalf("could not decode sent body: %v", err)
	}
	if sent.Name != "job1" {
		t.Errorf("expected name=job1, got %q", sent.Name)
	}
}

func TestDoReportList_EmptyResult(t *testing.T) {
	srv, _ := newRecorder(t, 200, ReportListResponse{
		HttpStatusReply: httpd.HttpStatusReply{Code: "success", Message: "success"},
		Result:          []httpd.ReportBackupListDbResults{},
	})
	defer srv.Close()

	decoded, _, err := doReportList(cfg(srv.URL), "job1", time.Now().Add(-24*time.Hour), time.Now(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(decoded.Result) != 0 {
		t.Errorf("expected empty result, got %+v", decoded.Result)
	}
}

func TestDoReportList_WithNextToken(t *testing.T) {
	srv, rs := newRecorder(t, 200, ReportListResponse{
		HttpStatusReply: httpd.HttpStatusReply{Code: "success", Message: "success"},
		Result:          []httpd.ReportBackupListDbResults{},
	})
	defer srv.Close()

	_, _, err := doReportList(cfg(srv.URL), "job1", time.Now(), time.Now(), "some-token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var sent httpd.ReportRestoreList
	_ = json.Unmarshal(rs.lastBody, &sent)
	if sent.Next != "some-token" {
		t.Errorf("expected next=some-token, got %q", sent.Next)
	}
}

func TestDoReportList_ServerError(t *testing.T) {
	srv, _ := newRecorder(t, 400, httpd.HttpStatusReply{Code: "error", Message: "bad request"})
	defer srv.Close()

	_, _, err := doReportList(cfg(srv.URL), "job1", time.Now(), time.Now(), "")
	if err == nil {
		t.Fatal("expected error on 400 response")
	}
	if !strings.Contains(err.Error(), "bad request") {
		t.Errorf("expected error to contain 'bad request', got %q", err.Error())
	}
}

func TestDoReportList_ConnectionError(t *testing.T) {
	srv, _ := newRecorder(t, 200, ReportListResponse{})
	addr := srv.URL
	srv.Close()

	_, _, err := doReportList(cfg(addr), "job1", time.Now(), time.Now(), "")
	if err == nil {
		t.Fatal("expected network error when server is unreachable")
	}
}

// -----------------------------------------------------------------------------
// doReportShow
// -----------------------------------------------------------------------------

func TestDoReportShow_ParsesResponse(t *testing.T) {
	srv, rs := newRecorder(t, 200, ReportShowResponse{
		HttpStatusReply: httpd.HttpStatusReply{Code: "success", Message: "success"},
		Result: shared.BackupJobStatus{
			Name:        "job1",
			BackupJobId: "RID-1",
			State:       "finished",
		},
	})
	defer srv.Close()

	decoded, body, err := doReportShow(cfg(srv.URL), "job1", "RID-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(body) == 0 {
		t.Error("expected raw body to be populated")
	}
	if decoded.Result.BackupJobId != "RID-1" || decoded.Result.State != "finished" {
		t.Errorf("unexpected decoded response: %+v", decoded.Result)
	}
	if rs.lastMethod != "POST" || rs.lastPath != "/api/v1/report/restore/show" {
		t.Errorf("unexpected request method/path: %s %s", rs.lastMethod, rs.lastPath)
	}
	var sent httpd.ReportRestoreJob
	_ = json.Unmarshal(rs.lastBody, &sent)
	if sent.Name != "job1" || sent.JobId != "RID-1" {
		t.Errorf("unexpected request payload: %+v", sent)
	}
}

func TestDoReportShow_ServerError(t *testing.T) {
	srv, _ := newRecorder(t, 404, httpd.HttpStatusReply{Code: "error", Message: "not found"})
	defer srv.Close()

	_, _, err := doReportShow(cfg(srv.URL), "job1", "nonexistent")
	if err == nil {
		t.Fatal("expected error on 404 response")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected error to contain 'not found', got %q", err.Error())
	}
}

func TestDoReportShow_ConnectionError(t *testing.T) {
	srv, _ := newRecorder(t, 200, ReportShowResponse{})
	addr := srv.URL
	srv.Close()

	_, _, err := doReportShow(cfg(addr), "job1", "RID")
	if err == nil {
		t.Fatal("expected network error when server is unreachable")
	}
}
