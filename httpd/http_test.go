package httpd

import (
	"testing"
	"cloudbackup/config"
	"cloudbackup/testutils"
	"net/http"
	"net/http/httptest"
	"strconv"
	"io/ioutil"
	"os"
	"sync"
)

const host = "localhost"
const port = 8080

func TestNew(t *testing.T) {
	var compare = &SrvData{}
	var path = testutils.SetupFakeFile(testutils.MockYaml, "unittest_httpd_test_", t)
	defer func() {
		err := os.Remove(path)
		if err != nil {
			t.Fatal(err)
		}
	}()
	cfgResult, _ := config.Load(path, false, &sync.Mutex{})
	result := New(make(chan bool), make(chan bool), cfgResult, port, host)
	// we just ensure that we have the same type in the result as what we expect
	if compare == result {
	}
	if result.serverExiting {
		t.Errorf("Expected serverExiting to be 'false' but it was '%+v'", result.serverExiting)
	}
	address := "localhost:8080"
	if result.httpsrv.Addr != address {
		t.Errorf("Expected result.httpsrv.Addr to be '%v' but it was %+v", address, result.httpsrv.Addr)
	}
}

func TestStartAndClose(t *testing.T) {
	var path = testutils.SetupFakeFile(testutils.MockYaml, "unittest_httpd_test_", t)
	defer func() {
		err := os.Remove(path)
		if err != nil {
			t.Fatal(err)
		}
	}()
	cfgResult, _ := config.Load(path, false, &sync.Mutex{})
	srv := New(make(chan bool), make(chan bool), cfgResult, port, host)
	srv.Start()
	_, err := http.Get("http://" + host + ":" + strconv.Itoa(port) + "/")
	if err != nil {
		t.Fatal(err)
	}
	// "manual" cleanup
	//srv.serverExiting = true
	//srv.httpsrv.Close()
	srv.Stop()
	_, err = http.Get("http://" + host + ":" + strconv.Itoa(port) + "/")
	if err == nil {
		t.Fatalf("After stopping the webserver we attempted to fetch a url and this should have produced an " +
			"error but instead it succeeded which means the server did not stop")
	}
}

func TestPageRoot(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(pageRoot))
	defer ts.Close()

	res, err := http.Get(ts.URL)
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != 200 {
		t.Fatalf("Expected HTTP 200 when requesting '/' but got instead %+v", res.StatusCode)
	}

	// test if response body for / is what we expect
	expectedResponse := "Http server is running\n"
	defer func() {_ = res.Body.Close()}()
	contents, err := ioutil.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("%s", err)
	}
	if string(contents) != expectedResponse {
		t.Fatalf("Response body was '%+v' while we were expecting '%+v'", string(contents), expectedResponse)
	}
}

//func TestStop(t *testing.T) {
//	// the default server Mux remains initialised from previous tests so we need to clean it up first
//	http.DefaultServeMux = http.NewServeMux()
//	srv := New(make(chan bool), make(chan bool), config.Load("/etc/just/a/config.file"), port, host)
//	srv.Start()
//	srv.Stop()
//	_, err := http.Get("http://" + host + ":" + strconv.Itoa(port) + "/")
//	if err == nil {
//		t.Fatalf("After stopping the webserver we attempted to fetch a url and this should have produced an " +
//			"error but instead it succeeded which means the server did not stop")
//	}
//
//}