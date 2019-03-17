package httpd

import (
	"github.com/julienschmidt/httprouter"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

func TestPageRootHttp(t *testing.T) {
	fakeSrvData := SrvData{httpsEnabled: false,
		Mutex: &sync.RWMutex{}}
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
	defer func() { _ = res.Body.Close() }()
	contents, err := ioutil.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("%s", err)
	}
	if string(contents) != expectedResponse {
		t.Fatalf("Response body was '%+v' while we were expecting '%+v'", string(contents), expectedResponse)
	}
}
