package watcher

import (
	"cloudbackup/shared"
	"context"
	log "github.com/sirupsen/logrus"
)

const loggingContext = "watcher"

var logger = log.WithFields(log.Fields{
	"context": loggingContext,
})

// this should be started as go routine by the caller
func Start(multiplexer *shared.WatchMultiplexer) {
	logger.Debug("Watcher (message multiplexer) is starting")
	multiplexer.Mutex.Lock()
	multiplexer.Running = true
	multiplexer.Mutex.Unlock()
	for {
		select {
		case <-multiplexer.Ctx.Done():
			{
				logger.Debug("Watcher (message multiplexer) is shutting down")
				// ensure new clients are no longer added
				multiplexer.Mutex.Lock()
				multiplexer.Running = false
				multiplexer.Mutex.Unlock()
				//
				tellClientsToExit(multiplexer)
				return
			}
		case receivedMsg := <-multiplexer.WatchMsgSender:
			{
				sendMsgToClients(multiplexer, receivedMsg)
				continue
			}
		}
	}
}

// this actually sends a message which will be received by the HTTP handler and it will be the one to close the
// connection held by the client(s)
func tellClientsToExit(multiplexer *shared.WatchMultiplexer) {
	multiplexer.Mutex.RLock()
	defer multiplexer.Mutex.RUnlock()
	for _, client := range multiplexer.Consumers {
		logger.Debugf("Notifying consumer %s to exit due to the Watcher exiting itself", client.Identifier)
		client.Cancel()
	}
}

// Tells watch clients that a particular job has finished so they should cleanup and exit
// $JobType must be one of "backup" or "restore". If cancelled == true then it means the job was cancelled while
// running (and before it completed)
func TellClientsJobFinished(JobType string, JobName string, JobId string, WatchMsgReceiver chan<- shared.WatchMessage, JobCancelled bool, JobFailed bool) {
	msg := shared.WatchMessage{
		Sequence:        0,
		JobType:         JobType,
		JobName:         JobName,
		JobId:           JobId,
		Path:            "",
		PercentDone:     100,
		Rate:            0,
		ObjectType:      "unknown",
		ObjectStoreName: "",
		ObjectStoreType: "",
		OperationType:   "",
		Error:           "",
		JobAborted:      JobCancelled,
		JobFailed:       JobFailed,
	}
	// if JobFailed then it means the backup job failed to start
	if !JobFailed && !JobCancelled {
		msg.JobCompleted = true
	}
	shared.SendMsgToWatcher(msg, WatchMsgReceiver)
}

// send message to each http handler which in turn will send then to their connect client
func sendMsgToClients(multiplexer *shared.WatchMultiplexer, msg shared.WatchMessage) {
	// avoid logging in this function as this is a performance sensitive step and slowdowns would lead to the
	// multiplexer being slower at forwarding messages
	multiplexer.Mutex.RLock()
	defer multiplexer.Mutex.RUnlock()
	for k, client := range multiplexer.Consumers {
		if msg.JobId == client.JobId && msg.JobName == client.JobName && msg.JobType == client.JobType {
			// check if rate limiting and other factors allows us to send
			// Given that this function makes changes on $client it is MANDATORY THAT THE CALLER DOES LOCKING on the parent []*shared.WatchConsumer
			if clientSendAllowed(multiplexer.Ctx, multiplexer.Consumers[k], msg) {
				select {
				// the receiver is a buffered channel
				case client.CommChan <- msg:
					continue
				// if client channel is full then try to remove the first message available for consumption and append
				// the current event which could not be sent. Basically act like a ring buffer.
				default:
					select {
					case <-client.CommChan:
						select {
						// try to send again the message to the client
						case client.CommChan <- msg:
							continue
						// discard message if channel is still full
						default:
							continue
						}
					// if a message can't be fetched from the client channel then abort and discard the new message
					default:
						continue
					}
				}
			} else {
				continue
			}

		}
	}
}

// checks if it's allowed to send messages to a given client. Also updates the path and sequence numbers whenever needed
// Given that this function makes changes on $client it is MANDATORY THAT THE CALLER DOES LOCKING on the parent []*shared.WatchConsumer
func clientSendAllowed(ctx context.Context, client *shared.WatchConsumer, msg shared.WatchMessage) bool {
	sendAllowed := false
	// if this is the first message sent to this client then send it or if we got an error then forward it to the client and ignore rate limiting
	if client.CurrentPath == "" || msg.Error != "" {
		sendAllowed = true
		// init path
		if client.CurrentPath == "" {
			client.CurrentPath = msg.Path
		}
	} else {
		// as previous messages have been sent for this object then decide if rate limiting allows us to send a message
		if client.CurrentPath == msg.Path {
			// check if 1 token is available
			if client.Limiter.Allow() {
				// we have 1 token available so let's consume it (should NOT cause any pause)
				err := client.Limiter.Wait(ctx)
				if err != nil {
					// if the context gets cancelled then this would also lead to Limiter.Wait() returning an error but
					// we still want to send in this case as the last message we send will signal to the client to exit too
					if err == context.Canceled {
						sendAllowed = true
					} else {
						logger.Debugf("While running Limiter.Wait() before sending a message to connected "+
							"client '%s' got error: %s", client.Identifier, err)
					}
				} else {
					sendAllowed = true
				}
			} else {
				// even if rate limiting would prevent us to send, ignore and send if this is the last message for this Object(file)
				if msg.PercentDone == 100 {
					sendAllowed = true
				}
			}
		} else {
			// this object had no message sent so far so we want to send at least 1 for it
			sendAllowed = true
			// update CurrentPath
			client.CurrentPath = msg.Path
		}
	}
	return sendAllowed
}
