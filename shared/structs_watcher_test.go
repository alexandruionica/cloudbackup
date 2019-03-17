package shared

import (
	"context"
	"github.com/satori/go.uuid"
	"sync"
	"testing"
)

// try to add consumer to a multiplexer which is either not yet running or shutting down
func TestAddConsumer1(t *testing.T) {
	serverMsgChan := make(chan WatchMessage, 1000)
	clientMsgChan := make(chan WatchMessage, 1000)

	ctxMultiplexer, cancelMultiplexer := context.WithCancel(context.Background())
	multiplexer := &WatchMultiplexer{
		Mutex:          &sync.RWMutex{},
		Ctx:            ctxMultiplexer,
		Cancel:         cancelMultiplexer,
		Running:        false,
		WatchMsgSender: serverMsgChan,
	}

	ctx, cancel := context.WithCancel(context.Background())
	ClientIdentifier := "192.168.0.43:3423"
	ClientUuid := uuid.NewV4().String()
	JobUuid := uuid.NewV4().String()
	err := multiplexer.AddConsumer("backup", "first_backup", JobUuid, clientMsgChan, ctx, cancel,
		ClientIdentifier, ClientUuid)
	if err != nil {
		if err.Error() != MultiplexerNotReady {
			t.Fatalf("multiplexer.AddConsumer() was expected to return error: %s   but it returned error:"+
				" %s", MultiplexerNotReady, err)
		}
	} else {
		t.Fatalf("multiplexer.AddConsumer() was expected to return an error but it didn't")
	}

	multiplexer.Running = true
	err = multiplexer.AddConsumer("backup", "first_backup", JobUuid, clientMsgChan, ctx, cancel,
		ClientIdentifier, ClientUuid)
	if err != nil {
		t.Fatalf("multiplexer.AddConsumer() returned error: %s", err)
	}
}

// add consumer to running multiplexer and then remove it
func TestAddConsumerAndRemoveConsumer(t *testing.T) {
	serverMsgChan := make(chan WatchMessage, 1000)
	clientMsgChan := make(chan WatchMessage, 1000)

	ctxMultiplexer, cancelMultiplexer := context.WithCancel(context.Background())
	multiplexer := &WatchMultiplexer{
		Mutex:          &sync.RWMutex{},
		Ctx:            ctxMultiplexer,
		Cancel:         cancelMultiplexer,
		Running:        false,
		WatchMsgSender: serverMsgChan,
	}

	ctx, cancel := context.WithCancel(context.Background())
	ClientIdentifier := "192.168.0.43:3423"
	ClientUuid := uuid.NewV4().String()
	JobUuid := uuid.NewV4().String()
	multiplexer.Running = true
	err := multiplexer.AddConsumer("backup", "first_backup", JobUuid, clientMsgChan, ctx, cancel,
		ClientIdentifier, ClientUuid)
	if err != nil {
		t.Fatalf("multiplexer.AddConsumer() returned error: %s", err)
	}

	if len(multiplexer.Consumers) != 1 {
		t.Fatalf("1. Was expecting to have 1 watch consumer but insted found: %d", len(multiplexer.Consumers))
	}

	if multiplexer.Consumers[0].Uuid != ClientUuid {
		t.Fatalf("Was expecting the only watch consumer to have uuid %s but found %s", ClientUuid,
			multiplexer.Consumers[0].Uuid)
	}

	multiplexer.RemoveConsumer("abcd", "invaliduuid")
	// function doesn't return an error so we need to be sneaky
	if len(multiplexer.Consumers) != 1 {
		t.Fatalf("2. Was expecting to have 1 watch consumer but insted found: %d", len(multiplexer.Consumers))
	}

	multiplexer.RemoveConsumer(ClientIdentifier, ClientUuid)
	// function doesn't return an error so we need to be sneaky
	if len(multiplexer.Consumers) != 0 {
		t.Fatalf("Was expecting to have 0 watch consumers but insted found: %d", len(multiplexer.Consumers))
	}
}
