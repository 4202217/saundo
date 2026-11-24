package engine

import (
	"testing"
	"time"

	"saundo/internal/airplay/sender"
)

func members(head float64, under, retrans uint64) []sender.Stats {
	return []sender.Stats{{BufferMS: head, BufferValid: true, Underruns: under, Retransmits: retrans}}
}

func TestStreamHistoryBucketsAndMin(t *testing.T) {
	var h streamHistory
	base := time.Date(2026, 11, 24, 10, 0, 0, 0, time.UTC)
	h.sample(base, members(30, 0, 0), false)
	h.sample(base.Add(100*time.Millisecond), members(0, 0, 0), false)
	h.sample(base.Add(200*time.Millisecond), members(20, 4, 0), false)
	h.sample(base.Add(300*time.Millisecond), members(25, 4, 2), false)

	now := base.Add(400 * time.Millisecond)
	if got, ok := h.minHead(now); !ok || got != 0 {
		t.Fatalf("min head = %v/%v, want 0/true", got, ok)
	}
	var dst [streamBucketCount]StreamBucket
	buckets := h.buckets(now, dst[:])
	if len(buckets) != streamBucketCount {
		t.Fatalf("buckets = %d, want %d", len(buckets), streamBucketCount)
	}
	last := buckets[streamBucketCount-1]
	if !last.Valid || last.HeadMS != 0 {
		t.Fatalf("newest bucket = %+v, want the 0 ms low", last)
	}
	if last.Underruns != 4 || last.Retransmits != 2 {
		t.Fatalf("events = %d/%d, want 4/2", last.Underruns, last.Retransmits)
	}
	for i := 0; i < streamBucketCount-1; i++ {
		if buckets[i].Valid || buckets[i].Underruns != 0 {
			t.Fatalf("older bucket %d = %+v, want empty", i, buckets[i])
		}
	}
}

func TestStreamHistoryResetsWhenCountersGoBack(t *testing.T) {
	var h streamHistory
	base := time.Date(2026, 11, 24, 10, 0, 0, 0, time.UTC)
	h.sample(base, members(30, 7, 1), false)
	h.sample(base.Add(100*time.Millisecond), members(30, 0, 0), false)
	now := base.Add(200 * time.Millisecond)
	if got, ok := h.minHead(now); !ok || got != 30 {
		t.Fatalf("min head = %v/%v, want 30/true", got, ok)
	}
	var dst [streamBucketCount]StreamBucket
	h.buckets(now, dst[:])
	if last := dst[streamBucketCount-1]; last.Underruns != 0 {
		t.Fatalf("a new session kept an old event: %+v", last)
	}
}

func TestRateSampleKeepsThePreviousValue(t *testing.T) {
	var r rateSample
	base := time.Date(2026, 11, 24, 10, 0, 0, 0, time.UTC)
	if _, ok := r.rate(base, 1000); ok {
		t.Fatal("the first sample reported a rate")
	}
	if _, ok := r.rate(base.Add(500*time.Millisecond), 2000); ok {
		t.Fatal("a half-second window reported a rate")
	}
	if got, ok := r.rate(base.Add(1500*time.Millisecond), 4000); !ok || got != 2000 {
		t.Fatalf("rate = %v/%v, want 2000/true", got, ok)
	}
	if got, ok := r.rate(base.Add(1600*time.Millisecond), 4100); !ok || got != 2000 {
		t.Fatalf("an event-driven sample changed the rate: %v/%v", got, ok)
	}
}

func TestStreamHistoryIdleDoesNotLowerTheMin(t *testing.T) {
	var h streamHistory
	base := time.Date(2026, 11, 24, 10, 0, 0, 0, time.UTC)
	h.sample(base, members(30, 0, 0), false)
	h.sample(base.Add(100*time.Millisecond), members(0, 0, 0), true)
	now := base.Add(200 * time.Millisecond)
	if got, ok := h.minHead(now); !ok || got != 30 {
		t.Fatalf("min head = %v/%v, want the 30 ms before the idle second", got, ok)
	}
	var dst [streamBucketCount]StreamBucket
	buckets := h.buckets(now, dst[:])
	if last := buckets[streamBucketCount-1]; !last.Idle {
		t.Fatalf("the idle second is not marked: %+v", last)
	}
}
