package misc

import (
	"testing"
	//"github.com/stretchr/testify/assert"
	"github.com/sirupsen/logrus/hooks/test"
)

func TestSetupLogging(t *testing.T) {
	hook := test.NewGlobal()
	args := LoggingArgs{}

	SetupLogging(args)

	if len(hook.Entries) != 0 {
		t.Fatalf("expected 0 log lines but received %+v", hook.Entries)
	}
	// cleanup
	hook.Reset()
}

func TestSetupLoggingTextmode(t *testing.T) {
	hook := test.NewGlobal()
	args := LoggingArgs{TextLog: true}

	SetupLogging(args)

	if len(hook.Entries) != 0 {
		t.Fatalf("expected 0 log lines but received %+v", hook.Entries)
	}
	// cleanup
	hook.Reset()
}

func TestSetupLoggingQuiet(t *testing.T) {
	hook := test.NewGlobal()
	args := LoggingArgs{Quiet: true}

	SetupLogging(args)

	if len(hook.Entries) != 0 {
		t.Fatalf("expected 0 log lines but received %+v", hook.Entries)
	}
	// cleanup
	hook.Reset()
}

func TestSetupLoggingDebug(t *testing.T) {
	hook := test.NewGlobal()
	args := LoggingArgs{Debug: true}

	SetupLogging(args)

	if len(hook.Entries) != 1 {
		t.Fatalf("expected 1 log lines but received %+v", hook.Entries)
	}

	msg := "Debug level messages enabled"
	if hook.LastEntry().Message != msg {
		t.Fatalf("expected log entry '%s' but got '%+v'", msg, hook.LastEntry().Message)
	}
	// cleanup
	hook.Reset()
}
