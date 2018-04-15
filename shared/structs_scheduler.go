package shared

import "sync"

type CommWithSchedulerForBackup struct {
	// this needs to be locked before acquiring the channel to send messages to the scheduler goroutine or read messages
	// sent by the scheduler goroutine
	Mutex *sync.Mutex
	// on this chanel the scheduler receives commands
	ReceivedCommand chan ReceiveBackupCommand
	// on this chanel the scheduler sends the response to commands it received
	SendResponse chan ResponseBackupCommand
}

type ReceiveBackupCommand struct {
	// uuid of the command
	Id string
	// one of "start" or "stop"
	Command string
	// uuid of the backup job referenced. Makes sense only for "stop" command
	BackupJobId string
	// name of the backup job as it is defined in the configuration file
	Name string
}

type ResponseBackupCommand struct {
	// uuid of the command
	Id string
	// what command was requested one of "start" or "stop"
	Command string
	// uuid of the backup job referenced. This will be an existing uuid for responses to "stop" commands and a new
	// uuid when this is a response of a successful "start" command.
	BackupJobId string
	// name of the backup job as it is defined in the configuration file
	Name string
	// true if the command did not succeed
	Err bool
	// message to send back to the user. Will matter only when err == true
	Message string
}

// init the CommWithSchedulerForBackup structure
func (comm *CommWithSchedulerForBackup) Init () {
	comm.Mutex = &sync.Mutex{}
	comm.ReceivedCommand = make(chan ReceiveBackupCommand)
	comm.SendResponse = make(chan ResponseBackupCommand)
}