package ring

import (
	"sync"
	"testing"
)

func TestPushPop(t *testing.T) {
	b := New(8)
	if got := b.Push([]int16{1, 2, 3, 4}); got != 0 {
		t.Fatalf("drop = %d, want 0", got)
	}
	if b.Available() != 4 {
		t.Fatalf("available = %d, want 4", b.Available())
	}
	out := make([]int16, 3)
	if n := b.Pop(out); n != 3 || out[0] != 1 || out[2] != 3 {
		t.Fatalf("pop = %d %v", n, out)
	}
	if n := b.Pop(out); n != 1 || out[0] != 4 {
		t.Fatalf("pop = %d %v", n, out)
	}
	if n := b.Pop(out); n != 0 {
		t.Fatalf("pop empty = %d", n)
	}
}

func TestWraps(t *testing.T) {
	b := New(5)
	b.Push([]int16{1, 2, 3})
	out := make([]int16, 2)
	b.Pop(out)
	b.Push([]int16{4, 5, 6, 7})
	got := make([]int16, 5)
	n := b.Pop(got)
	want := []int16{3, 4, 5, 6, 7}
	if n != 5 {
		t.Fatalf("n = %d", n)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestOverwriteOldest(t *testing.T) {
	b := New(4)
	if drop := b.Push([]int16{1, 2, 3, 4, 5, 6}); drop != 2 {
		t.Fatalf("drop = %d, want 2", drop)
	}
	got := make([]int16, 8)
	n := b.Pop(got)
	if n != 4 {
		t.Fatalf("n = %d", n)
	}
	for i, want := range []int16{3, 4, 5, 6} {
		if got[i] != want {
			t.Fatalf("got %v, want [3 4 5 6]", got[:n])
		}
	}
	if b.Dropped() != 2 {
		t.Fatalf("dropped = %d, want 2", b.Dropped())
	}
}

func TestOversizedPushKeepsTail(t *testing.T) {
	b := New(3)
	b.Push([]int16{1, 2, 3, 4, 5})
	got := make([]int16, 4)
	n := b.Pop(got)
	if n != 3 || got[0] != 3 || got[1] != 4 || got[2] != 5 {
		t.Fatalf("got %v", got[:n])
	}
}

func TestKeepNewestKeepsTheNewestSamples(t *testing.T) {
	b := New(16)
	b.Push([]int16{1, 2, 3, 4, 5, 6})
	if drop := b.KeepNewest(2); drop != 4 {
		t.Fatalf("drop = %d, want 4", drop)
	}
	if got := b.Available(); got != 2 {
		t.Fatalf("available = %d, want 2", got)
	}
	out := make([]int16, 4)
	if n := b.Pop(out); n != 2 || out[0] != 5 || out[1] != 6 {
		t.Fatalf("pop = %d %v, want [5 6]", n, out[:n])
	}
	if b.Dropped() != 0 {
		t.Fatalf("policy trim counted as a capture drop: %d", b.Dropped())
	}
}

func TestKeepNewestZeroNegativeAndOversized(t *testing.T) {
	b := New(8)
	b.Push([]int16{1, 2, 3, 4})
	if drop := b.KeepNewest(0); drop != 4 {
		t.Fatalf("zero trim drop = %d, want 4", drop)
	}
	if b.Available() != 0 {
		t.Fatalf("available after zero trim = %d", b.Available())
	}
	b.Push([]int16{1, 2, 3})
	if drop := b.KeepNewest(10); drop != 0 {
		t.Fatalf("oversized trim drop = %d, want 0", drop)
	}
	if b.Available() != 3 {
		t.Fatalf("available after oversized trim = %d", b.Available())
	}
	if drop := b.KeepNewest(-1); drop != 3 {
		t.Fatalf("negative trim drop = %d, want 3", drop)
	}
	if b.Dropped() != 0 {
		t.Fatalf("trim counted as a capture drop: %d", b.Dropped())
	}
}

func TestKeepNewestWrappedBuffer(t *testing.T) {
	b := New(4)
	b.Push([]int16{1, 2, 3})
	var out [2]int16
	b.Pop(out[:])
	b.Push([]int16{4, 5, 6, 7}) // wraps: the live window is 4 5 6 7
	if drop := b.KeepNewest(2); drop != 2 {
		t.Fatalf("drop = %d, want 2", drop)
	}
	got := make([]int16, 4)
	if n := b.Pop(got); n != 2 || got[0] != 6 || got[1] != 7 {
		t.Fatalf("pop = %d %v, want [6 7]", n, got[:n])
	}
}

func TestConcurrentProducerConsumer(t *testing.T) {
	const total = 20000
	b := New(total * 2)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		block := []int16{1, 2, 3, 4}
		for i := 0; i < total/len(block); i++ {
			b.Push(block)
		}
	}()
	received := 0
	out := make([]int16, 8)
	for received < total {
		n := b.Pop(out)
		if n == 0 {
			continue
		}
		for _, sample := range out[:n] {
			if sample != 1 && sample != 2 && sample != 3 && sample != 4 {
				t.Fatalf("corrupt sample %d", sample)
			}
		}
		received += n
	}
	wg.Wait()
}
