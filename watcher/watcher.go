package watcher

import (
	"cloudbackup/shared"
	"context"
	"sync"

	log "github.com/sirupsen/logrus"
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
	for _, client := range multiplexer.Consumers {
		if msg.JobId == client.JobId && msg.JobName == client.JobName && msg.JobType == client.JobType {
			select {
			// the receiver is a buffered channel
			case client.CommChan <- msg:
				continue
			default:
				// do nothing if channel is full and continue to next iteration
				logger.Debugf("Watcher (message multiplexer) can't send message on channel belonging to client" +
					" '%s' as the channel is full. Discarding the message for this client as the client may be too " +
					"slow to keep up with the server", client.Identifier)
				continue
			}
		}
	}
}
