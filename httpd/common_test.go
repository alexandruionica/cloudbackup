package httpd

import (
	"cloudbackup/config"
	"cloudbackup/shared"
	"cloudbackup/testutils"
	"encoding/json"
	"github.com/julienschmidt/httprouter"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// basic auth test - if not supplying credentials then 401 is returned
func TestBasicAuth1(t *testing.T) {
	// load config file
	path, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_httpd_common_test_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	configuration, err := config.Load(path, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}

	fakeSrvData := SrvData{httpsEnabled: false,
		Mutex:     &sync.RWMutex{},
		globalcfg: configuration}

	req := httptest.NewRequest("POST", "http://example.com/foo", nil)
	w := httptest.NewRecorder()
	handle := func(http.ResponseWriter, *http.Request, httprouter.Params) {
	}

	auth := fakeSrvData.BasicAuth(handle)
	auth(w, req, []httprouter.Param{})

	resp := w.Result()
	// body, _ := ioutil.ReadAll(resp.Body)

	if resp.StatusCode != 401 {
		t.Fatalf("HTTP response when not supplying validation was not 401 but %d", resp.StatusCode)
	}
}

// basic auth test - valid user + pass
func TestBasicAuth2(t *testing.T) {
	//
	username := "testuser1"
	password := "HV}H/y?<9$]Z5N4N" //nolint:gosec
	// load config file
	path, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_httpd_common_test_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	configuration, err := config.Load(path, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}

	fakeSrvData := SrvData{httpsEnabled: false,
		Mutex:     &sync.RWMutex{},
		globalcfg: configuration}

	req := httptest.NewRequest("POST", "http://example.com/foo", nil)
	req.SetBasicAuth(username, password)
	w := httptest.NewRecorder()
	handle := func(http.ResponseWriter, *http.Request, httprouter.Params) {
	}

	auth := fakeSrvData.BasicAuth(handle)
	auth(w, req, []httprouter.Param{})

	resp := w.Result()

	if resp.StatusCode != 200 {
		t.Fatalf("HTTP response when supplying valid credentials was not 200 but %d", resp.StatusCode)
	}
}

// basic auth test - valid user + INVALID pass
func TestBasicAuth3(t *testing.T) {
	//
	username := "testuser1"
	password := "@#$@#$" //nolint:gosec
	// load config file
	path, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_httpd_common_test_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	configuration, err := config.Load(path, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}

	fakeSrvData := SrvData{httpsEnabled: false,
		Mutex:     &sync.RWMutex{},
		globalcfg: configuration}

	req := httptest.NewRequest("POST", "http://example.com/foo", nil)
	req.SetBasicAuth(username, password)
	w := httptest.NewRecorder()
	handle := func(http.ResponseWriter, *http.Request, httprouter.Params) {
	}

	auth := fakeSrvData.BasicAuth(handle)
	auth(w, req, []httprouter.Param{})

	resp := w.Result()

	if resp.StatusCode != 401 {
		t.Fatalf("HTTP response when supplying INVALID credentials was not 401 but %d", resp.StatusCode)
	}
}

// basic auth test - INVALID user +  pass
func TestBasicAuth4(t *testing.T) {
	//
	username := "justauser"
	password := "HV}H/y?<9$]Z5N4N" //nolint:gosec
	// load config file
	path, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_httpd_common_test_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	configuration, err := config.Load(path, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}

	fakeSrvData := SrvData{httpsEnabled: false,
		Mutex:     &sync.RWMutex{},
		globalcfg: configuration}

	req := httptest.NewRequest("POST", "http://example.com/foo", nil)
	req.SetBasicAuth(username, password)
	w := httptest.NewRecorder()
	handle := func(http.ResponseWriter, *http.Request, httprouter.Params) {
	}

	auth := fakeSrvData.BasicAuth(handle)
	auth(w, req, []httprouter.Param{})

	resp := w.Result()

	if resp.StatusCode != 401 {
		t.Fatalf("HTTP response when supplying INVALID credentials was not 401 but %d", resp.StatusCode)
	}
}

// basic auth test - no user + pair defined in the config database
func TestBasicAuth5(t *testing.T) {
	//
	username := "justauser"
	password := "some-pass" //nolint:gosec
	// load config file
	path, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_httpd_common_test_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	configuration, err := config.Load(path, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}

	// ensure we don't have any user + pass defined
	configuration.Config.User = make([]shared.ConfigUser, 0)

	fakeSrvData := SrvData{httpsEnabled: false,
		Mutex:     &sync.RWMutex{},
		globalcfg: configuration}

	req := httptest.NewRequest("POST", "http://example.com/foo", nil)
	req.SetBasicAuth(username, password)
	w := httptest.NewRecorder()
	handle := func(http.ResponseWriter, *http.Request, httprouter.Params) {
	}

	auth := fakeSrvData.BasicAuth(handle)
	auth(w, req, []httprouter.Param{})

	resp := w.Result()

	if resp.StatusCode != 401 {
		t.Fatalf("HTTP response when supplying INVALID credentials was not 401 but %d", resp.StatusCode)
	}
}

// calling CheckAccess() on unauthenticated sessions should return HTTP response code 500
func TestCheckAccess1(t *testing.T) {
	// load config file
	path, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_httpd_common_test_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	configuration, err := config.Load(path, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}

	fakeSrvData := SrvData{httpsEnabled: false,
		Mutex:     &sync.RWMutex{},
		globalcfg: configuration}

	req := httptest.NewRequest("GET", "http://example.com/api/v1/config", nil)
	// req.SetBasicAuth(username, password)
	w := httptest.NewRecorder()
	handle := func(http.ResponseWriter, *http.Request, httprouter.Params) {
	}

	acc := fakeSrvData.CheckAccess(handle)
	acc(w, req, []httprouter.Param{})

	resp := w.Result()

	if resp.StatusCode != 500 {
		t.Fatalf("calling CheckAccess() on unauthenticated sessions should return HTTP response code 500 but "+
			"we got %d", resp.StatusCode)
	}
}

// authenticated session with 'write' permissions is granted access to anything
func TestCheckAccess2(t *testing.T) {
	// this user has "write" access which means access to anything
	username := "testuser1"
	password := "HV}H/y?<9$]Z5N4N" //nolint:gosec
	// load config file
	path, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_httpd_common_test_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	configuration, err := config.Load(path, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}

	fakeSrvData := SrvData{httpsEnabled: false,
		Mutex:     &sync.RWMutex{},
		globalcfg: configuration}

	req := httptest.NewRequest("POST", "http://example.com/foo", nil)
	req.SetBasicAuth(username, password)
	w := httptest.NewRecorder()
	handle := func(http.ResponseWriter, *http.Request, httprouter.Params) {
	}

	acc := fakeSrvData.BasicAuth(fakeSrvData.CheckAccess(handle))
	acc(w, req, []httprouter.Param{})

	resp := w.Result()
	body, _ := ioutil.ReadAll(resp.Body)

	if resp.StatusCode != 200 {
		t.Fatalf("calling CheckAccess() on authenticated session with valid 'write' access user should return "+
			"HTTP response code 200 but we got %d. Response body was: '%s'", resp.StatusCode, body)
	}
}

// authenticated session with 'read' permissions is NOT granted access to paths which have not been white listed
func TestCheckAccess3(t *testing.T) {
	// this user has "write" access which means access to anything
	username := "testuser2"
	password := "Oonaawai8Eep]eethe8eefa$" //nolint:gosec
	// load config file
	path, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_httpd_common_test_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	configuration, err := config.Load(path, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}

	fakeSrvData := SrvData{httpsEnabled: false,
		Mutex:     &sync.RWMutex{},
		globalcfg: configuration}

	req := httptest.NewRequest("POST", "http://example.com/foo", nil)
	req.SetBasicAuth(username, password)
	w := httptest.NewRecorder()
	handle := func(http.ResponseWriter, *http.Request, httprouter.Params) {
	}

	acc := fakeSrvData.BasicAuth(fakeSrvData.CheckAccess(handle))
	acc(w, req, []httprouter.Param{})

	resp := w.Result()
	body, _ := ioutil.ReadAll(resp.Body)

	if resp.StatusCode != 403 {
		t.Fatalf("calling CheckAccess() on authenticated session with 'read' access user is NOT granted access"+
			" to paths which have not been white listed and HTTP code 403 is returned but we got %d. Response "+
			"body was: '%s'", resp.StatusCode, body)
	}
}

// authenticated session with 'read' permissions is granted access to paths which HAVE been white listed
func TestCheckAccess4(t *testing.T) {
	// this user has "write" access which means access to anything
	username := "testuser2"
	password := "Oonaawai8Eep]eethe8eefa$" //nolint:gosec
	// load config file
	path, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_httpd_common_test_")
	// remove tmpfile which holds the yaml as the config has been parsed and loaded
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)

	configuration, err := config.Load(path, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("Could not load fake config file. Error was: %s", err)
	}

	fakeSrvData := SrvData{httpsEnabled: false,
		Mutex:     &sync.RWMutex{},
		globalcfg: configuration}

	req := httptest.NewRequest("GET", "http://example.com/api/v1/config", nil)
	req.SetBasicAuth(username, password)
	w := httptest.NewRecorder()
	handle := func(http.ResponseWriter, *http.Request, httprouter.Params) {
	}

	acc := fakeSrvData.BasicAuth(fakeSrvData.CheckAccess(handle))
	acc(w, req, []httprouter.Param{})

	resp := w.Result()
	body, _ := ioutil.ReadAll(resp.Body)

	if resp.StatusCode != 200 {
		t.Fatalf("calling CheckAccess() on authenticated session with 'read' access user is granted access"+
			" to paths which HAVE been white listed and HTTP code 200 is returned but we got %d. Response "+
			"body was: '%s'", resp.StatusCode, body)
	}
}

