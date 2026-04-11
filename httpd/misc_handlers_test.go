package httpd

import (
	"github.com/julienschmidt/httprouter"
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

	// do not follow redirects so we can inspect the Location header
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	res, err := client.Get(ts.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()

	if res.StatusCode != http.StatusFound {
		t.Fatalf("Expected HTTP 302 when requesting '/' but got instead %+v", res.StatusCode)
	}
	if loc := res.Header.Get("Location"); loc != "/ui/" {
		t.Fatalf("Expected Location header '/ui/' but got '%s'", loc)
	}
}
