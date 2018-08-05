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