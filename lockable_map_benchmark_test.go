package lockablemap

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"sync"
	"testing"
	"time"
)

type testEntry struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

func TestLockableMapGoldenTable(t *testing.T) {

	tests := []struct {
		name string
		run  func(t *testing.T, lm *LockableMap[string, testEntry])
		want goldenMap[string, testEntry]
	}{
		{
			name: "new map is empty",
			run:  func(t *testing.T, lm *LockableMap[string, testEntry]) {},
			want: goldenMap[string, testEntry]{
				Map: map[string]testEntry{},
			},
		},
		{
			name: "set inserts and replaces values",
			run: func(t *testing.T, lm *LockableMap[string, testEntry]) {
				lm.Set("one", testEntry{Name: "first", Count: 1})
				lm.Set("one", testEntry{Name: "updated", Count: 2})
				lm.Set("two", testEntry{Name: "second", Count: 3})
			},
			want: goldenMap[string, testEntry]{
				Map: map[string]testEntry{
					"one": {Name: "updated", Count: 2},
					"two": {Name: "second", Count: 3},
				},
			},
		},
		{
			name: "remove deletes only the requested key",
			run: func(t *testing.T, lm *LockableMap[string, testEntry]) {
				lm.Set("one", testEntry{Name: "first", Count: 1})
				lm.Set("two", testEntry{Name: "second", Count: 2})
				lm.Remove("one")
			},
			want: goldenMap[string, testEntry]{
				Map: map[string]testEntry{
					"two": {Name: "second", Count: 2},
				},
			},
		},
		{
			name: "remove missing key is a no-op",
			run: func(t *testing.T, lm *LockableMap[string, testEntry]) {
				lm.Set("one", testEntry{Name: "first", Count: 1})
				lm.Remove("missing")
			},
			want: goldenMap[string, testEntry]{
				Map: map[string]testEntry{
					"one": {Name: "first", Count: 1},
				},
			},
		},
		{
			name: "get all returns a copy",
			run: func(t *testing.T, lm *LockableMap[string, testEntry]) {
				lm.Set("one", testEntry{Name: "first", Count: 1})

				cp := lm.GetAll()
				cp["one"] = testEntry{Name: "modified copy", Count: 99}
				cp["two"] = testEntry{Name: "new copy value", Count: 2}
			},
			want: goldenMap[string, testEntry]{
				Map: map[string]testEntry{
					"one": {Name: "first", Count: 1},
				},
			},
		},
		{
			name: "get all slice returns all values",
			run: func(t *testing.T, lm *LockableMap[string, testEntry]) {
				lm.Set("one", testEntry{Name: "first", Count: 1})
				lm.Set("two", testEntry{Name: "second", Count: 2})
			},
			want: goldenMap[string, testEntry]{
				Map: map[string]testEntry{
					"one": {Name: "first", Count: 1},
					"two": {Name: "second", Count: 2},
				},
			},
		},
		{
			name: "get keys returns all keys",
			run: func(t *testing.T, lm *LockableMap[string, testEntry]) {
				lm.Set("one", testEntry{Name: "first", Count: 1})
				lm.Set("two", testEntry{Name: "second", Count: 2})
			},
			want: goldenMap[string, testEntry]{
				Map: map[string]testEntry{
					"one": {Name: "first", Count: 1},
					"two": {Name: "second", Count: 2},
				},
			},
		},
		{
			name: "filtered operations use keys",
			run: func(t *testing.T, lm *LockableMap[string, testEntry]) {
				lm.Set("keep-1", testEntry{Name: "first", Count: 1})
				lm.Set("drop-1", testEntry{Name: "second", Count: 2})
				lm.Set("keep-2", testEntry{Name: "third", Count: 3})
			},
			want: goldenMap[string, testEntry]{
				Map: map[string]testEntry{
					"keep-1": {Name: "first", Count: 1},
					"keep-2": {Name: "third", Count: 3},
					"drop-1": {Name: "second", Count: 2},
				},
			},
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {

			lm := NewLockableMap[string, testEntry]()
			tt.run(t, lm)

			got := lm.GetAll()

			switch tt.name {
			case "get all slice returns all values":
				assertSameEntries(t, lm.GetAllSlice(), []testEntry{
					{Name: "first", Count: 1},
					{Name: "second", Count: 2},
				})
			case "get keys returns all keys":
				assertStringSet(t, lm.GetKeys(), []string{"one", "two"})
			case "filtered operations use keys":
				gotSlice := lm.GetFilteredSlice(func(key string, value testEntry) bool {
					return len(key) >= 5 && key[:5] == "keep-"
				})
				assertSameEntries(t, gotSlice, []testEntry{
					{Name: "first", Count: 1},
					{Name: "third", Count: 3},
				})

				gotFilteredMap := lm.GetFilteredMap(func(key string, value testEntry) bool {
					return len(key) >= 5 && key[:5] == "keep-"
				})
				assertMapEqual(t, gotFilteredMap, map[string]testEntry{
					"keep-1": {Name: "first", Count: 1},
					"keep-2": {Name: "third", Count: 3},
				})
			}

			if tt.name != "filtered operations use keys" {
				assertMapEqual(t, got, tt.want.Map)
			}
		})
	}
}

