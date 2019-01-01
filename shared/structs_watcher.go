package shared

import (
	"context"
	"golang.org/x/time/rate"
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
	Rate            int64  `json:"rate"`
	ObjectStoreName string `json:"object_store_name"`
	ObjectStoreType string `json:"object_store_type"`
	// if set to true then it means that the job has finished (not that it succeeded but that it finished its run)
	// and that the client should not expect any further messages
	Completed bool `json:"-"`
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
	CommChan chan <-WatchMessage
	// when the channel Ctx.Done is closed then tell the consumer that the server is shutting down
	Ctx context.Context `json:"-"`
	// cancel function produced when above context is created. This is needed in order to actually issue the cancel
	Cancel context.CancelFunc `json:"-"`
	// a string giving some details about the consumer (like src ip + src port) to be used for logging messages (debugging mainly)
	Identifier string
	// consumer id is a uuid and is used when deregistering a consumer
	Uuid string
	// ensures that no more than X messages/second are sent for a given file (but if more then X files are uploaded
	// per second then for each one of them at least 1 message will be sent to the clients)
	Limiter *rate.Limiter
	// Name of the object (file) for which the last message was received.
	CurrentPath string
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
	Consumers []*WatchConsumer
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

// appends a new consumer(client) to the slice of clients
func (multiplexer *WatchMultiplexer) AddConsumer (JobType string, JobName string, JobId string,
	CommChan chan <-WatchMessage,  Ctx context.Context, Cancel context.CancelFunc, ClientIdentifier string,
	ClientUuid string){
	NewClient := &WatchConsumer{
		JobType: JobType,
		JobName: JobName,
		JobId: JobId,
		CommChan: CommChan,
		Ctx: Ctx,
		Cancel: Cancel,
		Identifier: ClientIdentifier,
		Uuid: ClientUuid,
		// rate limit to max 5 updates per second for a given file (actually 6 per second in case it reaches 100%
		// upload during interval). Given multiple files in a 1 second interval then this limit will both be breached
		// and we could also get less than 5 updates during the interval for a given file
		Limiter: rate.NewLimiter(5, 1),
		CurrentPath: "",
	}
	multiplexer.Mutex.Lock()
	defer multiplexer.Mutex.Unlock()
	multiplexer.Consumers = append(multiplexer.Consumers, NewClient)
	logger.Debugf("Added entry for watch consumer '%s' having uuid '%s' for %s job '%s' having id '%s'",
		ClientIdentifier, ClientUuid, JobType, JobName, JobId)
}


// removes a consumer(client) from the slice of clients
func (multiplexer *WatchMultiplexer) RemoveConsumer (ClientIdentifier string, ClientUuid string) {
	multiplexer.Mutex.Lock()
	defer multiplexer.Mutex.Unlock()
	for k, entry := range multiplexer.Consumers {
		if entry.Uuid == ClientUuid {
			// deleted match "entry" from multiplexer.Consumers - unfortunately in GO there is no bespoke function so ...
			multiplexer.Consumers[k] = multiplexer.Consumers[len(multiplexer.Consumers)-1]
			multiplexer.Consumers[len(multiplexer.Consumers)-1] = nil // without this Garbage Collection will leak memory
			multiplexer.Consumers = multiplexer.Consumers[:len(multiplexer.Consumers)-1]
			logger.Debugf("Deleted entry for watch consumer '%s' having uuid '%s'. There are '%d' remaining " +
				"consumers", ClientIdentifier, ClientUuid, len(multiplexer.Consumers))
			return
		}
	}
	// if we got here, we got a problem
	logger.Debugf("Did not find an entry to delete for watch consumer '%s' having uuid '%s'. This is most " +
		"likely a bug", ClientIdentifier, ClientUuid,)

}