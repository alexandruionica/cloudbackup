# README #
This project was an attempt to build an opensource backup solution which provides file level granularity of backups and is able to store resulting backups in a cloud provider's blob store, like AWS S3, Azure Blob Storage or Google's GCP Cloud Storage.
The project was a continuation of a tool(https://bitbucket.org/alexandru_ionica/s3backuptool/) a built many years ago(in 2016), in Python. This new attempt was in GO and was setup from the get go to use modern coding practices.

It had a server which would take care of backups, restores and reporting and a command line client capable to connect to the server and request on demand backups, restores and reporting.
The client could also connect and see in realtime the progress of a backup. Additionally an HTTP API (used by the client) was documented using SWAGGER.
Supported platforms were Linux, FreeBSD and MS Windows. While MacOS was not integrated into the CI/CD pipeline, it would most likely has minor issues to fix before a build would be possible.

The project never completed the restore capability while the on-demand backup one was implemented, for the three mentioned object stores.
If you wish to continue then to get started you need to make a build

The CI/CD setup is [in a separate repository](https://bitbucket.org/alexandru_ionica/cloudbackup_infrastructure/src/master/). Jenkins for the actual CI/CD and some tooling for building the AWS AMIs for Linux/FreeBSD/Windows to be used by Jenkins. 

To do initial setup:

*  install Golang 1.22 and golangci-lint v1.64.4 https://github.com/golangci/golangci-lint/releases/tag/v1.63.4
*  set $GOPATH
*  install Make, Python3 >=3.12.3 , Python3 Virtualenv, Python3 pip
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

## LLM usage ##

Code submitted before April 2026 was produced in "classical ways" and represents the vast majority of the codebase. 

Submissions starting with April 2026 represent code produced with agentic LLMs and have allowed to:

* add functionality in areas where my expertise is very limited like Javascript/Typescript web UIs.

* work to progress in areas where I didn't have any more significant time to invest 

## Quick demo ##

Watch https://www.youtube.com/watch?v=wyoO3pm_fmY for a quick demo of the project in action, showing CLI usage and access to documentation.

For a view of the web UI see https://youtu.be/EFjg5-VDSu8 .

# Required for running tests

Various credentials are needed for the tests which use object stores, like AWS S3.


```
# AWS S3 credentials for integration tests
export CLD_AWS_ACCESS_KEY_ID="REPLACE_WITH_KEY_ID"
export CLD_AWS_SECRET_ACCESS_KEY="REPLACE_WITH_SECRET"
export CLD_S3_BUCKET="aionica-tests"
export CLD_S3_REGION="us-east-1"

# GCP Storage credentials for integration tests
export CLD_GCP_TYPE="service_account"
export CLD_GCP_PROJECT_ID="FILL_IN"
export CLD_GCP_PRIVATE_KEY_ID="FILL_IN"
export CLD_GCP_PRIVATE_KEY='FILL_IN'
export CLD_GCP_CLIENT_EMAIL="FILL_IN"
export CLD_GCP_CLIENT_ID="FILL_IN"
export CLD_GCP_AUTH_URI="FILL_IN"
export CLD_GCP_TOKEN_URI="FILL_IN"
export CLD_GCP_AUTH_PROVIDER_X509_CERT_URL="FILL_IN"
export CLD_GCP_CLIENT_X509_CERT_URL="FILL_IN"
export CLD_GCP_STORAGE_BUCKET="aionica-tests"

# Azure Blobs credentials for integration tests
export CLD_AZURE_STORAGE_ACCOUNT='FILL_IN'
export CLD_AZURE_STORAGE_ACCESS_KEY='FILL_IN'
export CLD_AZURE_STORAGE_CONTAINER='FILL_IN'
```

It's generally recommended to add them to a `.creds` file at the root of the repo as this filename is blackliested 
(via `.gitignore`) and then just execute once `source .creds` after starting a new terminal session (in which the tests
 will eventually be ran). You will most likely also have to copy said values into your IDE so it can run tests and give
  you the coverage report (integrated with your IDE, assuming it has such functionality).

# Re-Generating Documentation

Run on Linux/Unixes only:

```
make docs
```
The documentation is in the `documentation_src` folder but once the server is launched  (`./cloudbackup server start -c config.yaml`) the the documentation can be accessed at http://127.0.0.1:8080/docs or the network reachable IP + port of the server (if one was configured)