// with valid json
func TestValidateJsonHTTPInput1(t *testing.T) {
	input := "[{\"Id\": 100, \"Name\": \"Go\"}, {\"Id\": 200, \"Name\": \"Java\"}]"
	req := httptest.NewRequest("POST", "http://example.com/foo", strings.NewReader(input))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	_, err := ValidateJsonHTTPInput(w, req)

	if err != nil {
		t.Fatalf("ValidateJsonHTTPInput() did not validate string which is json. Received error was: %s", err)
	}
}

// with invalid json
func TestValidateJsonHTTPInput2(t *testing.T) {
	input := "[{\"Id\": 100, \"Name\": \"Go\"}, {\"Id\": 200, \"Name\" \"Java\"}]"
	req := httptest.NewRequest("POST", "http://example.com/foo", strings.NewReader(input))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	_, err := ValidateJsonHTTPInput(w, req)

	if err == nil {
		t.Fatal("ValidateJsonHTTPInput() did not fail to validate string which was NOT json. ")
	}
}

// with valid json but invalid header for Content-Type
func TestValidateJsonHTTPInput3(t *testing.T) {
	input := "[{\"Id\": 100, \"Name\": \"Go\"}, {\"Id\": 200, \"Name\": \"Java\"}]"
	req := httptest.NewRequest("POST", "http://example.com/foo", strings.NewReader(input))
	req.Header.Set("Content-Type", "application/jsonNAS")
	w := httptest.NewRecorder()

	_, err := ValidateJsonHTTPInput(w, req)

	if err == nil {
		t.Fatal("ValidateJsonHTTPInput() did not fail to validate string which was json but had incorrect value for Content-Type instead of 'application/json'")
	}

	expectedMsg := "POST 'Content-Type' is not of type 'application/json'"
	if err.Error() != expectedMsg {
		t.Fatalf("Expected error to be: '%s' but it was '%s'", expectedMsg, err.Error())
	}
}

