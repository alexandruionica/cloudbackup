package httpd

import (
	"testing"
	"cloudbackup/config"
	"cloudbackup/testutils"
	"cloudbackup/utils"
	"net/http"
	"net/http/httptest"
	"io/ioutil"
	"os"
	"sync"
	"reflect"
	"crypto/tls"
	"github.com/julienschmidt/httprouter"
)

const addr = "localhost:8080"
const addrSsl = "localhost:8443"

func TestNew(t *testing.T) {
	var compare = &SrvData{}
	path, err := utils.SetupTmpFileWithContent(testutils.MockYaml, "unittest_httpd_test_")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		err := os.Remove(path)
		if err != nil {
			t.Fatal(err)
		}
	}()
	cfgResult, _ := config.Load(path, false, &sync.RWMutex{})
	result := New(make(chan bool), make(chan bool), cfgResult, addr, false, "", "")
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
	path, err := utils.SetupTmpFileWithContent(testutils.MockYaml, "unittest_httpd_test_")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		err := os.Remove(path)
		if err != nil {
			t.Fatal(err)
		}
	}()
	cfgResult, _ := config.Load(path, false, &sync.RWMutex{})
	srv := New(make(chan bool), make(chan bool), cfgResult, addr, false, "", "")
	srv.Start()
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
	path, err := utils.SetupTmpFileWithContent(testutils.MockYaml, "unittest_httpd_test_")
	if err != nil {
		t.Fatal(err)
	}
	var sslCert, sslKey = testutils.SetupSslCertAndKey("unittest_httpd_test_", t)
	defer func() {
		err := os.Remove(path)
		if err != nil {
			t.Fatal(err)
		}
		err = os.Remove(sslCert)
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
	srv := New(make(chan bool), make(chan bool), cfgResult, addrSsl, true, sslCert, sslKey)
	srv.Start()

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

func TestPageRootHttp(t *testing.T) {
	fakeSrvData := SrvData{httpsEnabled: false,
							Mutex: &sync.RWMutex{},}
	router := httprouter.New()
	router.GET("/", fakeSrvData.handlerRoot)
	ts := httptest.NewServer(router)
	defer ts.Close()

	res, err := http.Get(ts.URL)
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != 200 {
		t.Fatalf("Expected HTTP 200 when requesting '/' but got instead %+v", res.StatusCode)
	}

	// test if response body for / is what we expect
	expectedResponse := "HTTP server is running\n"
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
//	var path = utils.SetupTmpFileWithContent(testutils.MockYaml, "unittest_httpd_test_", t)
//	defer func() {
//		err := os.Remove(path)
//		if err != nil {
//			t.Fatal(err)
//		}
//	}()
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