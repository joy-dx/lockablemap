package lockablemap

import (
	"encoding/json"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
)

// Bench notes:
// - Run: go test -bench . -benchmem ./...
// - Add race if you want (slower): go test -race -bench . ./...
// - Most benches prefill the map so Get paths are realistic.

func BenchmarkLockableMap_Set(b *testing.B) {
	lm := NewLockableMap[int, int]()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		lm.Set(i, i)
	}
}

func BenchmarkLockableMap_Get_Hit(b *testing.B) {
	lm := NewLockableMap[int, int]()
	const keys = 1 << 16
	for i := 0; i < keys; i++ {
		lm.Set(i, i)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := lm.Get(i & (keys - 1))
		if err != nil {
			b.Fatalf("unexpected error: %v", err)
		}
	}
}

func BenchmarkLockableMap_Get_Miss(b *testing.B) {
	lm := NewLockableMap[int, int]()
	const keys = 1 << 16
	for i := 0; i < keys; i++ {
		lm.Set(i, i)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := lm.Get((i & (keys - 1)) + keys) // guaranteed miss
		if err == nil {
			b.Fatalf("expected error, got nil")
		}
	}
}

func BenchmarkLockableMap_Remove(b *testing.B) {
	lm := NewLockableMap[int, int]()
	// Pre-populate with at least b.N keys so removes usually hit.
	for i := 0; i < b.N; i++ {
		lm.Set(i, i)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		lm.Remove(i)
	}
}

func BenchmarkLockableMap_GetAll_Snapshot(b *testing.B) {
	lm := NewLockableMap[int, int]()
	const keys = 1 << 14
	for i := 0; i < keys; i++ {
		lm.Set(i, i)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = lm.GetAll()
	}
}

func BenchmarkLockableMap_GetAllSlice(b *testing.B) {
	lm := NewLockableMap[int, int]()
	const keys = 1 << 14
	for i := 0; i < keys; i++ {
		lm.Set(i, i)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = lm.GetAllSlice()
	}
}

func BenchmarkLockableMap_MarshalJSON(b *testing.B) {
	lm := NewLockableMap[string, int]()
	const keys = 1 << 12
	for i := 0; i < keys; i++ {
		lm.Set("k"+strconv.Itoa(i), i)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := json.Marshal(&lm)
		if err != nil {
			b.Fatalf("unexpected error: %v", err)
		}
	}
}

// Parallel benchmarks

func BenchmarkLockableMap_Set_Parallel(b *testing.B) {
	lm := NewLockableMap[int, int]()

	var ctr uint64
	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			i := int(atomic.AddUint64(&ctr, 1))
			lm.Set(i, i)
		}
	})
}

func BenchmarkLockableMap_Get_Hit_Parallel(b *testing.B) {
	lm := NewLockableMap[int, int]()
	const keys = 1 << 16
	for i := 0; i < keys; i++ {
		lm.Set(i, i)
	}

	var ctr uint64
	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			i := int(atomic.AddUint64(&ctr, 1))
			_, err := lm.Get(i & (keys - 1))
			if err != nil {
				b.Fatalf("unexpected error: %v", err)
			}
		}
	})
}

func BenchmarkLockableMap_Mixed_Parallel_90Read10Write(b *testing.B) {
	lm := NewLockableMap[int, int]()
	const keys = 1 << 16
	for i := 0; i < keys; i++ {
		lm.Set(i, i)
	}

	var ctr uint64
	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			n := atomic.AddUint64(&ctr, 1)
			i := int(n)

			// ~10% writes, 90% reads
			if n%10 == 0 {
				lm.Set(i&(keys-1), i)
				continue
			}

			_, err := lm.Get(i & (keys - 1))
			if err != nil {
				b.Fatalf("unexpected error: %v", err)
			}
		}
	})
}

func BenchmarkLockableMap_GetAll_Parallel(b *testing.B) {
	lm := NewLockableMap[int, int]()
	const keys = 1 << 14
	for i := 0; i < keys; i++ {
		lm.Set(i, i)
	}

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = lm.GetAll()
		}
	})
}

func BenchmarkLockableMap_MarshalJSON_Parallel(b *testing.B) {
	lm := NewLockableMap[string, int]()
	const keys = 1 << 12
	for i := 0; i < keys; i++ {
		lm.Set("k"+strconv.Itoa(i), i)
	}

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, err := json.Marshal(&lm)
			if err != nil {
				b.Fatalf("unexpected error: %v", err)
			}
		}
	})
}

// Contended scenario: writers + readers in fixed goroutines.
// This can be useful alongside RunParallel to resemble real services.
func BenchmarkLockableMap_Contended_ReadMostly(b *testing.B) {
	lm := NewLockableMap[int, int]()
	const keys = 1 << 16
	for i := 0; i < keys; i++ {
		lm.Set(i, i)
	}

	b.ReportAllocs()
	b.ResetTimer()

	var wg sync.WaitGroup
	stop := make(chan struct{})

	// Writers
	wg.Add(2)
	for w := 0; w < 2; w++ {
		go func(id int) {
			defer wg.Done()
			i := id
			for {
				select {
				case <-stop:
					return
				default:
					lm.Set(i&(keys-1), i)
					i += 17
				}
			}
		}(w)
	}

	// Readers: benchmark loop (single goroutine) to measure throughput while
	// background writers contend.
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = lm.Get(i & (keys - 1))
	}

	close(stop)
	wg.Wait()
}
