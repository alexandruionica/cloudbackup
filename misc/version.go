package misc

func CloudBackupVersion() Version {
	// the values below are supposed to be set before the build starts, by the script generate_version.sh or generate_version.ps1
	return Version{
		AwsSdk:              "",
		GcpStorageSdk:       "",
		AzureBlobStorageSdk: "",
		CloudBackup:         "",
		BuildDate:           "",
	}
}
