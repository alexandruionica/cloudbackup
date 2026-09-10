#!/usr/bin/env bash
set -e
set -o pipefail

VERSION=$(cat misc/version.txt)

if [ -z "$VERSION" ]
then
   echo "misc/version.txt is empty" &>2
   exit 1
fi

AWS_SDK=$(grep 'github.com/aws/aws-sdk-go-v2 ' go.mod  | awk {'print $2'})
GCP_STORAGE_SDK=$(grep cloud.google.com/go/storage go.mod  | awk {'print $2'})
AZURE_BLOB_STORAGE_SDK=$(grep github.com/Azure/azure-sdk-for-go/sdk/storage/azblob go.mod  | awk {'print $2'})

LATEST_COMMIT_ID=$(git rev-parse --short HEAD)
if [[ $(date -u +"%6N") =~ ^[0-9]{6}$ ]]; then
  BUILD_START_DATE=$(date -u +"%Y-%m-%dT%H:%M:%S.%6NZ")
else
  # BSD date (FreeBSD, macOS) doesn't support sub second and would print "%6N" as a literal "6N"
  BUILD_START_DATE=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
fi

cat << EOF > misc/version.go
package misc

func CloudBackupVersion() Version {
	return Version{
		AwsSdk: "$AWS_SDK",
		GcpStorageSdk: "$GCP_STORAGE_SDK",
		AzureBlobStorageSdk: "$AZURE_BLOB_STORAGE_SDK",
		CloudBackup: "$VERSION-$LATEST_COMMIT_ID",
		BuildDate: "$BUILD_START_DATE",
	}
}
EOF