func TestJSONError1(t *testing.T) {
	input := "[{\"Id\": 100, \"Name\": \"Go\"}, {\"Id\": 200, \"Name\": \"Java\"}]"
	req := httptest.NewRequest("POST", "http://example.com/foo", strings.NewReader(input))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	codestring := "codestring"
	messagestring := "messagestring"
	JSONError(w, 300, codestring, messagestring)

	resp := w.Result()
	body, _ := ioutil.ReadAll(resp.Body)

	if resp.StatusCode != 300 {
		t.Fatalf("calling JSONError() passing http code '300' but got as reply HTTP code '%d' and response "+
			"body '%s'", resp.StatusCode, body)
	}

	// ValidateJsonHTTPInput() succeeding means that reply contains both 'Content-Type' = 'application/json' and also
	// valid json
	_, err := ValidateJsonHTTPInput(w, req)
	if err != nil {
		t.Fatalf("JSONError() output was not validated by ValidateJsonHTTPInput(). Received error was '%s'", err)
	}

	var bodyStruct struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	// no point in checking again if json correctly unMarshalls
	_ = json.Unmarshal(body, &bodyStruct)
	if bodyStruct.Code != codestring {
		t.Fatalf("Response from JSONError() was expected to have code='%s' but instead it's value was: %s",
			codestring, bodyStruct.Code)
	}

	if bodyStruct.Message != messagestring {
		t.Fatalf("Response from JSONError() was expected to have message='%s' but instead it's value was: %s",
			messagestring, bodyStruct.Message)
	}
}

