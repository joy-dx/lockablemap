# lockablemap

A small, generic, concurrency-safe map wrapper for Go. It provides a `map[K]V`
protected by an internal `sync.RWMutex`, plus helpers for snapshot reads and JSON
marshaling.

## Features

- Generic key/value types: `K comparable`, `V any`
- Safe concurrent `Set`, `Get`, and `Remove`
- Snapshot reads via `GetAll()` (returns a copy, not the underlying map)
- `GetAllSlice()` returns a slice of values (order not guaranteed)
- `MarshalJSON()` is lock-aware (safe under concurrent access)
- A typed error for missing keys: `*KeyNotFoundError`

## Installation

```bash
go get github.com/joy-dx/lockablemap
```

## Quick start

```go
package lockablemap

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/joy-dx/lockablemap"
)

func main() {
	lm := lockablemap.NewLockableMap[string, int]()

	lm.Set("a", 1)
	lm.Set("b", 2)

	v, err := lm.Get("a")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("a =", v)

	// Snapshot of the whole map (safe to use without holding a lock).
	snap := lm.GetAll()
	fmt.Println("snapshot:", snap)

	// Values only (order is unspecified).
	values := lm.GetAllSlice()
	fmt.Println("values:", values)

	// JSON marshaling uses a read lock internally.
	b, err := json.Marshal(&lm)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(string(b))
}
```

## API

### `type LockableMap[K comparable, V any]`

```go
type LockableMap[K comparable, V any] struct {
	Map map[K]V `json:"map" yaml:"map"`
	mu  sync.RWMutex
}
```

> Note: `Map` is exported to allow encoding/inspection, but you should treat it
> as an implementation detail and avoid reading/writing it directly from
> multiple goroutines. Use the methods below for concurrency safety.

### Constructor

```go
func NewLockableMap[K comparable, V any]() LockableMap[K, V]
```

Creates a `LockableMap` with an initialized underlying map.

### Methods

- `Set(key K, value V)`
- `Remove(key K)`
- `Get(key K) (V, error)`
    - returns `*KeyNotFoundError` if the key does not exist
- `GetAll() map[K]V`
    - returns a **copy** (snapshot) of the map
- `GetAllSlice() []V`
    - returns a slice of values; order is unspecified
- `MarshalJSON() ([]byte, error)`
    - marshals the underlying map with a read lock held

## Errors

### `KeyNotFoundError`

```go
type KeyNotFoundError struct {
	Key any
}

func (e *KeyNotFoundError) Error() string {
	return fmt.Sprintf("key not found: %v", e.Key)
}
```

Example:

```go
v, err := lm.Get("missing")
if err != nil {
	var knf *lockablemap.KeyNotFoundError
	if errors.As(err, &knf) {
		fmt.Println("missing key:", knf.Key)
	}
}
```

## Concurrency notes / guarantees

- All provided methods are safe for concurrent use.
- `GetAll()` returns a snapshot copy so callers can iterate/mutate the returned
  map without racing the internal state.
- `GetAllSlice()` does not guarantee ordering (Go map iteration is randomized).
- If `V` contains pointers or reference types, the wrapper does **not** provide
  deep immutability. It only protects access to the map itself.

## Tests and benchmarks

Run unit tests:

```bash
go test ./...
```

Run with the race detector:

```bash
go test -race ./...
```

Run benchmarks:

```bash
go test -bench . -benchmem ./...
```

## When should I use this?

This wrapper can be useful when you want:

- a simple, strongly-typed API around a map
- encapsulated locking (to reduce accidental data races)
- snapshot reads and lock-safe marshaling

If you need very specific high-contention behavior, consider alternatives like:

- a plain `map` with external lock control (often simplest)
- `sync.Map` for certain read-heavy, key-stable workloads

## Benchmark

```
goos: darwin
goarch: arm64
pkg: github.com/joy-dx/lockablemap
cpu: Apple M4 Pro
BenchmarkLockableMap_Set-14                                     21673803               140.4 ns/op            55 B/op       0 allocs/op
BenchmarkLockableMap_Get_Hit-14                                 142134645                8.305 ns/op           0 B/op       0 allocs/op
BenchmarkLockableMap_Get_Miss-14                                53772475                21.43 ns/op           24 B/op       2 allocs/op
BenchmarkLockableMap_Remove-14                                  21870090               135.7 ns/op             0 B/op       0 allocs/op
BenchmarkLockableMap_GetAll_Snapshot-14                             3706            320893 ns/op          591153 B/op      66 allocs/op
BenchmarkLockableMap_GetAllSlice-14                                12732             93289 ns/op          131072 B/op       1 allocs/op
BenchmarkLockableMap_MarshalJSON-14                                 1644            714187 ns/op          378549 B/op    8195 allocs/op
BenchmarkLockableMap_Set_Parallel-14                             4516569               349.4 ns/op            67 B/op       0 allocs/op
BenchmarkLockableMap_Get_Hit_Parallel-14                         8654930               138.8 ns/op             0 B/op       0 allocs/op
BenchmarkLockableMap_Mixed_Parallel_90Read10Write-14            13295649                89.27 ns/op            0 B/op       0 allocs/op
BenchmarkLockableMap_GetAll_Parallel-14                            14694             80563 ns/op          591157 B/op      66 allocs/op
BenchmarkLockableMap_MarshalJSON_Parallel-14                        7068            142402 ns/op          391766 B/op    8199 allocs/op
BenchmarkLockableMap_Contended_ReadMostly-14                     3882816               290.0 ns/op             0 B/op       0 allocs/op
```