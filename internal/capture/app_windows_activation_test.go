//go:build windows

package capture

import (
	"sync"
	"syscall"
	"testing"
	"unsafe"
)

// fakeComObject counts Release calls, so a dropped client is observable.
type fakeComObject struct {
	vtbl     *[3]uintptr
	released int
}

var (
	fakeComMu      sync.Mutex
	fakeComObjects = map[uintptr]*fakeComObject{}
)

func fakeComRelease(this uintptr) uintptr {
	fakeComMu.Lock()
	defer fakeComMu.Unlock()
	if o, ok := fakeComObjects[this]; ok {
		o.released++
	}
	return 0
}

func newFakeComObject() (uintptr, *fakeComObject) {
	o := &fakeComObject{vtbl: &[3]uintptr{0, 0, syscall.NewCallback(fakeComRelease)}}
	this := uintptr(unsafe.Pointer(o))
	fakeComMu.Lock()
	fakeComObjects[this] = o
	fakeComMu.Unlock()
	return this, o
}

func releasedCount(o *fakeComObject) int {
	fakeComMu.Lock()
	defer fakeComMu.Unlock()
	return o.released
}

// TestActivationResultGoesToItsOwnHandler pins the routing key: two concurrent
// activations each receive the client their own completion produced.
func TestActivationResultGoesToItsOwnHandler(t *testing.T) {
	a := &handlerObject{vtbl: &handlerVTbl, ch: make(chan appClient, 1)}
	b := &handlerObject{vtbl: &handlerVTbl, ch: make(chan appClient, 1)}
	thisA := registerHandler(a)
	defer retireHandler(thisA, a)
	thisB := registerHandler(b)
	defer retireHandler(thisB, b)

	select {
	case got := <-a.ch:
		t.Fatalf("A had %+v before its delivery", got)
	default:
	}
	select {
	case got := <-b.ch:
		t.Fatalf("B had %+v before its delivery", got)
	default:
	}

	clientA, _ := newFakeComObject()
	clientB, _ := newFakeComObject()
	deliverActivation(thisA, appClient{client: clientA})
	deliverActivation(thisB, appClient{client: clientB})

	gotA := <-a.ch
	if gotA.client != clientA {
		t.Fatalf("A got %+v, want its own client %#x", gotA, clientA)
	}
	gotB := <-b.ch
	if gotB.client != clientB {
		t.Fatalf("B got %+v, want its own client %#x", gotB, clientB)
	}
}

// TestRetiredHandlerReleasesLateActivation pins the timeout path: a completion
// that arrives after the waiter gave up must not sit in an unread buffer, and
// its interface must be released.
func TestRetiredHandlerReleasesLateActivation(t *testing.T) {
	h := &handlerObject{vtbl: &handlerVTbl, ch: make(chan appClient, 1)}
	this := registerHandler(h)
	retireHandler(this, h)

	client, obj := newFakeComObject()
	deliverActivation(this, appClient{client: client})

	select {
	case got := <-h.ch:
		t.Fatalf("retired handler buffered %+v", got)
	default:
	}
	if got := releasedCount(obj); got != 1 {
		t.Fatalf("late client Release calls = %d, want 1", got)
	}
}
