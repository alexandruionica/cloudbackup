package httpd

import (
	"net/http"
	"github.com/julienschmidt/httprouter"
	"encoding/json"
	"fmt"
)

type BackupJobName struct {
	Name string `required:"true" json:"name"`
}

type BackupJobStarted struct {
	Name string `json:"name"`
	Id string `json:"id"`
}

func (srvSrc SrvData) handlerPostBackupStart(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	bodyBytes, err := ValidateJsonHTTPInput(w, r)
	if err != nil {
		// the ValidateJsonHTTPInput takes care of sending a reply to the user so there isn't much else to do here
		return
	}
	var decodedJson BackupJobName
	err = json.Unmarshal(bodyBytes, &decodedJson)
	if decodedJson.Name == "" {
		JSONError(w, http.StatusBadRequest, HttpErrInvalidJson, fmt.Sprint("'name' key is mandatory. The name" +
			" is needed in order to know what backup job you're requesting to be started"))
		return
	}
	srv := srvSrc.GetWithLock(loggingContext + ".handlerPostBackupStart")
	config := srv.globalcfg.GetWithLock(loggingContext + ".handlerPutConfig")
	found := false
	for _, backup := range config.Backup {
		if backup.Name == decodedJson.Name {
			found = true
		}
	}
	if found == false {
		JSONError(w, http.StatusNotFound, HttpErrNotFound, fmt.Sprintf("No backup job was found matching name:" +
			" %s", decodedJson.Name))
		return
	}
	// TODO -  check if a backup is already running
	//  if yes then reply with   HttpErrIncorrectClientData and a 400 code
	// else attempt to start backup
	//   TODO - add code to communicate with scheduler goroutine
	result := BackupJobStarted{
		Name: decodedJson.Name,
		Id: "UUID-TO-ADD",
	}
	JSONSuccessWithResult(w, "success", "successfully started backup", result)
}