func TestLockableMapGetMissingReturnsTypedError(t *testing.T) {

	lm := NewLockableMap[string, testEntry]()

	got, err := lm.Get("missing")
	if err == nil {
		t.Fatal("expected an error")
	}

	var keyErr *KeyNotFoundError
	if !errors.As(err, &keyErr) {
		t.Fatalf("expected KeyNotFoundError, got %T", err)
	}

	if keyErr.Key != "missing" {
		t.Fatalf("unexpected error key: %q", keyErr.Key)
	}

	if got != (testEntry{}) {
		t.Fatalf("expected zero value, got %#v", got)
	}
}

func TestLockableMapJSONMarshaling(t *testing.T) {

	lm := NewLockableMap[string, testEntry]()
	lm.Set("one", testEntry{Name: "first", Count: 1})

	got, err := json.Marshal(lm)
	if err != nil {
		t.Fatalf("marshal map: %v", err)
	}

	var decoded map[string]testEntry
	if err := json.Unmarshal(got, &decoded); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}

	want := map[string]testEntry{
		"one": {Name: "first", Count: 1},
	}

	assertMapEqual(t, decoded, want)
}

func TestLockableMapConcurrentAccess(t *testing.T) {

	lm := NewLockableMap[int, int]()

	const (
		workers      = 16
		iterations   = 1_000
		distinctKeys = 32
	)

	var wg sync.WaitGroup
	wg.Add(workers)

	for worker := 0; worker < workers; worker++ {
		worker := worker

		go func() {
			defer wg.Done()

			for i := 0; i < iterations; i++ {
				key := (worker + i) % distinctKeys
				lm.Set(key, i)

				_, _ = lm.Get(key)
				_ = lm.GetKeys()
				_ = lm.GetAll()
				_ = lm.GetAllSlice()
				lm.GetFilteredMap(func(k, v int) bool {
					return k%2 == 0
				})
				_ = lm.GetFilteredSlice(func(k, v int) bool {
					return k%2 == 1
				})

				if i%10 == 0 {
					lm.Remove(key)
				}
			}
		}()
	}

	wg.Wait()

	if got := len(lm.GetAll()); got > distinctKeys {
		t.Fatalf("map contains %d keys; expected at most %d", got, distinctKeys)
	}
}

func TestLockableMapRecordLocksSameKeyAreExclusive(t *testing.T) {

	lm := NewLockableMap[string, testEntry]()
	lm.LockRecord("same-key")

	acquired := make(chan struct{})
	release := make(chan struct{})

	go func() {
		lm.LockRecord("same-key")
		close(acquired)
		<-release
		lm.UnlockRecord("same-key")
	}()

	select {
	case <-acquired:
		t.Fatal("second lock acquired while first lock was held")
	case <-time.After(50 * time.Millisecond):
		// Expected: the second lock is blocked.
	}

	lm.UnlockRecord("same-key")
	close(release)

	select {
	case <-acquired:
	case <-time.After(time.Second):
		t.Fatal("second lock did not acquire after first lock was released")
	}
}

func TestLockableMapRecordLocksDifferentKeysCanProceed(t *testing.T) {

	lm := NewLockableMap[string, testEntry]()
	lm.LockRecord("first")

	acquired := make(chan struct{})

	go func() {
		lm.LockRecord("second")
		close(acquired)
		lm.UnlockRecord("second")
	}()

	select {
	case <-acquired:
		// Expected: a different key has a different lock.
	case <-time.After(time.Second):
		t.Fatal("different-key lock was blocked")
	}

	lm.UnlockRecord("first")
}

func TestLockableMapUnlockAllReleasesHeldLocks(t *testing.T) {

	lm := NewLockableMap[string, testEntry]()
	lm.LockRecord("one")
	lm.LockRecord("two")

	oneAcquired := make(chan struct{})
	twoAcquired := make(chan struct{})

	go func() {
		lm.LockRecord("one")
		close(oneAcquired)
		lm.UnlockRecord("one")
	}()

	go func() {
		lm.LockRecord("two")
		close(twoAcquired)
		lm.UnlockRecord("two")
	}()

	select {
	case <-oneAcquired:
		t.Fatal("one lock acquired before UnlockAll")
	case <-time.After(50 * time.Millisecond):
	}

	select {
	case <-twoAcquired:
		t.Fatal("two lock acquired before UnlockAll")
	case <-time.After(50 * time.Millisecond):
	}

	lm.UnlockAll()

	select {
	case <-oneAcquired:
	case <-time.After(time.Second):
		t.Fatal("one lock was not released by UnlockAll")
	}

	select {
	case <-twoAcquired:
	case <-time.After(time.Second):
		t.Fatal("two lock was not released by UnlockAll")
	}
}

