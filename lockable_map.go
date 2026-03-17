package lockablemap

import (
	"encoding/json"
	"sync"
)

type LockableMap[T comparable, T2 any] struct {
	Map map[T]T2 `json:"map" yaml:"map"`
	mu  sync.RWMutex
}

func NewLockableMap[T comparable, T2 any]() *LockableMap[T, T2] {
	return &LockableMap[T, T2]{
		Map: make(map[T]T2),
		mu:  sync.RWMutex{},
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
	if val, found := lm.Map[key]; found {
		return val, nil
	}
	var t T2
	return t, &KeyNotFoundError{Key: key}
}

func (lm *LockableMap[T, T2]) GetAll() map[T]T2 {
	lm.mu.RLock()
	defer lm.mu.RUnlock()
	cp := make(map[T]T2, len(lm.Map))
	for k, val := range lm.Map {
		cp[k] = val
	}
	return cp
}
func (lm *LockableMap[T, T2]) GetAllSlice() []T2 {
	lm.mu.RLock()
	defer lm.mu.RUnlock()
	entries := make([]T2, 0, len(lm.Map))
	for _, value := range lm.Map {
		entries = append(entries, value)
	}
	return entries
}

func (lm *LockableMap[T, T2]) GetKeys() []T {
	lm.mu.RLock()
	defer lm.mu.RUnlock()

	keys := make([]T, 0, len(lm.Map))
	for k := range lm.Map {
		keys = append(keys, k)
	}
	return keys
}

func (lm *LockableMap[T, T2]) GetFilteredSlice(
	filter func(T) bool,
) []T2 {
	lm.mu.RLock()
	defer lm.mu.RUnlock()

	result := make([]T2, 0)
	for k, v := range lm.Map {
		if filter(k) {
			result = append(result, v)
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
	for k, v := range lm.Map {
		if filter(k) {
			result[k] = v
		}
	}
	return result
}
