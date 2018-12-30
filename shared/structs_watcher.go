package shared

import (
	"context"
	"sync"

	log "github.com/sirupsen/logrus"
)

var logger = log.WithFields(log.Fields{
	"context": loggingContext,
})

// this type will be sent to clients watching in real time a particular backup or restore job
type WatchMessage struct {
	// incremented for each item being backed up. This is used in order to determine if messages were dropped due to
	// the communication channel between the http server and the client being full
	Sequence uint64 `json:"sequence"`
	// One of: restore of backup
	JobType string `json:"-"`
	// backup job name as defined in the server's config file
	JobName string `json:"-"`
	// uuid of job
	JobId string `json:"-"`
	// object being backed up or restored
	Path string `json:"path"`
	// for a given object, shows progress
	PercentDone uint `json:"percent_done"`
	// one minute rate in bytes per second for given $Path
	Rate            int64  `json:"rate_1min"`
	ObjectStoreName string `json:"object_store_name"`
	ObjectStoreType string `json:"object_store_type"`
	// if set to true then it means that the job has finished (not that it succeeded but that it finished its run)
	// and that the client should not expect any further messages
	Completed bool
}


// each consumer will have a struct like below and the Multiplexer routine will walk a slice of structs and send
// received messages to each slice entry matching the job type, name and uuid
type WatchConsumer struct {
	// One of: restore of backup
	JobType string
	// backup job name as defined in the server's config file
	JobName string
	// uuid of the ob
	JobId string
	// the multiplexer sends messages for the consumption of the client
	CommChan chan WatchMessage
	// when the channel Ctx.Done is closed then tell the consumer that the server is shutting down
	Ctx context.Context `json:"-"`
	// cancel function produced when above context is created. This is needed in order to actually issue the cancel
	Cancel context.CancelFunc `json:"-"`
	// a string giving some details about the consumer (like src ip + src port) to be used for logging messages (debugging mainly)
	Identifier string
}

// the Multiplexer routine will be methods attached to this struct
type WatchMultiplexer struct {
	// lock this before reading or making any changes to the struct
	Mutex *sync.RWMutex
	// when the channel Ctx.Done is close then tell the multiplexer to signal all consumers they need to exit and then
	// proceed to exit itself
	Ctx context.Context `json:"-"`
	// cancel function produced when above context is created. This is needed in order to actually issue the cancel
	Cancel context.CancelFunc `json:"-"`
	// the Multiplexer sets this to "true" once it's ready to receive messages and should set it to "false" when it
	// prepares to exit. Http handlers should check this == true before attempting to register a new client
	Running bool
	// For each registered consumer there should be an entry in this slice
	Consumers []WatchConsumer
	// on this channel messages to be sent to clients are received (from backup or restore jobs)
	WatchMsgSender <- chan WatchMessage
}

// sends message to shutdown the multiplexer  by cancelling the context
func (multiplexer *WatchMultiplexer) Stop (){
	multiplexer.Cancel()
}


func SendMsgToWatcher(msg WatchMessage, WatchMsgReceiver chan <-WatchMessage) {
	select {
	case WatchMsgReceiver <- msg:
		return
	default:
		logger.Debug("Watcher's receive channel is full, discarding message")
	}
}