func TestLockableMapRecordLockContextTimeout(t *testing.T) {

	lm := NewLockableMap[string, testEntry]()
	lm.LockRecord("busy")
	defer lm.UnlockRecord("busy")

	ctx, cancel := context.WithTimeout(
		context.Background(),
		50*time.Millisecond,
	)
	defer cancel()

	err := lm.LockRecordContext(ctx, "busy")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected context deadline exceeded, got %v", err)
	}
}

func TestLockableMapRecordLockContextSucceeds(t *testing.T) {

	lm := NewLockableMap[string, testEntry]()

	ctx, cancel := context.WithTimeout(
		context.Background(),
		time.Second,
	)
	defer cancel()

	if err := lm.LockRecordContext(ctx, "available"); err != nil {
		t.Fatalf("lock record: %v", err)
	}

	lm.UnlockRecord("available")
}

func TestLockableMapConcurrentRecordUpdates(t *testing.T) {

	lm := NewLockableMap[string, testEntry]()
	const workers = 16
	const increments = 1_000

	var wg sync.WaitGroup
	wg.Add(workers)

	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()

			for j := 0; j < increments; j++ {
				lm.LockRecord("shared")

				value, err := lm.Get("shared")
				if err != nil {
					var keyErr *KeyNotFoundError
					if !errors.As(err, &keyErr) {
						t.Errorf("get shared: %v", err)
						lm.UnlockRecord("shared")
						return
					}
				}

				value.Count++
				lm.Set("shared", value)
				lm.UnlockRecord("shared")
			}
		}()
	}

	wg.Wait()

	got, err := lm.Get("shared")
	if err != nil {
		t.Fatalf("get final shared value: %v", err)
	}

	want := workers * increments
	if got.Count != want {
		t.Fatalf("final count = %d, want %d", got.Count, want)
	}
}

func TestLockableMapRecordLocksCanBeFlushedWithDefer(t *testing.T) {
	lm := NewLockableMap[string, testEntry]()

	func() {
		lm.LockRecord("one")
		lm.LockRecord("two")
		defer lm.UnlockAll()

		lm.Set("one", testEntry{Name: "one", Count: 1})
		lm.Set("two", testEntry{Name: "two", Count: 2})
	}()

	// Both locks were released by UnlockAll.
	ctx, cancel := context.WithTimeout(
		context.Background(),
		time.Second,
	)
	defer cancel()

	if err := lm.LockRecordContext(ctx, "one"); err != nil {
		t.Fatalf("lock one after flush: %v", err)
	}
	lm.UnlockRecord("one")

	if err := lm.LockRecordContext(ctx, "two"); err != nil {
		t.Fatalf("lock two after flush: %v", err)
	}
	lm.UnlockRecord("two")
}

type goldenMap[T comparable, T2 any] struct {
	Map map[T]T2
}

func assertMapEqual[T comparable, T2 any](
	t *testing.T,
	got map[T]T2,
	want map[T]T2,
) {
	t.Helper()

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("maps differ:\n got: %#v\nwant: %#v", got, want)
	}
}

func assertSameEntries[T any](t *testing.T, got, want []T) {
	t.Helper()

	gotCopy := append([]T(nil), got...)
	wantCopy := append([]T(nil), want...)

	sort.Slice(gotCopy, func(i, j int) bool {
		return fmt.Sprintf("%#v", gotCopy[i]) < fmt.Sprintf("%#v", gotCopy[j])
	})
	sort.Slice(wantCopy, func(i, j int) bool {
		return fmt.Sprintf("%#v", wantCopy[i]) < fmt.Sprintf("%#v", wantCopy[j])
	})

	if !reflect.DeepEqual(gotCopy, wantCopy) {
		t.Fatalf("slices differ:\n got: %#v\nwant: %#v", got, want)
	}
}

func assertStringSet(t *testing.T, got, want []string) {
	t.Helper()

	gotCopy := append([]string(nil), got...)
	wantCopy := append([]string(nil), want...)

	sort.Strings(gotCopy)
	sort.Strings(wantCopy)

	if !reflect.DeepEqual(gotCopy, wantCopy) {
		t.Fatalf("string sets differ:\n got: %#v\nwant: %#v", gotCopy, wantCopy)
	}
}
