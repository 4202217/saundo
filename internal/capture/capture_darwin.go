//go:build darwin

package capture

import (
	"errors"
	"fmt"
	"sync"
	"time"
	"unsafe"

	"github.com/ebitengine/purego"

	"saundo/internal/darwin"
)

// Capture runs on one goroutine. It taps the output device's process mix
// through a Core Audio process tap, aggregates the tap with that device, and
// reads the tap's mix through an IOProc into the ring as interleaved stereo
// int16 at the tap rate. When the default device changes or the stream
// breaks, it tears the tap down and reopens with a short backoff.

// ioProcEntry is the single C function pointer the package registers; purego
// callback slots are finite, so streams register themselves in ioRegistry
// instead of each allocating a callback.
var ioProcPtr = sync.OnceValue(func() uintptr {
	return purego.NewCallback(ioProcEntry)
})

var (
	ioRegistryMu sync.Mutex
	ioRegistry   = map[uint32]*stream{}
)

func ioProcEntry(device uintptr, now *darwin.AudioTimeStamp, input *darwin.AudioBufferList,
	inTime *darwin.AudioTimeStamp, output unsafe.Pointer, outTime *darwin.AudioTimeStamp) int32 {
	ioRegistryMu.Lock()
	st := ioRegistry[uint32(device)]
	ioRegistryMu.Unlock()
	if st != nil {
		st.push(input)
	}
	return 0
}

// Start opens the tap capture and returns once the first stream is running.
// If the first open fails, so does Start.
func Start(cfg Config) (*Source, error) {
	if cfg.Ring == nil {
		return nil, errors.New("capture: Ring is required")
	}
	if cfg.ProcessID != 0 {
		// Fail closed: per-application capture is a Windows feature here.
		return nil, errors.New("capture: per-application capture is unsupported on this platform")
	}
	s := &Source{cfg: cfg, stop: make(chan struct{}), done: make(chan struct{})}
	ready := make(chan error, 1)
	go s.run(ready)
	if err := <-ready; err != nil {
		<-s.done
		return nil, err
	}
	return s, nil
}

// run drives capture, reopening with a backoff after device changes and
// stream breaks. It sends exactly one value on ready: nil once the first
// stream is open, or the reason it could not.
func (s *Source) run(ready chan<- error) {
	defer close(s.done)

	backoff := retryDelayFirst
	first := true
	for {
		st, err := openStream(s)
		if err != nil {
			if first {
				ready <- err
				return
			}
			s.reportError(err)
			if !s.wait(backoff) {
				return
			}
			backoff = min(backoff*2, retryDelayMax)
			continue
		}
		wasFirst := first
		first = false
		backoff = retryDelayFirst
		s.streamOpened(st.rate, st.name)
		if wasFirst {
			ready <- nil
		}

		err = st.pump(s, s.cfg.EndpointID == "")
		st.close()
		s.streamClosed()
		if err == nil || errors.Is(err, errStopped) {
			return
		}
		if errors.Is(err, errDefaultDeviceChanged) {
			backoff = retryDelayFirst
			continue
		}
		s.reportError(err)
		if !s.wait(backoff) {
			return
		}
		backoff = min(backoff*2, retryDelayMax)
	}
}

// stream is one open tap capture. The IOProc lives on the aggregate device,
// whose input is the tap; outputID records the real output device the tap
// follows, for the default-device watch and the stats name.
type stream struct {
	src      *Source
	aggID    uint32
	outputID uint32
	name     string
	rate     int
	conv     converter
	tapID    uint32
	procID   darwin.DeviceIOProcID
	scratch  []int16
}

