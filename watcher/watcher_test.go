package watcher

import (
	"cloudbackup/shared"
	"context"
	"github.com/satori/go.uuid"
	"strconv"
	"sync"
	"testing"
	"time"
)

func TestStop1(t *testing.T) {
	serverMsgChan := make(chan shared.WatchMessage, 1000)

	ctxMultiplexer, cancelMultiplexer := context.WithCancel(context.Background())
	multiplexer := &shared.WatchMultiplexer{
		Mutex:          &sync.RWMutex{},
		Ctx:            ctxMultiplexer,
		Cancel:         cancelMultiplexer,
		Running:        false,
		WatchMsgSender: serverMsgChan,
	}

	multiplexer.Stop()
	select {
	case <-multiplexer.Ctx.Done():
		// do nothing, we're good
	default: {
		t.Fatalf("multiplexer.Stop() should have led to multiplexer.Ctx.Done() to return instantly but it " +
			"didn't most likely because the context wasn't cancelled")
	}
	}
}


// send first one message, fetch it back from the client channel and verify it matches expectation. Proceed then
// to send 2 more messages and check the channels size is 2
func TestSendMsgToWatcher1(t *testing.T) {
	serverMsgChan := make(chan shared.WatchMessage, 1000)
	clientMsgChan := make(chan shared.WatchMessage, 1000)

	ctxMultiplexer, cancelMultiplexer := context.WithCancel(context.Background())
	multiplexer := &shared.WatchMultiplexer{
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
	JobName := "first_backup"
	multiplexer.Running = true
	err := multiplexer.AddConsumer("backup", JobName, JobUuid, clientMsgChan, ctx, cancel,
		ClientIdentifier, ClientUuid)
	if err != nil {
		t.Fatalf("multiplexer.AddConsumer() returned error: %s", err)
	}

	go Start(multiplexer)
	msgToSend := shared.WatchMessage{
		JobId: JobUuid,
		JobName: JobName,
		JobType: "backup",
		Path: "/testpath1",
		PercentDone: 0,
		}
	shared.SendMsgToWatcher(msgToSend, serverMsgChan)
	// sleep 10 milliseconds in order to allow the go Start() routine to consume the message and forward it to clients
	time.Sleep(10 * time.Millisecond)
	if len(serverMsgChan) != 0 {
		t.Fatalf("1. It was expected to find 0 messages on the watch multiplexer channel but %d where found",
			len(serverMsgChan))
	}
	if len(clientMsgChan) != 1 {
		t.Fatalf("It was expected to find 1 messages on the watch client channel but %d where found",
			len(clientMsgChan))
	}
	select {
	case receivedMsg := <- clientMsgChan: {
		if receivedMsg.JobId != JobUuid {
			t.Fatalf("Was expecting Jobid=%s in the message received by the client but instead got " +
				"Jobid=%s", JobUuid, receivedMsg.JobId)
		}
	}
	default: {
		t.Fatalf("Did not manage to fetch the message which was supposed to exist on the client channel")
	}
	}

	// send another 2 messages and check client chan size is 2
	msgToSend.Path = "/testpath2"
	shared.SendMsgToWatcher(msgToSend, serverMsgChan)
	msgToSend.Path = "/testpath3"
	shared.SendMsgToWatcher(msgToSend, serverMsgChan)
	// sleep 10 milliseconds in order to allow the go Start() routine to consume the message and forward it to clients
	time.Sleep(10 * time.Millisecond)
	if len(serverMsgChan) != 0 {
		t.Fatalf("2. It was expected to find 0 messages on the watch multiplexer channel but %d where found",
			len(serverMsgChan))
	}
	if len(clientMsgChan) != 2 {
		t.Fatalf("It was expected to find 2 messages on the watch client channel but %d where found",
			len(clientMsgChan))
	}
	select {
	case receivedMsg := <- clientMsgChan: {
		if receivedMsg.JobId != JobUuid {
			t.Fatalf("Was expecting Jobid=%s in the message received by the client but instead got " +
				"Jobid=%s", JobUuid, receivedMsg.JobId)
		}
	}
	default: {
		t.Fatalf("Did not manage to fetch the message which was supposed to exist on the client channel")
	}
	}

	// shutdown multiplexer and check that it did stop properly
	multiplexer.Stop()
	select {
	case <-multiplexer.Ctx.Done():
		// do nothing, we're good
	default: {
		t.Fatalf("multiplexer.Stop() should have led to multiplexer.Ctx.Done() to return instantly but it " +
			"didn't most likely because the context wasn't cancelled")
	}
	}
}

// send to 2 clients, both listening to same job id
func TestSendMsgToWatcher2(t *testing.T) {
	serverMsgChan := make(chan shared.WatchMessage, 1000)
	clientMsgChan1 := make(chan shared.WatchMessage, 1000)
	clientMsgChan2 := make(chan shared.WatchMessage, 1000)

	ctxMultiplexer, cancelMultiplexer := context.WithCancel(context.Background())
	multiplexer := &shared.WatchMultiplexer{
		Mutex:          &sync.RWMutex{},
		Ctx:            ctxMultiplexer,
		Cancel:         cancelMultiplexer,
		Running:        false,
		WatchMsgSender: serverMsgChan,
	}

	ctx, cancel := context.WithCancel(context.Background())
	ClientIdentifier1 := "192.168.0.43:3423"
	ClientUuid1 := uuid.NewV4().String()
	ClientIdentifier2 := "192.168.0.43:3423"
	ClientUuid2 := uuid.NewV4().String()
	JobUuid := uuid.NewV4().String()
	JobName := "first_backup"
	multiplexer.Running = true
	err := multiplexer.AddConsumer("backup", JobName, JobUuid, clientMsgChan1, ctx, cancel,
		ClientIdentifier1, ClientUuid1)
	if err != nil {
		t.Fatalf("1. multiplexer.AddConsumer() returned error: %s", err)
	}

	err = multiplexer.AddConsumer("backup", JobName, JobUuid, clientMsgChan2, ctx, cancel,
		ClientIdentifier2, ClientUuid2)
	if err != nil {
		t.Fatalf("2. multiplexer.AddConsumer() returned error: %s", err)
	}

	go Start(multiplexer)
	msgToSend := shared.WatchMessage{
		JobId:       JobUuid,
		JobName:     JobName,
		JobType:     "backup",
		Path:        "/testpath1",
		PercentDone: 0,
	}
	shared.SendMsgToWatcher(msgToSend, serverMsgChan)
	// sleep 10 milliseconds in order to allow the go Start() routine to consume the message and forward it to clients
	time.Sleep(10 * time.Millisecond)
	if len(serverMsgChan) != 0 {
		t.Fatalf("It was expected to find 0 messages on the watch multiplexer channel but %d where found",
			len(serverMsgChan))
	}
	// client 1
	if len(clientMsgChan1) != 1 {
		t.Fatalf("1. It was expected to find 1 messages on the watch client channel but %d where found",
			len(clientMsgChan1))
	}
	select {
	case receivedMsg := <-clientMsgChan1:
		{
			if receivedMsg.JobId != JobUuid {
				t.Fatalf("1. Was expecting Jobid=%s in the message received by the client but instead got "+
					"Jobid=%s", JobUuid, receivedMsg.JobId)
			}
		}
	default:
		{
			t.Fatalf("1. Did not manage to fetch the message which was supposed to exist on the client channel")
		}
	}

	// client 2
	if len(clientMsgChan2) != 1 {
		t.Fatalf("2. It was expected to find 1 messages on the watch client channel but %d where found",
			len(clientMsgChan2))
	}
	select {
	case receivedMsg := <-clientMsgChan2:
		{
			if receivedMsg.JobId != JobUuid {
				t.Fatalf("2. Was expecting Jobid=%s in the message received by the client but instead got "+
					"Jobid=%s", JobUuid, receivedMsg.JobId)
			}
		}
	default:
		{
			t.Fatalf("2. Did not manage to fetch the message which was supposed to exist on the client channel")
		}
	}
}

// send 50 messages in a row and then check
func TestSendMsgToClients1(t *testing.T) {
	serverMsgChan := make(chan shared.WatchMessage, 1000)
	clientMsgChan := make(chan shared.WatchMessage, 1000)

	ctxMultiplexer, cancelMultiplexer := context.WithCancel(context.Background())
	multiplexer := &shared.WatchMultiplexer{
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
	JobName := "first_backup"
	multiplexer.Running = true
	err := multiplexer.AddConsumer("backup", JobName, JobUuid, clientMsgChan, ctx, cancel,
		ClientIdentifier, ClientUuid)
	if err != nil {
		t.Fatalf("multiplexer.AddConsumer() returned error: %s", err)
	}

	msgToSend := shared.WatchMessage{
		JobId: JobUuid,
		JobName: JobName,
		JobType: "backup",
		Path: "/testpath1",
		PercentDone: 0,
	}
	for i := 1; i <= 10; i++ {
		sendMsgToClients(multiplexer, msgToSend)
	}
	if len(clientMsgChan) != shared.WatchClientRateLimitBurst {
		t.Fatalf("It was expected to find %d messages on the watch client channel but %d where found",
			shared.WatchClientRateLimitBurst, len(clientMsgChan))
	}
}

// send 2 * $ChanSize messages in a row and then check that we don't have more the channel size
func TestSendMsgToClients2(t *testing.T) {
	ChanSize := 1000
	serverMsgChan := make(chan shared.WatchMessage, ChanSize)
	clientMsgChan := make(chan shared.WatchMessage, ChanSize)

	ctxMultiplexer, cancelMultiplexer := context.WithCancel(context.Background())
	multiplexer := &shared.WatchMultiplexer{
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
	JobName := "first_backup"
	multiplexer.Running = true
	err := multiplexer.AddConsumer("backup", JobName, JobUuid, clientMsgChan, ctx, cancel,
		ClientIdentifier, ClientUuid)
	if err != nil {
		t.Fatalf("multiplexer.AddConsumer() returned error: %s", err)
	}

	msgToSend := shared.WatchMessage{
		JobId: JobUuid,
		JobName: JobName,
		JobType: "backup",
		PercentDone: 0,
	}
	for i := 1; i <= 2 * ChanSize; i++ {
		msgToSend.Path = "/testpath" + strconv.Itoa(i)
		sendMsgToClients(multiplexer, msgToSend)
	}
	if len(clientMsgChan) != ChanSize {
		t.Fatalf("It was expected to find %d messages on the watch client channel but %d where found",
			2 * ChanSize, len(clientMsgChan))
	}
}


// tell client job has been finished
func TestTellClientsJobFinished1(t *testing.T) {
	serverMsgChan := make(chan shared.WatchMessage, 1000)
	clientMsgChan := make(chan shared.WatchMessage, 1000)

	ctxMultiplexer, cancelMultiplexer := context.WithCancel(context.Background())
	multiplexer := &shared.WatchMultiplexer{
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
	JobName := "first_backup"
	multiplexer.Running = true
	err := multiplexer.AddConsumer("backup", JobName, JobUuid, clientMsgChan, ctx, cancel,
		ClientIdentifier, ClientUuid)
	if err != nil {
		t.Fatalf("multiplexer.AddConsumer() returned error: %s", err)
	}

	go Start(multiplexer)
	TellClientsJobFinished("backup", JobName,JobUuid, serverMsgChan, false, false)

	// sleep 10 milliseconds in order to allow the go Start() routine to consume the message and forward it to clients
	time.Sleep(10 * time.Millisecond)

	if len(clientMsgChan) != 1 {
		t.Fatalf("It was expected to find 1 messages on the watch client channel but %d where found",
			len(clientMsgChan))
	}
	select {
	case receivedMsg := <-clientMsgChan:
		{
			if receivedMsg.JobId != JobUuid {
				t.Fatalf("Was expecting Jobid=%s in the message received by the client but instead got "+
					"Jobid=%s", JobUuid, receivedMsg.JobId)
			}
			if receivedMsg.JobCompleted != true {
				t.Fatalf("Was expecting a JobComplete marker but didn't find it")
			}
		}
	default:
		{
			t.Fatalf("Did not manage to fetch the message which was supposed to exist on the client channel")
		}
	}
}

// tell client job has been cancelled
func TestTellClientsJobFinished2(t *testing.T) {
	serverMsgChan := make(chan shared.WatchMessage, 1000)
	clientMsgChan := make(chan shared.WatchMessage, 1000)

	ctxMultiplexer, cancelMultiplexer := context.WithCancel(context.Background())
	multiplexer := &shared.WatchMultiplexer{
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
	JobName := "first_backup"
	multiplexer.Running = true
	err := multiplexer.AddConsumer("backup", JobName, JobUuid, clientMsgChan, ctx, cancel,
		ClientIdentifier, ClientUuid)
	if err != nil {
		t.Fatalf("multiplexer.AddConsumer() returned error: %s", err)
	}

	go Start(multiplexer)
	TellClientsJobFinished("backup", JobName,JobUuid, serverMsgChan, true, false)

	// sleep 10 milliseconds in order to allow the go Start() routine to consume the message and forward it to clients
	time.Sleep(10 * time.Millisecond)

	if len(clientMsgChan) != 1 {
		t.Fatalf("It was expected to find 1 messages on the watch client channel but %d where found",
			len(clientMsgChan))
	}
	select {
	case receivedMsg := <-clientMsgChan:
		{
			if receivedMsg.JobId != JobUuid {
				t.Fatalf("Was expecting Jobid=%s in the message received by the client but instead got "+
					"Jobid=%s", JobUuid, receivedMsg.JobId)
			}
			if receivedMsg.JobCompleted != false {
				t.Fatalf("Was expecting a JobComplete=false")
			}
			if receivedMsg.JobAborted != true {
				t.Fatalf("Was expecting a JobAborted=true")
			}
		}
	default:
		{
			t.Fatalf("Did not manage to fetch the message which was supposed to exist on the client channel")
		}
	}
}

// tell client job has failed
func TestTellClientsJobFinished3(t *testing.T) {
	serverMsgChan := make(chan shared.WatchMessage, 1000)
	clientMsgChan := make(chan shared.WatchMessage, 1000)

	ctxMultiplexer, cancelMultiplexer := context.WithCancel(context.Background())
	multiplexer := &shared.WatchMultiplexer{
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
	JobName := "first_backup"
	multiplexer.Running = true
	err := multiplexer.AddConsumer("backup", JobName, JobUuid, clientMsgChan, ctx, cancel,
		ClientIdentifier, ClientUuid)
	if err != nil {
		t.Fatalf("multiplexer.AddConsumer() returned error: %s", err)
	}

	go Start(multiplexer)
	TellClientsJobFinished("backup", JobName,JobUuid, serverMsgChan, false, true)

	// sleep 10 milliseconds in order to allow the go Start() routine to consume the message and forward it to clients
	time.Sleep(10 * time.Millisecond)

	if len(clientMsgChan) != 1 {
		t.Fatalf("It was expected to find 1 messages on the watch client channel but %d where found",
			len(clientMsgChan))
	}
	select {
	case receivedMsg := <-clientMsgChan:
		{
			if receivedMsg.JobId != JobUuid {
				t.Fatalf("Was expecting Jobid=%s in the message received by the client but instead got "+
					"Jobid=%s", JobUuid, receivedMsg.JobId)
			}
			if receivedMsg.JobCompleted != false {
				t.Fatalf("Was expecting a JobComplete=false")
			}
			if receivedMsg.JobFailed != true {
				t.Fatalf("Was expecting a JobFailed=true")
			}
		}
	default:
		{
			t.Fatalf("Did not manage to fetch the message which was supposed to exist on the client channel")
		}
	}
}