//go:build darwin

package capture

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
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

// errMuteChanged asks the capture loop to rebuild a per-app tap with a new
// local-output behavior; the tap fixes that behavior at creation.
var errMuteChanged = errors.New("capture: process mute changed")

// ioProcEntry is the single C function pointer the package registers; purego
// callback slots are finite, so streams register themselves in ioRegistry
// instead of each allocating a callback.
var ioProcPtr = sync.OnceValue(func() uintptr {
	return purego.NewCallback(ioProcEntry)
})

// activeIO is the stream the IOProc serves. It is one atomic pointer rather
// than a locked map because the IOProc runs on Core Audio's realtime thread.
var activeIO atomic.Pointer[stream]

func ioProcEntry(device uintptr, now *darwin.AudioTimeStamp, input *darwin.AudioBufferList,
	inTime *darwin.AudioTimeStamp, output unsafe.Pointer, outTime *darwin.AudioTimeStamp) int32 {
	if st := activeIO.Load(); st != nil && st.aggID == uint32(device) {
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
	s := &Source{cfg: cfg, stop: make(chan struct{}), done: make(chan struct{})}
	s.muteReq.Store(cfg.MuteProcess)
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
		s.streamOpened(st.rate, st.name, st.appIDs)
		if wasFirst {
			ready <- nil
		}

		err = st.pump(s, s.cfg.ProcessID != 0 || len(s.cfg.AppIDs) > 0 || s.cfg.EndpointID == "")
		st.close()
		s.streamClosed()
		if err == nil || errors.Is(err, errStopped) {
			return
		}
		if errors.Is(err, errMuteChanged) || errors.Is(err, errDefaultDeviceChanged) {
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
	perApp   bool
	muted    bool
	appIDs   []string
	procID   darwin.DeviceIOProcID
	// scratch is allocated at open and only written by the audio thread. push
	// never grows it; an oversized buffer is dropped and counted.
	scratch []int16
}

// openStream taps the chosen output device (or the default one when id is
// empty) and starts its aggregate. Per-app capture ignores EndpointID: the
// tap follows the process, and the default output is only the clock master.
func openStream(s *Source) (strm *stream, err error) {
	if err := darwin.EnsureCoreAudio(); err != nil {
		return nil, err
	}
	perApp := s.cfg.ProcessID != 0 || len(s.cfg.AppIDs) > 0
	var dev darwin.DeviceInfo
	if id := s.cfg.EndpointID; id == "" || perApp {
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

	strm = &stream{src: s, perApp: perApp}
	defer func() {
		// return nil, err clears the named strm before this defer runs.
		if err != nil && strm != nil {
			strm.close()
		}
	}()

	var tapDesc darwin.ID
	var appName string
	var appIDs []string
	var terr error
	if perApp {
		var family []uint32
		if len(s.cfg.AppIDs) > 0 {
			appName, appIDs, family, err = appsTap(s.cfg.AppIDs)
		} else {
			appName, family, err = appTap(s.cfg.ProcessID)
		}
		if err != nil {
			return nil, err
		}
		strm.appIDs = appIDs
		strm.muted = s.muteReq.Load()
		tapDesc, terr = newMixdownTapDescription(family, strm.muted)
	} else {
		tapDesc, terr = newGlobalTapDescription()
	}
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
	strm.scratch = makeScratch(strm.rate)

	if strm.aggID, err = createTapAggregate(dev.UID, tapUID); err != nil {
		return nil, err
	}
	strm.outputID = dev.ID
	if strm.procID, err = darwin.CreateDeviceIOProc(strm.aggID, ioProcPtr()); err != nil {
		return nil, err
	}
	activeIO.Store(strm)
	if err = darwin.StartDevice(strm.aggID, strm.procID); err != nil {
		return nil, err
	}
	if perApp {
		strm.name = appName
	} else {
		strm.name = dev.Name
	}
	return strm, nil
}

// close stops the stream and tears the aggregate, IOProc and tap down.
func (st *stream) close() {
	if st.procID != nil {
		darwin.StopDevice(st.aggID, st.procID)
		activeIO.CompareAndSwap(st, nil)
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
		if st.perApp && s.muteReq.Load() != st.muted {
			return errMuteChanged
		}
		if follow && !time.Now().Before(nextCheck) {
			nextCheck = time.Now().Add(openCheckEvery)
			if id, _, _, err := darwin.DefaultOutputDevice(); err == nil && id != st.outputID {
				return errDefaultDeviceChanged
			}
		}
	}
}

// push converts one IO buffer to stereo int16 and drops it into the ring,
// which discards its oldest samples when full, so capture never blocks. It
// runs on the audio thread and does not lock or allocate.
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
		if n > len(st.scratch) {
			st.src.addDropped(uint64(n))
			continue
		}
		dst := st.scratch[:n]
		st.conv.convert(dst, src)
		st.src.cfg.Ring.Push(dst)
		st.src.addFrames(uint32(frames))
	}
}

// makeScratch returns the conversion buffer: half a second of stereo samples,
// with a floor for low or unknown rates. A Core Audio IO buffer is much
// smaller; an oversized one is dropped instead of allocated on the audio
// thread.
func makeScratch(rate int) []int16 {
	n := rate
	if n < 4096 {
		n = 4096
	}
	return make([]int16, n)
}

// newGlobalTapDescription builds the CATapDescription of a stereo global tap
// that excludes no process and plays on locally (unmuted).
func newGlobalTapDescription() (desc darwin.ID, err error) {
	return newTapDescription("initStereoGlobalTapButExcludeProcesses:", nil, false)
}

// newMixdownTapDescription builds the CATapDescription of a stereo mixdown of
// the given HAL process objects. muted captures the mix without sending it to
// the local output.
func newMixdownTapDescription(objects []uint32, muted bool) (desc darwin.ID, err error) {
	return newTapDescription("initStereoMixdownOfProcesses:", objects, muted)
}

// newTapDescription runs one of the CATapDescription initializers and applies
// the settings both tap shapes need. A nil objects list means a global tap.
func newTapDescription(initSelector string, objects []uint32, muted bool) (desc darwin.ID, err error) {
	darwin.WithPool(func() {
		cls, ok := darwin.Class("CATapDescription")
		if !ok {
			err = errors.New("capture: CATapDescription is unavailable on this system")
			return
		}
		nums := make([]darwin.ID, len(objects))
		for i, id := range objects {
			nums[i] = darwin.NumberUint(id)
		}
		desc = darwin.Send(darwin.Send(cls, "alloc"), initSelector, uintptr(darwin.NSArray(nums...)))
		if desc == 0 {
			err = errors.New("capture: CATapDescription init failed")
			return
		}
		// CATapUnmuted, or CATapMuted for a per-app tap that owns the app's
		// local output for the session.
		v := uintptr(0)
		if muted {
			v = 1
		}
		darwin.Send(desc, "setMuteBehavior:", v)
		uuid := darwin.Alloc("NSUUID")
		if uuid == 0 {
			darwin.Send(desc, "release")
			desc = 0
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
	// ID is the stable selection key: the bundle ID, or pid:<n> for a
	// process without one. A regular app's ID survives a relaunch.
	ID string
	// Icon is the app's PNG icon, or nil when launching it produced none.
	Icon []byte
	// Active reports whether the app currently renders audio. A tap can only
	// include processes the HAL already knows, but a silent app taps silence
	// once it is connected.
	Active bool
}

// runningApp and processObject are the capture package's view of the
// LaunchServices and HAL lists; AudioApplications converts at the boundary.
type runningApp struct {
	PID      uint32
	Name     string
	BundleID string
	Path     string
	Policy   int
}

type processObject struct {
	ID            uint32
	PID           uint32
	BundleID      string
	RunningOutput bool
}

// appPolicyRegular is NSApplicationActivationPolicyRegular: an app with a
// Dock presence, the macOS counterpart of a Windows visible-window app.
const appPolicyRegular = 0

// MultiAppSupported reports whether one tap can mix several selected apps.
// Core Audio's process mixdown can.
func MultiAppSupported() bool { return true }

// ProcessMuteSupported reports whether a per-app tap can mute just the
// captured app. Core Audio's tap description can.
func ProcessMuteSupported() bool { return true }

// SetProcessMuted mutes or unmutes the app of the running per-app tap.
func SetProcessMuted(muted bool) error {
	st := activeIO.Load()
	if st == nil || !st.perApp {
		if !muted {
			return nil // no tap means the app is already audible
		}
		return errors.New("capture: no per-app stream")
	}
	// The tap fixes its mute behavior at creation, so the capture loop
	// rebuilds it. The pump notices within one tick.
	st.src.muteReq.Store(muted)
	return nil
}

// AppCaptureSupported reports whether the HAL exposes client processes and
// CATapDescription can mix them. Both exist on macOS 14.4, the app's minimum;
// the check keeps a bare binary on an older system failing closed.
func AppCaptureSupported() bool {
	if err := darwin.EnsureCoreAudio(); err != nil {
		return false
	}
	if _, ok := darwin.Class("CATapDescription"); !ok {
		return false
	}
	_, err := darwin.ProcessObjects()
	return err == nil
}

// AudioApplications lists the apps the capture picker offers: every regular
// app, whether or not it has played audio yet, plus any background process
// currently rendering output. Audio helpers are folded into the app whose
// bundle ID prefixes theirs.
func AudioApplications() ([]AppInfo, error) {
	apps, err := runningApps()
	if err != nil {
		return nil, err
	}
	procs, err := processObjects()
	if err != nil {
		return nil, err
	}
	list := buildAppList(apps, procs, uint32(os.Getpid()))
	byPID := make(map[uint32]runningApp, len(apps))
	for _, a := range apps {
		byPID[a.PID] = a
	}
	for i := range list {
		a := byPID[list[i].PID]
		if a.BundleID != "" {
			list[i].Icon = darwin.AppIconForBundleID(a.BundleID)
		}
		if list[i].Icon == nil && a.Path != "" {
			list[i].Icon = darwin.AppIconForPath(a.Path)
		}
	}
	return list, nil
}

func runningApps() ([]runningApp, error) {
	apps, err := darwin.RunningApplications()
	if err != nil {
		return nil, err
	}
	out := make([]runningApp, 0, len(apps))
	for _, a := range apps {
		out = append(out, runningApp{PID: a.PID, Name: a.Name, BundleID: a.BundleID, Path: a.Path, Policy: a.Policy})
	}
	return out, nil
}

func processObjects() ([]processObject, error) {
	procs, err := darwin.ProcessObjects()
	if err != nil {
		return nil, err
	}
	out := make([]processObject, 0, len(procs))
	for _, p := range procs {
		out = append(out, processObject{ID: p.ID, PID: p.PID, BundleID: p.BundleID, RunningOutput: p.RunningOutput})
	}
	return out, nil
}

// appTap resolves the HAL objects a tap for one app includes, and the name
// the capture status shows.
func appTap(pid uint32) (name string, objects []uint32, err error) {
	apps, err := runningApps()
	if err != nil {
		return "", nil, err
	}
	procs, err := processObjects()
	if err != nil {
		return "", nil, err
	}
	var root runningApp
	for _, a := range apps {
		if a.PID == pid {
			root = a
			break
		}
	}
	objects = appFamily(procs, pid, root)
	if len(objects) == 0 {
		return "", nil, fmt.Errorf("%w: pid %d; start audio in the app and try again", ErrNoAppProcess, pid)
	}
	name = root.Name
	if name == "" {
		name = fmt.Sprintf("PID %d", pid)
	}
	return name, objects, nil
}

// appsTap resolves stable app IDs to the HAL objects a tap includes, the IDs
// it actually captured, and the name the capture status shows.
func appsTap(ids []string) (name string, resolved []string, objects []uint32, err error) {
	apps, err := runningApps()
	if err != nil {
		return "", nil, nil, err
	}
	procs, err := processObjects()
	if err != nil {
		return "", nil, nil, err
	}
	names, resolved, objects := appsTapIDs(apps, procs, ids)
	if len(objects) == 0 {
		return "", nil, nil, fmt.Errorf("%w: %s; start the app and try again", ErrNoAppProcess, strings.Join(ids, ", "))
	}
	return strings.Join(names, " + "), resolved, objects, nil
}

// appsTapIDs resolves IDs to the union of their process objects. A bundle ID
// selects the app's family; pid:<n> selects one process. An ID whose app is
// not running is skipped, so one closed app does not stop the others.
func appsTapIDs(apps []runningApp, procs []processObject, ids []string) (names, resolved []string, objects []uint32) {
	seen := make(map[uint32]bool)
	add := func(id uint32) {
		if !seen[id] {
			seen[id] = true
			objects = append(objects, id)
		}
	}
	for _, id := range ids {
		if pid, ok := parsePIDID(id); ok {
			name := ""
			for _, a := range apps {
				if a.PID == pid {
					name = a.Name
					break
				}
			}
			found := false
			for _, p := range procs {
				if p.PID == pid {
					add(p.ID)
					found = true
				}
			}
			if !found {
				continue
			}
			if name == "" {
				name = fmt.Sprintf("PID %d", pid)
			}
			names = append(names, name)
			resolved = append(resolved, id)
			continue
		}
		var root runningApp
		found := false
		for _, a := range apps {
			if a.Policy == appPolicyRegular && a.BundleID == id {
				root, found = a, true
				break
			}
		}
		if !found {
			continue
		}
		family := appFamily(procs, root.PID, root)
		if len(family) == 0 {
			continue
		}
		for _, obj := range family {
			add(obj)
		}
		name := root.Name
		if name == "" {
			name = id
		}
		names = append(names, name)
		resolved = append(resolved, id)
	}
	return names, resolved, objects
}

func parsePIDID(id string) (uint32, bool) {
	if !strings.HasPrefix(id, "pid:") {
		return 0, false
	}
	v, err := strconv.ParseUint(strings.TrimPrefix(id, "pid:"), 10, 32)
	if err != nil {
		return 0, false
	}
	return uint32(v), true
}

// buildAppList groups HAL processes under running apps. Every regular app is
// offered even before it connects to Core Audio, so the picker does not
// depend on playback order. A background process with output and no owner is
// its own entry, the way WebKit's Graphics and Media helper appears.
func buildAppList(apps []runningApp, procs []processObject, selfPID uint32) []AppInfo {
	byPID := make(map[uint32]runningApp, len(apps))
	regular := make(map[string]runningApp)
	for _, a := range apps {
		if a.PID == selfPID {
			continue
		}
		byPID[a.PID] = a
		if a.Policy == appPolicyRegular && a.BundleID != "" {
			if _, ok := regular[a.BundleID]; !ok {
				regular[a.BundleID] = a
			}
		}
	}

	// Two live processes of one app, as a browser relaunch leaves behind,
	// are one picker row: a regular app's bundle ID names its family.
	canonical := make(map[string]uint32)
	canonicalRoot := func(a runningApp) runningApp {
		if a.Policy != appPolicyRegular || a.BundleID == "" {
			return a
		}
		if pid, ok := canonical[a.BundleID]; ok {
			if root, ok := byPID[pid]; ok {
				return root
			}
		}
		canonical[a.BundleID] = a.PID
		return a
	}

	type family struct {
		root  runningApp
		procs []processObject
	}
	families := make(map[uint32]*family)
	familyFor := func(root runningApp) *family {
		f := families[root.PID]
		if f == nil {
			f = &family{root: root}
			families[root.PID] = f
		}
		return f
	}
	for _, a := range apps {
		if a.PID != selfPID && a.Policy == appPolicyRegular {
			familyFor(canonicalRoot(a))
		}
	}

	attributed := make(map[uint32]bool)
	for _, p := range procs {
		if p.PID == selfPID {
			continue
		}
		if a, ok := byPID[p.PID]; ok && a.Policy == appPolicyRegular {
			f := familyFor(canonicalRoot(a))
			f.procs = append(f.procs, p)
			attributed[p.PID] = true
			continue
		}
		if a, ok := ownerApp(p.BundleID, regular); ok {
			f := familyFor(canonicalRoot(a))
			f.procs = append(f.procs, p)
			attributed[p.PID] = true
		}
	}

	out := make([]AppInfo, 0, len(families)+len(procs))
	for _, f := range families {
		active := false
		for _, p := range f.procs {
			if p.RunningOutput {
				active = true
				break
			}
		}
		id := f.root.BundleID
		if id == "" {
			id = fmt.Sprintf("pid:%d", f.root.PID)
		}
		out = append(out, AppInfo{PID: f.root.PID, Name: f.root.Name, Path: f.root.Path, ID: id, Active: active})
	}
	for _, p := range procs {
		if p.PID == selfPID || attributed[p.PID] || !p.RunningOutput {
			continue
		}
		a := byPID[p.PID]
		name := a.Name
		if name == "" {
			name = p.BundleID
		}
		if name == "" {
			name = fmt.Sprintf("PID %d", p.PID)
		}
		out = append(out, AppInfo{PID: p.PID, Name: name, Path: a.Path, ID: fmt.Sprintf("pid:%d", p.PID), Active: true})
	}

	sort.Slice(out, func(i, j int) bool {
		ni, nj := strings.ToLower(out[i].Name), strings.ToLower(out[j].Name)
		if ni == nj {
			return out[i].PID < out[j].PID
		}
		return ni < nj
	})
	return out
}

// ownerApp finds the regular app a helper belongs to by walking the helper's
// bundle ID up its dot-separated prefixes: com.google.Chrome.helper belongs
// to com.google.Chrome.
func ownerApp(bundleID string, regular map[string]runningApp) (runningApp, bool) {
	for b := bundleID; b != ""; {
		if a, ok := regular[b]; ok {
			return a, true
		}
		i := strings.LastIndexByte(b, '.')
		if i < 0 {
			break
		}
		b = b[:i]
	}
	return runningApp{}, false
}

// appFamily returns the HAL objects a tap for pid includes. A regular app
// owns the helpers whose bundle ID extends its own; a background process
// picked on its own is tapped alone. A pid LaunchServices does not know,
// such as a command-line player, still matches its own object.
func appFamily(procs []processObject, pid uint32, root runningApp) []uint32 {
	var ids []uint32
	for _, p := range procs {
		if p.PID == pid {
			ids = append(ids, p.ID)
			continue
		}
		if root.Policy != appPolicyRegular || root.BundleID == "" {
			continue
		}
		if p.BundleID == root.BundleID || strings.HasPrefix(p.BundleID, root.BundleID+".") {
			ids = append(ids, p.ID)
		}
	}
	return ids
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
