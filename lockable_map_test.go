package lockablemap

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
)

// --- Golden/table scaffolding ---

type opKind int

const (
	opSet opKind = iota
	opRemove
	opGetOK
	opGetMissing
	opGetAll
	opGetAllSlice
	opMarshalJSON
	opGetKeys
	opGetFilteredSlice
	opGetFilteredMap
)

type step[K comparable, V any] struct {
	name string
	op   opKind

	key   K
	value V

	filter func(K) bool

	wantValue V
	wantMap   map[K]V
	wantSlice []V
	wantKeys  []K
}

type caseDef[K comparable, V any] struct {
	name  string
	steps []step[K, V]
}

// --- Assertions/helpers ---

func assertKeysMultisetEqual[K comparable](t *testing.T, got, want []K) {
	t.Helper()

	count := func(s []K) map[K]int {
		m := make(map[K]int, len(s))
		for _, v := range s {
			m[v]++
		}
		return m
	}

	if !reflect.DeepEqual(count(got), count(want)) {
		t.Fatalf("keys mismatch: got=%v want=%v", got, want)
	}
}

func assertKeyNotFound[K comparable](t *testing.T, err error, wantKey K) {
	t.Helper()

	if err == nil {
		t.Fatalf("expected *KeyNotFoundError, got nil")
	}

	var knf *KeyNotFoundError
	if !errors.As(err, &knf) {
		t.Fatalf("expected *KeyNotFoundError, got %T (%v)", err, err)
	}

	// Key is `any`, so we can compare it to wantKey via DeepEqual
	if !reflect.DeepEqual(knf.Key, wantKey) {
		t.Fatalf(
			"KeyNotFoundError key mismatch: got=%v (%T) want=%v (%T)",
			knf.Key,
			knf.Key,
			wantKey,
			wantKey,
		)
	}
}

func assertMapEqual[K comparable, V any](t *testing.T, got, want map[K]V) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("map mismatch:\n  got:  %#v\n  want: %#v", got, want)
	}
}

// Map iteration order is random. GetAllSlice returns values in unspecified order,
// so we compare as multisets by counting occurrences.
//
// Note: requires V comparable. If you want V any, switch to reflect-based counting
// or sort with a custom comparator (if possible).
func assertSliceMultisetEqual[V comparable](
	t *testing.T,
	got, want []V,
) {
	t.Helper()

	count := func(s []V) map[V]int {
		m := make(map[V]int, len(s))
		for _, v := range s {
			m[v]++
		}
		return m
	}

	gm := count(got)
	wm := count(want)
	if !reflect.DeepEqual(gm, wm) {
		t.Fatalf(
			"slice multiset mismatch:\n  got:  %#v (counts=%#v)\n  want: %#v (counts=%#v)",
			got,
			gm,
			want,
			wm,
		)
	}
}

// --- Tests ---