// openStream taps the chosen output device (or the default one when id is
// empty) and starts its aggregate.
func openStream(s *Source) (strm *stream, err error) {
	if err := darwin.EnsureCoreAudio(); err != nil {
		return nil, err
	}
	var dev darwin.DeviceInfo
	if id := s.cfg.EndpointID; id == "" {
		var uid, name string
		if dev.ID, uid, name, err = darwin.DefaultOutputDevice(); err != nil {
			return nil, err
		}
		dev = darwin.DeviceInfo{ID: dev.ID, UID: uid, Name: name}
	} else {
		var d uint32
		if d, err = darwin.DeviceForUID(id); err != nil {
			return nil, err
		}
		var name string
		if name, err = darwin.GetStringProperty(d, darwin.PropObjectName, darwin.ScopeGlobal); err != nil {
			return nil, err
		}
		dev = darwin.DeviceInfo{ID: d, UID: id, Name: name}
	}
	if dev.Name == "" {
		dev.Name = dev.UID
	}

	strm = &stream{src: s}
	defer func() {
		// return nil, err clears the named strm before this defer runs.
		if err != nil && strm != nil {
			strm.close()
		}
	}()

	tapDesc, terr := newGlobalTapDescription()
	if terr != nil {
		return nil, terr
	}
	// The description is an owned Objective-C object; the tap copies what it
	// needs during the create call.
	defer darwin.Send(tapDesc, "release")
	if strm.tapID, err = darwin.CreateProcessTap(tapDesc); err != nil {
		return nil, err
	}
	tapUID, terr := darwin.GetStringProperty(strm.tapID, darwin.PropTapUID, darwin.ScopeGlobal)
	if terr != nil {
		return nil, terr
	}
	var asbd darwin.AudioStreamBasicDescription
	asbdSize := uint32(unsafe.Sizeof(asbd))
	if err = darwin.GetProperty(strm.tapID, darwin.PropertyAddress(darwin.PropTapFormat, darwin.ScopeGlobal), unsafe.Pointer(&asbd), &asbdSize); err != nil {
		return nil, err
	}
	if strm.conv, strm.rate, err = converterForTapFormat(asbd); err != nil {
		return nil, err
	}

	if strm.aggID, err = createTapAggregate(dev.UID, tapUID); err != nil {
		return nil, err
	}
	strm.outputID = dev.ID
	if strm.procID, err = darwin.CreateDeviceIOProc(strm.aggID, ioProcPtr()); err != nil {
		return nil, err
	}
	ioRegistryMu.Lock()
	ioRegistry[strm.aggID] = strm
	ioRegistryMu.Unlock()
	if err = darwin.StartDevice(strm.aggID, strm.procID); err != nil {
		return nil, err
	}
	strm.name = dev.Name
	return strm, nil
}

// close stops the stream and tears the aggregate, IOProc and tap down.
func (st *stream) close() {
	ioRegistryMu.Lock()
	delete(ioRegistry, st.aggID)
	ioRegistryMu.Unlock()
	if st.procID != nil {
		darwin.StopDevice(st.aggID, st.procID)
		darwin.DestroyDeviceIOProc(st.aggID, st.procID)
		st.procID = nil
	}
	if st.aggID != 0 {
		darwin.DestroyAggregateDevice(st.aggID)
		st.aggID = 0
	}
	if st.tapID != 0 {
		darwin.DestroyProcessTap(st.tapID)
		st.tapID = 0
	}
}

// pump supervises the running IOProc until stopped. The IOProc pushes on the
// audio thread; the pump only watches the followed default device, the way
// the Windows pump watches its loopback event.
func (st *stream) pump(s *Source, follow bool) error {
	nextCheck := time.Now().Add(openCheckEvery)
	for {
		select {
		case <-s.stop:
			return errStopped
		default:
		}
		time.Sleep(100 * time.Millisecond)
		select {
		case <-s.stop:
			return errStopped
		default:
		}
		if !time.Now().Before(nextCheck) {
			nextCheck = time.Now().Add(openCheckEvery)
			if follow {
				if id, _, _, err := darwin.DefaultOutputDevice(); err == nil && id != st.outputID {
					return errDefaultDeviceChanged
				}
			}
			// A stopped aggregate means the device went away. A silent one
			// keeps running, so quiet audio does not trigger this.
			if running, err := darwin.DeviceRunningSomewhere(st.aggID); err == nil && !running {
				return errors.New("capture: the aggregate device stopped")
			}
		}
	}
}

// push converts one IO buffer to stereo int16 and drops it into the ring,
// which discards its oldest samples when full, so capture never blocks. It
// runs on the audio thread and must stay allocation-free in steady state.
func (st *stream) push(input *darwin.AudioBufferList) {
	for i := 0; i < int(input.NumberBuffers); i++ {
		buf := input.Buffer(i)
		if buf.Data == nil || buf.DataByteSize == 0 {
			continue
		}
		src := unsafe.Slice((*byte)(buf.Data), int(buf.DataByteSize))
		frames := len(src) / st.conv.blockAlign
		n := frames * 2
		if n == 0 {
			continue
		}
		if cap(st.scratch) < n {
			st.scratch = make([]int16, n+2048) // headroom for buffers of varying size
		}
		dst := st.scratch[:n]
		st.conv.convert(dst, src)
		st.src.cfg.Ring.Push(dst)
		st.src.addFrames(uint32(frames))
	}
}

// newGlobalTapDescription builds the CATapDescription of a stereo global tap
// that excludes no process and plays on locally (unmuted).
func newGlobalTapDescription() (desc darwin.ID, err error) {
	darwin.WithPool(func() {
		cls, ok := darwin.Class("CATapDescription")
		if !ok {
			err = errors.New("capture: CATapDescription is unavailable on this system")
			return
		}
		desc = darwin.Send(darwin.Send(cls, "alloc"), "initStereoGlobalTapButExcludeProcesses:", uintptr(darwin.NSArray()))
		if desc == 0 {
			err = errors.New("capture: CATapDescription init failed")
			return
		}
		// CATapUnmuted: the tap captures while the speakers keep playing.
		darwin.Send(desc, "setMuteBehavior:", 0)
		uuid := darwin.Alloc("NSUUID")
		if uuid == 0 {
			err = errors.New("capture: NSUUID is unavailable on this system")
			return
		}
		darwin.Send(desc, "setUUID:", uintptr(uuid))
		darwin.Send(uuid, "release") // the description retains its copy
	})
	return desc, err
}

