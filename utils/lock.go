package utils

import "time"

type MutexWithTimeout struct {
	lock chan struct{}
}

// this is not an optimal implementation as if you look at the source of Golang's sync/Mutex you will see there are
// also issues of starvation which are dealt with in the sync/Mutex package. None the less, this implementation is good
// enough if you don't care that a client may wait for a lock more than clients which attempt to get it after the first
// function attempted so in practical terms they may get the lock granted before the other client despite the other
// having requested before them (but when it requested the lock was already granted so since then it is blocking)
func NewMutexWithTimeout() *MutexWithTimeout {
	result := &MutexWithTimeout{
		lock: make(chan struct{}, 1),
	}
	// add an element to the chan so it's "unlocked"
	result.lock <- struct{}{}
	return result
}

// try to get a lock, if timeout is reached then abandon; return true on success, false otherwise
// Timeout after $timeout duration and return false in this case
func (mu *MutexWithTimeout) GetLockWithTimeout(timeout time.Duration) bool {
	select {
	case <-mu.lock:
		// Lock has been acquired
		return true
	case <-time.After(timeout):
		// Lock was not acquired
		return false
	}
}

// wait until lock is granted
func (mu *MutexWithTimeout) GetLock() {
	<-mu.lock
	return
}

// Release the lock; can be safely called multiple times.
func (mu *MutexWithTimeout) ReleaseLock() {
	select {
	case mu.lock <- struct{}{}:
		{
			return
		}
	default:
		{
			// if ReleaseLock is called on an already full channel (which means it's already released) then return
			return
		}
	}
}