func TestJSONSuccess1(t *testing.T) {
	input := "[{\"Id\": 100, \"Name\": \"Go\"}, {\"Id\": 200, \"Name\": \"Java\"}]"
	req := httptest.NewRequest("POST", "http://example.com/foo", strings.NewReader(input))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	codestring := "codestring"
	messagestring := "messagestring"
	JSONSuccess(w, codestring, messagestring)

	resp := w.Result()
	body, _ := ioutil.ReadAll(resp.Body)

	if resp.StatusCode != 200 {
		t.Fatalf("reply from JSONSuccess() didn't have http code '200' but got as reply HTTP code '%d' and "+
			"response body '%s'", resp.StatusCode, body)
	}

	// ValidateJsonHTTPInput() succeeding means that reply contains both 'Content-Type' = 'application/json' and also
	// valid json
	_, err := ValidateJsonHTTPInput(w, req)
	if err != nil {
		t.Fatalf("JSONSuccess() output was not validated by ValidateJsonHTTPInput(). Received error was '%s'", err)
	}

	var bodyStruct struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	// no point in checking again if json correctly unMarshalls
	_ = json.Unmarshal(body, &bodyStruct)
	if bodyStruct.Code != codestring {
		t.Fatalf("Response from JSONSuccess() was expected to have code='%s' but instead it's value was: %s",
			codestring, bodyStruct.Code)
	}

	if bodyStruct.Message != messagestring {
		t.Fatalf("Response from JSONSuccess() was expected to have message='%s' but instead it's value was: %s",
			messagestring, bodyStruct.Message)
	}
}

func TestJSONSuccessWithResult1(t *testing.T) {
	input := "[{\"Id\": 100, \"Name\": \"Go\"}, {\"Id\": 200, \"Name\": \"Java\"}]"
	req := httptest.NewRequest("POST", "http://example.com/foo", strings.NewReader(input))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	codestring := "codestring"
	messagestring := "messagestring"
	resultStruct := struct {
		Key1 string
		Key2 string
	}{
		"somevalue1",
		"somevalue2",
	}
	JSONSuccessWithResult(w, codestring, messagestring, resultStruct)

	resp := w.Result()
	body, _ := ioutil.ReadAll(resp.Body)

	if resp.StatusCode != 200 {
		t.Fatalf("reply from JSONSuccessWithResult() didn't have http code '200' but got as reply HTTP code '%d' and "+
			"response body '%s'", resp.StatusCode, body)
	}

	// ValidateJsonHTTPInput() succeeding means that reply contains both 'Content-Type' = 'application/json' and also
	// valid json
	_, err := ValidateJsonHTTPInput(w, req)
	if err != nil {
		t.Fatalf("JSONSuccessWithResult() output was not validated by ValidateJsonHTTPInput(). Received error was '%s'", err)
	}

	var bodyStruct struct {
		Code    string      `json:"code"`
		Message string      `json:"message"`
		Result  interface{} `json:"result"`
	}
	// no point in checking again if json correctly unMarshalls
	_ = json.Unmarshal(body, &bodyStruct)
	if bodyStruct.Code != codestring {
		t.Fatalf("Response from JSONSuccessWithResult() was expected to have code='%s' but instead it's value was: %s",
			codestring, bodyStruct.Code)
	}

	if bodyStruct.Message != messagestring {
		t.Fatalf("Response from JSONSuccessWithResult() was expected to have message='%s' but instead it's value was: %s",
			messagestring, bodyStruct.Message)
	}

	m := make(map[string]string)
	m["Key1"] = "somevalue1"
	m["Key2"] = "somevalue2"
	if bodyStruct.Result == "" || bodyStruct.Result == nil {
		t.Fatalf("Response from JSONSuccessWithResult() was expected to have result= non empty but instead it's value was: %+v",
			bodyStruct.Result)
	}
}