// createTapAggregate builds the private aggregate device that carries the
// tap: the output device is the sub-device and the clock master, and the tap
// rides in the tap list with drift compensation and auto start.
func createTapAggregate(outputUID, tapUID string) (aggID uint32, err error) {
	darwin.WithPool(func() {
		d := darwin.Send(darwin.MustClass("NSMutableDictionary"), "dictionary")
		sub := darwin.Send(darwin.MustClass("NSMutableDictionary"), "dictionary")
		tap := darwin.Send(darwin.MustClass("NSMutableDictionary"), "dictionary")
		if d == 0 || sub == 0 || tap == 0 {
			err = errors.New("capture: NSMutableDictionary unavailable")
			return
		}
		set := func(key string, v darwin.ID) {
			darwin.Send(d, "setObject:forKey:", uintptr(v), uintptr(darwin.NSString(key)))
		}
		darwin.Send(sub, "setObject:forKey:", uintptr(darwin.NSString(outputUID)), uintptr(darwin.NSString(darwin.AggKeyUID)))
		darwin.Send(tap, "setObject:forKey:", uintptr(darwin.NumberBool(true)), uintptr(darwin.NSString(darwin.AggKeyTapDrift)))
		darwin.Send(tap, "setObject:forKey:", uintptr(darwin.NSString(tapUID)), uintptr(darwin.NSString(darwin.AggKeyUID)))
		set(darwin.AggKeyName, darwin.NSString("saundo capture"))
		set(darwin.AggKeyUID, darwin.NSString("saundo.capture."+tapUID))
		set(darwin.AggKeyPrivate, darwin.NumberBool(true))
		set(darwin.AggKeyStacked, darwin.NumberBool(false))
		set(darwin.AggKeyTapAutoStart, darwin.NumberBool(true))
		set(darwin.AggKeyMaster, darwin.NSString(outputUID))
		set(darwin.AggKeySubDevices, darwin.NSArray(sub))
		set(darwin.AggKeyTaps, darwin.NSArray(tap))
		aggID, err = darwin.CreateAggregateDevice(d)
	})
	return aggID, err
}

// converterForTapFormat validates the tap's mix format and builds the
// converter and rate for it.
func converterForTapFormat(asbd darwin.AudioStreamBasicDescription) (converter, int, error) {
	if asbd.FormatID != darwin.FourCC("lpcm") {
		return converter{}, 0, fmt.Errorf("capture: unsupported tap format %#08x", asbd.FormatID)
	}
	if asbd.FormatFlags&darwin.FormatFlagNonInterleaved != 0 || asbd.FormatFlags&darwin.FormatFlagBigEndian != 0 {
		return converter{}, 0, errors.New("capture: unsupported tap format layout (non-interleaved or big endian)")
	}
	tag := uint16(1) // PCM
	if asbd.FormatFlags&darwin.FormatFlagFloat != 0 {
		tag = 3 // float
	} else if asbd.FormatFlags&darwin.FormatFlagSignedInteger == 0 {
		return converter{}, 0, errors.New("capture: unsupported tap sample encoding")
	}
	conv, err := newConverter(tag, int(asbd.BitsPerChannel), int(asbd.ChannelsPerFrame), int(asbd.BytesPerFrame))
	if err != nil {
		return converter{}, 0, err
	}
	return conv, int(asbd.SampleRate), nil
}

// AppInfo describes one capturable application. It mirrors the Windows
// definition so callers compile everywhere.
type AppInfo struct {
	PID   uint32
	Name  string
	Path  string
	Title string
	// Active reports whether the app (its process tree) currently renders
	// audio; process-loopback capture only initializes for active apps.
	Active bool
}

// AppCaptureSupported reports whether per-application capture enumeration is
// available on this platform. It is false on darwin, so callers skip the
// per-app picker instead of surfacing an empty list.
func AppCaptureSupported() bool { return false }

// AudioApplications is unsupported on darwin.
func AudioApplications() ([]AppInfo, error) {
	return nil, errors.New("capture: unsupported on this platform")
}

// Endpoints lists the output devices as endpoints for the settings picker.
func Endpoints() ([]Endpoint, error) {
	devs, err := darwin.OutputDevices()
	if err != nil {
		return nil, err
	}
	out := make([]Endpoint, 0, len(devs))
	for _, d := range devs {
		if !darwin.DeviceHasOutput(d.ID) {
			continue
		}
		out = append(out, Endpoint{ID: d.UID, Name: d.Name})
	}
	return out, nil
}

// DefaultEndpoint returns the default output device.
func DefaultEndpoint() (Endpoint, error) {
	_, uid, name, err := darwin.DefaultOutputDevice()
	if err != nil {
		return Endpoint{}, err
	}
	return Endpoint{ID: uid, Name: name}, nil
}
