$VERSION=(Get-Content misc/version.txt) -join ' '
$VERSION = $VERSION.replace("`n","").replace("`r","").replace(" ","")

Get-Content go.mod | Select-String -Pattern "github.com/aws/aws-sdk-go" | %{$a=$_.ToString().split(" "); $AWS_SDK = $a[1]}
Get-Content go.mod | Select-String -Pattern "cloud.google.com/go/storage" | %{$a=$_.ToString().split(" "); $GCP_STORAGE_SDK = $a[1]}
Get-Content go.mod | Select-String -Pattern "github.com/Azure/azure-sdk-for-go/sdk/storage/azblob" | %{$a=$_.ToString().split(" "); $AZURE_BLOB_STORAGE_SDK = $a[1]}

$LATEST_COMMIT_ID = (git rev-parse --short HEAD) -join "`n"
$BUILD_START_DATE = [Xml.XmlConvert]::ToString((get-date),[Xml.XmlDateTimeSerializationMode]::Utc)

@"
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
"@ | out-file -filepath misc/version.go -Encoding utf8