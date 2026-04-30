---
name: add-rest-endpoint
description: Add a new REST endpoint under the /api prefix — handler, route registration, basic-auth wiring, Swagger entry, and a unit test. Use when the user asks to expose new server functionality over HTTP.
---

# Add a REST endpoint

This repository's HTTP server is `httpd/`, using `julienschmidt/httprouter`. All
authenticated routes go through `BasicAuth(CheckAccess(...))`.

## Inputs to ask for if not given
- HTTP method + path (e.g. `POST /api/backup/foo`)
- Request body shape (or "no body" for GET)
- Response shape on success / error
- Whether the endpoint is admin-only or read-only (affects what `CheckAccess` allows)

## Steps

1. **Pick a file**. Group by resource: backup → `httpd/api_rest_backup.go`,
   restore → `httpd/api_rest_restore.go`, config → `httpd/api_rest_config.go`,
   reports → `httpd/api_rest_report.go`. Add a sibling file (`api_rest_<noun>.go`)
   if the resource is new.

2. **Write the handler**. Match the existing signature:
   `func (srvSrc SrvData) handlerXxx(w http.ResponseWriter, r *http.Request, _ httprouter.Params)`.
   Increment `globals.Stats.IncrementRoutines("httpd_handlers")` at the top
   with a deferred decrement (see `handlerPostBackupStart` for the pattern).
   Use `ValidateJsonHTTPInput` for POST bodies and `JSONError` / `JSONSuccessWithResult`
   for replies — never write JSON inline.

3. **Register the route** in `httpd/init.go`. Add one line wrapped in
   `srv.BasicAuth(srv.CheckAccess(...))`. Keep grouped near related routes.

4. **Update Swagger** in `webstatic/docs_api/swagger.yaml`. Add a path block
   matching the new route, request/response schemas, and the `BasicAuth`
   security entry. Then run `make docs` to regenerate `swagger.json`.

5. **Write a handler test** in `httpd/api_rest_<noun>_test.go`. Use the
   pattern from `api_rest_restore_test.go`: build a mock `SrvData`, call the
   handler directly with a synthesized `*http.Request` and a
   `httptest.ResponseRecorder`, assert status code + decoded JSON body.

## Validation
- `go build -mod=vendor ./...`
- `go test -mod=vendor -race ./httpd/...`
- `make testcp` — golangci-lint with gosec, must report 0 issues.

## Pitfalls specific to this repo
- The handler runs in its own goroutine; do not hold `configuration.Mutex` across
  any I/O. Always `GetCopyWithLock(...)` first.
- `CheckAccess` reads roles per-user from config; if the new endpoint should
  be writable by non-admins, add an entry to the role map (see `httpd/common.go`).
- Integration tests live in `integration_tests/rest_api*.py`. Adding a Python
  test there is encouraged for non-trivial endpoints.
