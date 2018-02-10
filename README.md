# README #
To do initial setup:

- install Glide - fetch binary from https://glide.sh/

- install Make

- install:
  - errcheck (used to test for unhandled errors)
  - aligncheck (used to find inefficiently packed structs)
  - structcheck
  - varcheck
```
cd $GOPATH
mkdir -p src
go get https://github.com/kisielk/errcheck
go install github.com/kisielk/errcheck
go get github.com/opennota/check/cmd/aligncheck
go install github.com/opennota/check/cmd/aligncheck
go get github.com/opennota/check/cmd/structcheck
go install github.com/opennota/check/cmd/structcheck
go get github.com/opennota/check/cmd/varcheck
go install github.com/opennota/check/cmd/varcheck
```

- aligncheck / structcheck / varcheck


- SafeSQL (for later, see go get github.com/stripe/safesql ) - looks for SQL injections
- Clone repo, install dependencies, build
```
cd $GOPATH
mkdir -p src
cd src
git clone git@bitbucket.org:alexandru_ionica/cloudbackup.git
cd cloudbackup
make deps
make
```