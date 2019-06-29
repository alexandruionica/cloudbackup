# README #
To do initial setup:

*  install Golang 1.10 and Glide - fetch binary from https://glide.sh/
*  set $GOPATH
*  install Make, Python3 , Python3 Virtualenv, Python3 pip
*  Clone repo

```
cd $GOPATH
mkdir -p src
cd src
git clone git@bitbucket.org:alexandru_ionica/cloudbackup.git
```
*  install:
    * errcheck (used to test for unhandled errors)
    * aligncheck (used to find inefficiently packed structs)
    * structcheck (used to check for unused struct fields)
    * varcheck (used to check for unused global variables and constants)
    * GoASTScanner (Inspects source code for security problems by scanning the Go AST)

```
cd $GOPATH/srv/cloudbackup
make testdeps
```         
* SafeSQL (for later, see go get github.com/stripe/safesql ) - looks for SQL injections
* Install dependencies, build, build dependencies and install (make future compile times shorter)

```
cd $GOPATH/srv/cloudbackup
make deps
make
go install
```

# Generating Documentation

Run on Linux/Unixes only:

```
make docs
```

# Required for running tests

Various credentials are needed for the tests which use object stores, like AWS S3.

```$xslt
export CLD_AWS_ACCESS_KEY_ID="REPLACE_WITH_KEY_ID"
export CLD_AWS_SECRET_ACCESS_KEY="REPLACE_WITH_SECRET"
export CLD_S3_BUCKET="aionica-tests"
export CLD_S3_REGION="us-east-1"
```
It's generally recommended to add them to a `.creds` file at the root of the repo as this filename is blackliested 
(via `.gitignore`) and then just execute once `source .creds` after starting a new terminal session (in which the tests
 will eventually be ran). You will most likely also have to copy said values into your IDE so it can run tests and give
  you the coverage report (integrated with your IDE, assuming it has such functionality).