func TestLockableMap_GoldenTable_StringInt(t *testing.T) {
	tests := []caseDef[string, int]{
		{
			name: "get_missing_from_empty",
			steps: []step[string, int]{
				{
					name: "get missing",
					op:   opGetMissing,
					key:  "nope",
				},
			},
		},
		{
			name: "set_get_remove_get",
			steps: []step[string, int]{
				{name: "set a=1", op: opSet, key: "a", value: 1},
				{name: "get a ok", op: opGetOK, key: "a", wantValue: 1},
				{name: "remove a", op: opRemove, key: "a"},
				{name: "get a missing", op: opGetMissing, key: "a"},
			},
		},
		{
			name: "getall_returns_snapshot_copy",
			steps: []step[string, int]{
				{name: "set a=1", op: opSet, key: "a", value: 1},
				{name: "set b=2", op: opSet, key: "b", value: 2},
				{
					name:    "getall snapshot",
					op:      opGetAll,
					wantMap: map[string]int{"a": 1, "b": 2},
				},
			},
		},
		{
			name: "getallslice_values",
			steps: []step[string, int]{
				{name: "set a=1", op: opSet, key: "a", value: 1},
				{name: "set b=2", op: opSet, key: "b", value: 2},
				{
					name:      "get all slice",
					op:        opGetAllSlice,
					wantSlice: []int{1, 2},
				},
			},
		},
		{
			name: "marshaljson_matches_snapshot",
			steps: []step[string, int]{
				{name: "set a=1", op: opSet, key: "a", value: 1},
				{name: "set b=2", op: opSet, key: "b", value: 2},
				{name: "marshal json", op: opMarshalJSON},
			},
		},
		{
			name: "getkeys",
			steps: []step[string, int]{
				{name: "set a=1", op: opSet, key: "a", value: 1},
				{name: "set b=2", op: opSet, key: "b", value: 2},
				{
					name:     "get keys",
					op:       opGetKeys,
					wantKeys: []string{"a", "b"},
				},
			},
		},
		{
			name: "filtered_slice",
			steps: []step[string, int]{
				{name: "set apple=1", op: opSet, key: "apple", value: 1},
				{name: "set banana=2", op: opSet, key: "banana", value: 2},
				{
					name:      "filter prefix a",
					op:        opGetFilteredSlice,
					filter:    func(k string) bool { return strings.HasPrefix(k, "a") },
					wantSlice: []int{1},
				},
			},
		},
		{
			name: "filtered_map",
			steps: []step[string, int]{
				{name: "set apple=1", op: opSet, key: "apple", value: 1},
				{name: "set banana=2", op: opSet, key: "banana", value: 2},
				{
					name:    "filter prefix b",
					op:      opGetFilteredMap,
					filter:  func(k string) bool { return strings.HasPrefix(k, "b") },
					wantMap: map[string]int{"banana": 2},
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			lm := NewLockableMap[string, int]()

			for _, st := range tc.steps {
				t.Run(st.name, func(t *testing.T) {
					switch st.op {
					case opSet:
						lm.Set(st.key, st.value)

					case opRemove:
						lm.Remove(st.key)

					case opGetOK:
						got, err := lm.Get(st.key)
						if err != nil {
							t.Fatalf("unexpected error: %v", err)
						}
						if got != st.wantValue {
							t.Fatalf(
								"value mismatch: got=%v want=%v",
								got,
								st.wantValue,
							)
						}

					case opGetMissing:
						_, err := lm.Get(st.key)
						assertKeyNotFound(t, err, st.key)

					case opGetAll:
						got := lm.GetAll()
						assertMapEqual(t, got, st.wantMap)

						// Ensure it's a copy: mutation doesn't leak back.
						got["mutate"] = 999
						after := lm.GetAll()
						if _, ok := after["mutate"]; ok {
							t.Fatalf(
								"GetAll returned an alias to internal map; mutation leaked back",
							)
						}

					case opGetAllSlice:
						got := lm.GetAllSlice()

						if len(got) != len(st.wantSlice) {
							t.Fatalf(
								"slice length mismatch: got=%d want=%d; got=%#v",
								len(got),
								len(st.wantSlice),
								got,
							)
						}
						assertSliceMultisetEqual(t, got, st.wantSlice)

					case opMarshalJSON:
						// JSON map key order is not guaranteed, so round-trip into a map.
						b, err := json.Marshal(&lm)
						if err != nil {
							t.Fatalf("unexpected marshal error: %v", err)
						}

						var got map[string]int
						if err := json.Unmarshal(b, &got); err != nil {
							t.Fatalf(
								"unmarshal marshaled JSON failed: %v; json=%s",
								err,
								string(b),
							)
						}

						want := lm.GetAll()
						assertMapEqual(t, got, want)

					case opGetKeys:
						got := lm.GetKeys()
						assertKeysMultisetEqual(t, got, st.wantKeys)

					case opGetFilteredSlice:
						got := lm.GetFilteredSlice(st.filter)
						assertSliceMultisetEqual(t, got, st.wantSlice)

					case opGetFilteredMap:
						got := lm.GetFilteredMap(st.filter)
						assertMapEqual(t, got, st.wantMap)

					default:
						t.Fatalf("unknown op: %v", st.op)
					}
				})
			}
		})
	}
}

func TestLockableMap_GoldenTable_IntKey_KeyNotFoundCarriesKey(t *testing.T) {
	tests := []caseDef[int, string]{
		{
			name: "missing_key_in_error_is_int",
			steps: []step[int, string]{
				{name: "get missing 42", op: opGetMissing, key: 42},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			lm := NewLockableMap[int, string]()

			for _, st := range tc.steps {
				t.Run(st.name, func(t *testing.T) {
					switch st.op {
					case opGetMissing:
						_, err := lm.Get(st.key)
						assertKeyNotFound(t, err, st.key)
					default:
						t.Fatalf("unexpected op in this suite: %v", st.op)
					}
				})
			}
		})
	}
}

func TestLockableMap_Concurrency_Smoke(t *testing.T) {
	// Run with: go test -race ./...
	lm := NewLockableMap[int, int]()

	const (
		writers    = 8
		readers    = 8
		iterations = 10_000
	)

	var wg sync.WaitGroup
	wg.Add(writers + readers)

	for w := 0; w < writers; w++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				key := (i + id) % 1024
				lm.Set(key, i)
				if i%10 == 0 {
					lm.Remove((key + 1) % 1024)
				}
			}
		}(w)
	}

	for r := 0; r < readers; r++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				key := (i + id) % 1024
				_, _ = lm.Get(key)
				_ = lm.GetAll()
				_ = lm.GetAllSlice()
				_, _ = json.Marshal(&lm)
			}
		}(r)
	}

	wg.Wait()
}
