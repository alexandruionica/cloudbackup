package watcher

import (
	"cloudbackup/shared"
	"context"
	log "github.com/sirupsen/logrus"
	"sync"
)


const loggingContext = "watcher"
var logger = log.WithFields(log.Fields{
	"context": loggingContext,
})

// return an initialised object
func New (msgReceiver <- chan shared.WatchMessage) *shared.WatchMultiplexer {
	ctx, cancel := context.WithCancel(context.Background())
	return &shared.WatchMultiplexer{
		Mutex: &sync.RWMutex{},
		Ctx: ctx,
		Cancel: cancel,
		Running: false,
		WatchMsgSender: msgReceiver,
	}
}

// this should be started as go routine by the caller
func Start(multiplexer *shared.WatchMultiplexer) {
	logger.Debug("Watcher (message multiplexer) is starting")
	for {
		select {
		case <-multiplexer.Ctx.Done():
			{
				logger.Debug("Watcher (message multiplexer) is shutting down")
				tellClientsToExit(multiplexer)
				return
			}
		case receivedMsg := <-multiplexer.WatchMsgSender: {
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

// send message to each http handler which in turn will send then to their connect client
func sendMsgToClients(multiplexer *shared.WatchMultiplexer, msg shared.WatchMessage) {
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
				default:
					// do nothing if channel is full and continue to next iteration
					// avoid logging here as this is a performance sensitive step and slowdowns would lead to the
					// multiplexer being slower at forwarding messages
					continue
				}
			} else {
				continue
			}

		}
	}
}

// checks if it's allowed to send messages to a given client. Also updates the path and sequence numbers whenever needed
// Given that this function makes changes on $client it is MANDATORY THAT THE CALLER DOES LOCKING on the parent []*shared.WatchConsumer
func clientSendAllowed (ctx context.Context, client *shared.WatchConsumer, msg shared.WatchMessage) bool {
	sendAllowed := false
	// if this is the first message sent to this client then send it
	if client.CurrentPath == "" {
		sendAllowed = true
		// init path
		client.CurrentPath = msg.Path
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
						logger.Debugf("While running Limiter.Wait() before sending a message to connected " +
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


// TODO - write function to tell consumers backup run is completed