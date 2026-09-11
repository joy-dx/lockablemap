package lockablemap

import (
	"context"
	"encoding/json"
	"sync"
)

type recordLock struct {
	token chan struct{}

	stateMu sync.Mutex
	held    bool
}

func newRecordLock() *recordLock {
	lock := &recordLock{
		token: make(chan struct{}, 1),
	}

	lock.token <- struct{}{}

	return lock
}

func (lock *recordLock) lock(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-lock.token:
		lock.stateMu.Lock()
		lock.held = true
		lock.stateMu.Unlock()

		return nil
	}
}

func (lock *recordLock) unlock() {
	lock.stateMu.Lock()
	if !lock.held {
		lock.stateMu.Unlock()
		return
	}

	lock.held = false
	lock.stateMu.Unlock()

	lock.token <- struct{}{}
}

type LockableMap[T comparable, T2 any] struct {
	Map map[T]T2 `json:"map" yaml:"map"`

	mu       sync.RWMutex
	recordMu sync.Mutex
	records  map[T]*recordLock
}

func NewLockableMap[T comparable, T2 any]() *LockableMap[T, T2] {
	return &LockableMap[T, T2]{
		Map:     make(map[T]T2),
		records: make(map[T]*recordLock),
	}
}

func (lm *LockableMap[T, T2]) MarshalJSON() ([]byte, error) {
	lm.mu.RLock()
	defer lm.mu.RUnlock()

	return json.Marshal(lm.Map)
}

func (lm *LockableMap[T, T2]) Set(key T, entry T2) {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	lm.Map[key] = entry
}

func (lm *LockableMap[T, T2]) Remove(key T) {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	delete(lm.Map, key)
}

func (lm *LockableMap[T, T2]) Get(key T) (T2, error) {
	lm.mu.RLock()
	defer lm.mu.RUnlock()

	if value, found := lm.Map[key]; found {
		return value, nil
	}

	var zero T2
	return zero, &KeyNotFoundError{Key: key}
}

func (lm *LockableMap[T, T2]) GetAll() map[T]T2 {
	lm.mu.RLock()
	defer lm.mu.RUnlock()

	result := make(map[T]T2, len(lm.Map))
	for key, value := range lm.Map {
		result[key] = value
	}

	return result
}

func (lm *LockableMap[T, T2]) GetAllSlice() []T2 {
	lm.mu.RLock()
	defer lm.mu.RUnlock()

	result := make([]T2, 0, len(lm.Map))
	for _, value := range lm.Map {
		result = append(result, value)
	}

	return result
}

func (lm *LockableMap[T, T2]) GetKeys() []T {
	lm.mu.RLock()
	defer lm.mu.RUnlock()

	result := make([]T, 0, len(lm.Map))
	for key := range lm.Map {
		result = append(result, key)
	}

	return result
}

func (lm *LockableMap[T, T2]) GetFilteredSlice(
	filter func(T) bool,
) []T2 {
	lm.mu.RLock()
	defer lm.mu.RUnlock()

	result := make([]T2, 0)
	for key, value := range lm.Map {
		if filter(key) {
			result = append(result, value)
		}
	}

	return result
}

func (lm *LockableMap[T, T2]) GetFilteredMap(
	filter func(T) bool,
) map[T]T2 {
	lm.mu.RLock()
	defer lm.mu.RUnlock()

	result := make(map[T]T2)
	for key, value := range lm.Map {
		if filter(key) {
			result[key] = value
		}
	}

	return result
}

func (lm *LockableMap[T, T2]) getRecordLock(key T) *recordLock {
	lm.recordMu.Lock()
	defer lm.recordMu.Unlock()

	if lock, found := lm.records[key]; found {
		return lock
	}

	lock := newRecordLock()
	lm.records[key] = lock

	return lock
}

type RecordLock[T comparable, T2 any] struct {
	lm     *LockableMap[T, T2]
	locks  map[T]*recordLock
	mu     sync.Mutex
	closed bool
}

func (lm *LockableMap[T, T2]) LockRecords(
	ctx context.Context,
	keys ...T,
) (*RecordLock[T, T2], error) {
	guard := &RecordLock[T, T2]{
		lm:    lm,
		locks: make(map[T]*recordLock, len(keys)),
	}

	for _, key := range keys {
		lock := lm.getRecordLock(key)

		if err := lock.lock(ctx); err != nil {
			guard.Unlock()
			return nil, err
		}

		guard.locks[key] = lock
	}

	return guard, nil
}

func (guard *RecordLock[T, T2]) Unlock() {
	guard.mu.Lock()
	defer guard.mu.Unlock()

	if guard.closed {
		return
	}

	guard.closed = true

	for _, lock := range guard.locks {
		lock.unlock()
	}

	clear(guard.locks)
}

// LockRecord blocks until the record lock is available.
func (lm *LockableMap[T, T2]) LockRecord(key T) {
	_ = lm.LockRecordContext(context.Background(), key)
}

// LockRecordContext attempts to lock a record until the context is canceled.
func (lm *LockableMap[T, T2]) LockRecordContext(
	ctx context.Context,
	key T,
) error {
	lock := lm.getRecordLock(key)
	return lock.lock(ctx)
}

func (lm *LockableMap[T, T2]) UnlockRecord(key T) {
	lm.recordMu.Lock()
	lock, found := lm.records[key]
	lm.recordMu.Unlock()

	if !found {
		panic("lockablemap: UnlockRecord called for an unknown key")
	}

	lock.unlock()
}

// UnlockAll unlocks all currently registered record locks.
//
// The caller must own all of these locks. Do not call this concurrently with
// LockRecordContext or UnlockRecord.
func (lm *LockableMap[T, T2]) UnlockAll() {
	lm.recordMu.Lock()
	locks := make([]*recordLock, 0, len(lm.records))

	for _, lock := range lm.records {
		locks = append(locks, lock)
	}

	lm.recordMu.Unlock()

	for _, lock := range locks {
		lock.unlock()
	}
}
