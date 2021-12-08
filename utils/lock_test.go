package utils

import (
	"fmt"
	"sync/atomic"
	"testing"
	"time"
)

func TestMutexWithTimeout(t *testing.T) {
	lock := NewMutexWithTimeout()
	if !lock.GetLockWithTimeout(1 * time.Second) {
		t.Fatal("1. Didn't get lock despite expecting to do so")
	}

	if lock.GetLockWithTimeout(1 * time.Second) {
		t.Fatal("got lock but should have not")
	}

	// call multiple times lock.ReleaseLock() ; should not cause any issue
	lock.ReleaseLock()
	lock.ReleaseLock()
	lock.ReleaseLock()

	if !lock.GetLockWithTimeout(1 * time.Second) {
		t.Fatal("2. Didn't get lock despite expecting to do so")
	}

	if lock.GetLockWithTimeout(1 * time.Second) {
		t.Fatal("got lock but should have not")
	}

	lock.ReleaseLock()

	// should not block
	lock.GetLock()

	var testInt uint32
	// use a channel to hand off the error
	errs := make(chan error, 1)

	go func() { // nolint
		t.Log("Routine attempting to get a lock, in blocking mode")
		atomic.AddUint32(&testInt, 1)
		lock.GetLock()
		errs <- fmt.Errorf("should have never gotten lock as it should be held by previous call to GetLock()")
	}()

	// give a change for the above go func to complete run
	time.Sleep(time.Millisecond * 50)
	if atomic.LoadUint32(&testInt) != 1 {
		t.Fatalf("GO routine did not ran")
	}
	select {
	case msg := <-errs:
		t.Fatalf(msg.Error())
	default:
		return
	}
}
