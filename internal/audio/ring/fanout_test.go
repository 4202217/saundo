package ring

import "testing"

func TestFanoutFeedsEverySinkIndependently(t *testing.T) {
	left, right := New(8), New(8)
	f := NewFanout(left, right)
	f.Push([]int16{1, 2, 3, 4})

	leftOut := make([]int16, 2)
	if n := left.Pop(leftOut); n != 2 || leftOut[0] != 1 || leftOut[1] != 2 {
		t.Fatalf("left pop = %d %v", n, leftOut)
	}
	// The right member must still see the whole packet after the left one
	// consumed part of it.
	rightOut := make([]int16, 8)
	if n := right.Pop(rightOut); n != 4 || rightOut[0] != 1 || rightOut[3] != 4 {
		t.Fatalf("right pop = %d %v", n, rightOut[:n])
	}
}

func TestFanoutAddRemoveReset(t *testing.T) {
	first, second := New(4), New(4)
	f := NewFanout(first)
	if f.Add(first) {
		t.Fatal("duplicate add accepted")
	}
	f.Add(second)
	f.Push([]int16{1, 2})
	f.Remove(first)
	f.Push([]int16{3, 4})

	out := make([]int16, 4)
	if n := first.Pop(out); n != 2 {
		t.Fatalf("removed sink received a push: n=%d", n)
	}
	if n := second.Pop(out); n != 4 {
		t.Fatalf("remaining sink pop = %d", n)
	}
	f.Reset()
	if first.Available() != 0 || second.Available() != 0 {
		t.Fatal("reset did not clear the sinks")
	}
}

func TestFanoutKeepNewestTrimsEverySinkToOneWindow(t *testing.T) {
	left, right := New(16), New(16)
	f := NewFanout(left, right)
	f.Push([]int16{1, 2, 3, 4, 5, 6})
	if drop := f.KeepNewest(2); drop != 4 {
		t.Fatalf("drop = %d, want 4", drop)
	}
	for i, b := range []*Buffer{left, right} {
		if got := b.Available(); got != 2 {
			t.Fatalf("ring %d available = %d, want 2", i, got)
		}
		var dst [2]int16
		if n := b.Pop(dst[:]); n != 2 || dst != [2]int16{5, 6} {
			t.Fatalf("ring %d kept %v (n=%d), want the newest window [5 6]", i, dst, n)
		}
		if b.Dropped() != 0 {
			t.Fatalf("ring %d counted a policy trim as a capture drop: %d", i, b.Dropped())
		}
	}
}

func TestFanoutReportsLargestDrop(t *testing.T) {
	left, right := New(2), New(8)
	f := NewFanout(left, right)
	f.Push([]int16{1, 2, 3, 4})
	if got := f.Dropped(); got != 2 {
		t.Fatalf("dropped = %d, want 2 from the small sink", got)
	}
}

func TestFanoutPushDoesNotAllocate(t *testing.T) {
	f := NewFanout(New(4096), New(4096))
	block := make([]int16, 512)
	f.Push(block)
	if got := testing.AllocsPerRun(1000, func() { f.Push(block) }); got != 0 {
		t.Fatalf("Fanout.Push allocates %.1f times per run", got)
	}
}
