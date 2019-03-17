package httpd

import (
	"github.com/julienschmidt/httprouter"
	"net/http"
)

// redirect to /docs_api/swagger.json
func handlerGETtlSwaggerJson(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	http.Redirect(w, r, "/docs_api/swagger.json", 301)
}

// redirect to /docs_api/swagger.yaml
func handlerGETtlSwaggerYaml(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	http.Redirect(w, r, "/docs_api/swagger.yaml", 301)
}
