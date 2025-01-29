# README #
This project was an attempt to build an opensource backup solution which provides file leval granularity of backups and is able to store resulting backups in a cloud provider's blob store, like WS S3, Azure Blob Storage or Google's GCP Cloud Storage.

It had a server which would take care of backups, restores and reporting and a command line client capable to connect to the server and request on demand backups, restores and reporting.
The client could also connect and see in realtime the progress of a backup. Additionally an HTTP API (used by the client) was documented using SWAGGER.
Supported platforms were Linux, FreeBSD and MS Windows. While MacOS was not integrated into the CI/CD pipeline, it would most likely has minor issues to fix before a build would be possible.

The project never completed the restore capability while the on-demand backup one was implemented, for the three mentioned object stores.
If you wish to continue then to get started you need to make a build

The CI/CD setup is in a separate repository. Jenkins for the actual CI/CD and some tooling for building the AWS AMIs for Linux/FreeBSD/Windows to be used by Jenkins. 

To do initial setup:

*  install Golang 1.22 and golangci-lint v1.64.4 https://github.com/golangci/golangci-lint/releases/tag/v1.63.4
*  set $GOPATH
*  install Make, Python3 , Python3 Virtualenv, Python3 pip
*  Clone repo

```
cd $GOPATH
mkdir -p src
cd src
git clone git@bitbucket.org:alexandru_ionica/cloudbackup.git
```

To do a build, run:
```
cd $GOPATH/srv/cloudbackup
make
```

# Re-Generating Documentation

Run on Linux/Unixes only:

```
make docs
```
The documentation is in the `documentation_src` folder but once the server is launched  (`./cloudbackup server start -c config.yaml`) the the documentation can be accessed at http://127.0.0.1:8080 or the network reachable IP + port of the server (if one was configured)

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