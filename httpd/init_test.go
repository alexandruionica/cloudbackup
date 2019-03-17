package httpd

import (
	"cloudbackup/config"
	"cloudbackup/shared"
	"cloudbackup/testutils"
	"crypto/tls"
	"net/http"
	"os"
	"reflect"
	"sync"
	"testing"
)

const addr = "localhost:8080"
const addrSsl = "localhost:8443"

func TestNew(t *testing.T) {
	var compare = &SrvData{}
	path, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_httpd_init_test_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	cfgResult, _ := config.Load(path, false, &sync.RWMutex{})
	//  struct containing the channels needed to communicate with the scheduler in order to start/stop Backups
	commWithSchedulerForBackup := &shared.CommWithSchedulerForBackup{}
	commWithSchedulerForBackup.Init()
	// backupJobState contains the state of all running backup jobs plus it has some handy methods
	backupJobsState := &shared.BackupJobsState{}

	result := New(make(chan bool), make(chan bool), cfgResult, addr, false, "", "",
		commWithSchedulerForBackup, backupJobsState)
	// we just ensure that we have the same type in the result as what we expect
	if reflect.ValueOf(compare).Kind() != reflect.ValueOf(result).Kind() {
		t.Errorf("Variable type returned by New()")
	}
	if result.serverExiting {
		t.Errorf("Expected serverExiting to be 'false' but it was '%+v'", result.serverExiting)
	}
	address := "localhost:8080"
	if result.httpsrv.Addr != address {
		t.Errorf("Expected result.httpsrv.Addr to be '%v' but it was %+v", address, result.httpsrv.Addr)
	}
}

func TestStartAndCloseHttp(t *testing.T) {
	path, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_httpd_init_test_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	cfgResult, _ := config.Load(path, false, &sync.RWMutex{})
	commWithSchedulerForBackup := &shared.CommWithSchedulerForBackup{}
	commWithSchedulerForBackup.Init()
	// backupJobState contains the state of all running backup jobs plus it has some handy methods
	backupJobsState := &shared.BackupJobsState{}

	srv := New(make(chan bool), make(chan bool), cfgResult, addr, false, "", "",
		commWithSchedulerForBackup, backupJobsState)
	srv.Start()
	// check several times is port is being listened on
	err := testutils.WaitForServerToStart("127.0.0.1", "8080", t)
	if err != nil {
		t.Fatal(err)
	}
	_, err = http.Get("http://" + addr + "/")
	if err != nil {
		t.Fatal(err)
	}
	// "manual" cleanup
	//srv.serverExiting = true
	//srv.httpsrv.Close()
	srv.Stop()
	_, err = http.Get("http://" + addr + "/")
	if err == nil {
		t.Fatalf("After stopping the webserver we attempted to fetch a url and this should have produced an " +
			"error but instead it succeeded which means the server did not stop")
	}
}

func TestStartAndCloseHttps(t *testing.T) {
	path, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_httpd_init_test_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	var sslCert, sslKey = testutils.SetupSslCertAndKey("unittest_httpd_test_", t)
	defer func() {
		err := os.Remove(sslCert)
		if err != nil {
			t.Fatal(err)
		}
		err = os.Remove(sslKey)
		if err != nil {
			t.Fatal(err)
		}
	}()
	http.DefaultServeMux = http.NewServeMux()
	cfgResult, _ := config.Load(path, false, &sync.RWMutex{})
	commWithSchedulerForBackup := &shared.CommWithSchedulerForBackup{}
	commWithSchedulerForBackup.Init()
	// backupJobState contains the state of all running backup jobs plus it has some handy methods
	backupJobsState := &shared.BackupJobsState{}

	srv := New(make(chan bool), make(chan bool), cfgResult, addrSsl, true, sslCert, sslKey,
		commWithSchedulerForBackup, backupJobsState)
	srv.Start()

	// check several times is port is being listened on
	err := testutils.WaitForServerToStart("127.0.0.1", "8443", t)
	if err != nil {
		t.Fatal(err)
	}
	// disable SSL cert verification as we're using a self signed cert
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	client := &http.Client{Transport: tr}
	_, err = client.Get("https://" + addrSsl + "/")
	if err != nil {
		t.Fatal(err)
	}
	srv.Stop()
	_, err = client.Get("https://" + addrSsl + "/")
	if err == nil {
		t.Fatalf("After stopping the webserver we attempted to fetch a url and this should have produced an " +
			"error but instead it succeeded which means the server did not stop")
	}
}

//func TestStop(t *testing.T) {
//	// the default server Mux remains initialised from previous tests so we need to clean it up first
//	http.DefaultServeMux = http.NewServeMux()
//	path, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_httpd_init_test_")
//	// remove tmpfile which holds the yaml as the config has been parsed and loaded
//	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)
//	cfgResult, _ := config.Load(path, false, &sync.RWMutex{})
//	srv := New(make(chan bool), make(chan bool), cfgResult, addr, false, "", "")
//	srv.Start()
//	srv.Stop()
//	_, err := http.Get("http://" + addr + "/")
//	if err == nil {
//		t.Fatalf("After stopping the webserver we attempted to fetch a url and this should have produced an " +
//			"error but instead it succeeded which means the server did not stop")
//	}
//
//}
