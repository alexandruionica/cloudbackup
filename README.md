# README #
To do initial setup:

* install Glide - fetch binary from https://glide.sh/
* install Make
+ install:
  * errcheck (used to test for unhandled errors)
  * aligncheck (used to find inefficiently packed structs)
  * structcheck (used to check for unused struct fields)
  * varcheck (used to check for unused global variables and constants)
  * GoASTScanner (Inspects source code for security problems by scanning the Go AST) 
```
cd $GOPATH
mkdir -p src
go get github.com/kisielk/errcheck
go install github.com/kisielk/errcheck
go get github.com/opennota/check/cmd/aligncheck
go install github.com/opennota/check/cmd/aligncheck
go get github.com/opennota/check/cmd/structcheck
go install github.com/opennota/check/cmd/structcheck
go get github.com/opennota/check/cmd/varcheck
go install github.com/opennota/check/cmd/varcheck
go get github.com/GoASTScanner/gas/cmd/gas/...
go install github.com/GoASTScanner/gas/cmd/gas
```
- SafeSQL (for later, see go get github.com/stripe/safesql ) - looks for SQL injections
* Clone repo, install dependencies, build, build dependencies and install (make future compile times shorter)
```
cd $GOPATH
mkdir -p src
cd src
git clone git@bitbucket.org:alexandru_ionica/cloudbackup.git
cd cloudbackup
make deps
make
go install
```