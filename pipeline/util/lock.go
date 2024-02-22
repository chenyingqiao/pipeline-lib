package util

import (
	"sync"
)

var lockerOnce sync.Once
var locker *Locker

type Locker struct {
	lockerPool    map[string]*sync.Mutex
	lockerUnblock map[string]bool
	lockForLock   *sync.Mutex
	lockForUnlock *sync.Mutex
}

func NewLocker() *Locker {
	lockerOnce.Do(func() {
		locker = &Locker{
			lockerPool:    map[string]*sync.Mutex{},
			lockerUnblock: map[string]bool{},
			lockForLock:   &sync.Mutex{},
			lockForUnlock: &sync.Mutex{},
		}
	})
	return locker
}

func (lk *Locker) LockUnblock(id string) bool {
	lk.lockForLock.Lock()
	defer lk.lockForLock.Unlock()
	if _, ok := lk.lockerUnblock[id]; !ok {
		lk.lockerUnblock[id] = true
		return true
	}
	return false
}
func (lk *Locker) UnlockUnblock(id string) {
	lk.lockForUnlock.Lock()
	defer lk.lockForUnlock.Unlock()
	if _, ok := lk.lockerUnblock[id]; ok {
		delete(lk.lockerUnblock, id)
	}
}

func (lk *Locker) Lock(id string) {
	lk.lockForLock.Lock()
	if _, ok := lk.lockerPool[id]; !ok {
		lk.lockerPool[id] = &sync.Mutex{}
	}
	lk.lockForLock.Unlock()
	lk.lockerPool[id].Lock()
}

func (lk *Locker) Unlock(id string) {
	lk.lockForUnlock.Lock()
	defer lk.lockForUnlock.Unlock()
	if _, ok := lk.lockerPool[id]; !ok {
		return
	}
	lk.lockerPool[id].Unlock()
	delete(lk.lockerUnblock, id)
}

