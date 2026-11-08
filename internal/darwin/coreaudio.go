//go:build darwin

package darwin

import (
	"errors"
	"fmt"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
)

// Core Audio's C API, the subset the capture and mute implementations need.
// Property selectors and scopes appear as their four-character codes.

// AudioObjectPropertyAddress mirrors the C struct of the same name.
type AudioObjectPropertyAddress struct {
	Selector uint32
	Scope    uint32
	Element  uint32
}

// AudioBuffer mirrors the C struct: two uint32 followed by a pointer.
type AudioBuffer struct {
	NumberChannels uint32
	DataByteSize   uint32
	Data           unsafe.Pointer
}

// audioBufferStride is the ABI stride of AudioBuffer on arm64 and amd64.
const audioBufferStride = 16

// AudioBufferList mirrors the C struct. Buffers is declared with one element
// because Go cannot declare the variable-length C array; Buffer reads any
// element the system allocated past the first.
type AudioBufferList struct {
	NumberBuffers uint32
	_             uint32
	Buffers       [1]AudioBuffer
}

// Buffer returns buffer i of a list the system handed out.
func (l *AudioBufferList) Buffer(i int) AudioBuffer {
	return *(*AudioBuffer)(unsafe.Add(unsafe.Pointer(l), 8+i*audioBufferStride))
}

// AudioTimeStamp mirrors the C struct.
type AudioTimeStamp struct {
	SampleTime float64
	HostTime   uint64
	Flags      uint32
	_          uint32
	Reserved   [4]uint64
}

// AudioStreamBasicDescription mirrors the C struct.
type AudioStreamBasicDescription struct {
	SampleRate       float64
	FormatID         uint32
	FormatFlags      uint32
	BytesPerPacket   uint32
	FramesPerPacket  uint32
	BytesPerFrame    uint32
	ChannelsPerFrame uint32
	BitsPerChannel   uint32
}

// PCM format flags of an AudioStreamBasicDescription.
const (
	FormatFlagFloat          uint32 = 0x1
	FormatFlagBigEndian      uint32 = 0x2
	FormatFlagSignedInteger  uint32 = 0x4
	FormatFlagPacked         uint32 = 0x8
	FormatFlagNonInterleaved uint32 = 0x10
)

// FourCC packs a four-character code the way Core Audio defines selectors.
func FourCC(s string) uint32 {
	var v uint32
	for _, b := range []byte(s) {
		v = v<<8 | uint32(b)
	}
	return v
}

// Audio object and property constants from AudioHardware.h.
const (
	AudioObjectSystemObject      uint32 = 1
	ElementMain                  uint32 = 0
	ScopeGlobal                  uint32 = 0x676c6f62 // 'glob'
	ScopeOutput                  uint32 = 0x6f757420 // 'out '
	PropDefaultOutputDevice      uint32 = 0x644f7574 // 'dOut'
	PropDevices                  uint32 = 0x64657623 // 'dev#'
	PropObjectName               uint32 = 0x6c6e616d // 'lnam'
	PropDeviceUID                uint32 = 0x75696420 // 'uid '
	PropDeviceIsRunningSomewhere uint32 = 0x676f6e65 // 'gone'
	PropDeviceMute               uint32 = 0x6d757465 // 'mute'
	PropTranslateUIDToDevice     uint32 = 0x75696464 // 'uidd'
	PropTapUID                   uint32 = 0x74756964 // 'tuid'
	PropTapFormat                uint32 = 0x74666d74 // 'tfmt'
	PropStreams                  uint32 = 0x73747223 // 'str#'
)

// EnsureCoreAudio loads the Core Audio framework and its symbols. Callers
// that look up its Objective-C classes must run it first.
func EnsureCoreAudio() error {
	_, err := coreAudio()
	return err
}

// DeviceHasOutput reports whether a device has output streams, so endpoint
// pickers skip microphones and other input-only devices.
func DeviceHasOutput(device uint32) bool {
	var size uint32
	if err := GetPropertySize(device, PropertyAddress(PropStreams, ScopeOutput), &size); err != nil {
		return false
	}
	return size > 0
}

// Aggregate device dictionary keys from AudioHardware.h.
const (
	AggKeyName         = "name"
	AggKeyUID          = "uid"
	AggKeyPrivate      = "private"
	AggKeyStacked      = "stacked"
	AggKeyTapAutoStart = "tapautostart"
	AggKeyMaster       = "master"
	AggKeySubDevices   = "subdevices"
	AggKeyTaps         = "taps"
	AggKeyTapDrift     = "drift"
)

type coreAudioAPI struct {
	getProp     func(id uint32, addr *AudioObjectPropertyAddress, qualSize uint32, qualifier unsafe.Pointer, size *uint32, data unsafe.Pointer) int32
	getPropSize func(id uint32, addr *AudioObjectPropertyAddress, qualSize uint32, qualifier unsafe.Pointer, size *uint32) int32
	setProp     func(id uint32, addr *AudioObjectPropertyAddress, qualSize uint32, qualifier unsafe.Pointer, size uint32, data unsafe.Pointer) int32
	destroyTap  func(tapID uint32) int32
	destroyAgg  func(deviceID uint32) int32
	// createTap and createAgg take Objective-C object references, so they
	// go through SyscallN with the object id as an integer register instead
	// of a typed pointer parameter.
	createTapAddr uintptr
	createAggAddr uintptr
	createIOProc  func(deviceID uint32, proc uintptr, clientData unsafe.Pointer, outProcID *unsafe.Pointer) int32
	destroyIOProc func(deviceID uint32, procID unsafe.Pointer) int32
	start         func(deviceID uint32, procID unsafe.Pointer) int32
	stop          func(deviceID uint32, procID unsafe.Pointer) int32
}

var coreAudio = sync.OnceValues(func() (*coreAudioAPI, error) {
	if err := load(); err != nil {
		return nil, err
	}
	lib, err := mustDlopen("/System/Library/Frameworks/CoreAudio.framework/CoreAudio")
	if err != nil {
		return nil, fmt.Errorf("coreaudio: %w", err)
	}
	k := &coreAudioAPI{}
	for _, e := range []struct {
		fn any
		nm string
	}{
		{&k.getProp, "AudioObjectGetPropertyData"},
		{&k.getPropSize, "AudioObjectGetPropertyDataSize"},
		{&k.setProp, "AudioObjectSetPropertyData"},
		{&k.destroyTap, "AudioHardwareDestroyProcessTap"},
		{&k.destroyAgg, "AudioHardwareDestroyAggregateDevice"},
		{&k.createIOProc, "AudioDeviceCreateIOProcID"},
		{&k.destroyIOProc, "AudioDeviceDestroyIOProcID"},
		{&k.start, "AudioDeviceStart"},
		{&k.stop, "AudioDeviceStop"},
	} {
		// A missing symbol panics: the framework exists but lost an entry
		// point, which is a programming error, not a runtime condition.
		purego.RegisterLibFunc(e.fn, lib, e.nm)
	}
	if k.createTapAddr, err = purego.Dlsym(lib, "AudioHardwareCreateProcessTap"); err != nil {
		return nil, fmt.Errorf("coreaudio: %w", err)
	}
	if k.createAggAddr, err = purego.Dlsym(lib, "AudioHardwareCreateAggregateDevice"); err != nil {
		return nil, fmt.Errorf("coreaudio: %w", err)
	}
	return k, nil
})

// StatusError renders an OSStatus with its four-character code when the code
// is printable, the shape Core Audio errors arrive in.
func StatusError(op string, status int32) error {
	c := uint32(status)
	tag := ""
	b := []byte{byte(c >> 24), byte(c >> 16), byte(c >> 8), byte(c)}
	readable := true
	for _, x := range b {
		if x < 0x20 || x >= 0x7f {
			readable = false
			break
		}
	}
	if readable {
		tag = fmt.Sprintf(" '%s'", b)
	}
	return fmt.Errorf("coreaudio: %s failed (OSStatus %d%s)", op, status, tag)
}

// PropertyAddress builds one AudioObjectPropertyAddress.
func PropertyAddress(selector uint32, scope uint32) AudioObjectPropertyAddress {
	return AudioObjectPropertyAddress{Selector: selector, Scope: scope, Element: ElementMain}
}

// GetProperty reads a fixed-size property of an audio object.
func GetProperty(id uint32, addr AudioObjectPropertyAddress, data unsafe.Pointer, size *uint32) error {
	k, err := coreAudio()
	if err != nil {
		return err
	}
	if st := k.getProp(id, &addr, 0, nil, size, data); st != 0 {
		return StatusError("AudioObjectGetPropertyData", st)
	}
	return nil
}

// GetPropertySize reports the byte size of a property.
func GetPropertySize(id uint32, addr AudioObjectPropertyAddress, size *uint32) error {
	k, err := coreAudio()
	if err != nil {
		return err
	}
	if st := k.getPropSize(id, &addr, 0, nil, size); st != 0 {
		return StatusError("AudioObjectGetPropertyDataSize", st)
	}
	return nil
}

// SetProperty writes a fixed-size property of an audio object.
func SetProperty(id uint32, addr AudioObjectPropertyAddress, data unsafe.Pointer, size uint32) error {
	k, err := coreAudio()
	if err != nil {
		return err
	}
	if st := k.setProp(id, &addr, 0, nil, size, data); st != 0 {
		return StatusError("AudioObjectSetPropertyData", st)
	}
	return nil
}

// GetStringProperty reads a CF-string property. The HAL's ownership of the
// returned reference is unspecified, so the helper retains before releasing
// and the pair balances either rule.
func GetStringProperty(id uint32, selector uint32, scope uint32) (string, error) {
	var str unsafe.Pointer
	size := uint32(unsafe.Sizeof(str))
	if err := GetProperty(id, PropertyAddress(selector, scope), unsafe.Pointer(&str), &size); err != nil {
		return "", err
	}
	if str == nil {
		return "", nil
	}
	obj := ID(str)
	Send(obj, "retain")
	defer CFRelease(uintptr(obj))
	return GoString(obj), nil
}

// DefaultOutputDevice returns the id, UID and name of the default output.
func DefaultOutputDevice() (id uint32, uid, name string, err error) {
	addr := PropertyAddress(PropDefaultOutputDevice, ScopeGlobal)
	size := uint32(4)
	if err = GetProperty(AudioObjectSystemObject, addr, unsafe.Pointer(&id), &size); err != nil {
		return 0, "", "", err
	}
	if uid, err = GetStringProperty(id, PropDeviceUID, ScopeGlobal); err != nil {
		return 0, "", "", err
	}
	if name, err = GetStringProperty(id, PropObjectName, ScopeGlobal); err != nil {
		return 0, "", "", err
	}
	return id, uid, name, nil
}

// DeviceInfo describes one audio device.
type DeviceInfo struct {
	ID   uint32
	UID  string
	Name string
}

// OutputDevices lists the devices with a UID, the shape the endpoint picker
// and the mute setting need. Private aggregates this process created carry
// the saundo UID prefix and are skipped.
func OutputDevices() ([]DeviceInfo, error) {
	addr := PropertyAddress(PropDevices, ScopeGlobal)
	var size uint32
	if err := GetPropertySize(AudioObjectSystemObject, addr, &size); err != nil {
		return nil, err
	}
	ids := make([]uint32, int(size)/4)
	if err := GetProperty(AudioObjectSystemObject, addr, unsafe.Pointer(&ids[0]), &size); err != nil {
		return nil, err
	}
	out := make([]DeviceInfo, 0, len(ids))
	for _, id := range ids {
		uid, uerr := GetStringProperty(id, PropDeviceUID, ScopeGlobal)
		if uerr != nil || uid == "" || len(uid) >= 7 && uid[:7] == "saundo." {
			continue
		}
		name, nerr := GetStringProperty(id, PropObjectName, ScopeGlobal)
		if nerr != nil || name == "" {
			name = uid
		}
		out = append(out, DeviceInfo{ID: id, UID: uid, Name: name})
	}
	return out, nil
}

// DeviceForUID resolves a device UID to its audio object.
func DeviceForUID(uid string) (uint32, error) {
	k, err := coreAudio()
	if err != nil {
		return 0, err
	}
	str := NSString(uid)
	qual := unsafe.Pointer(&str)
	addr := PropertyAddress(PropTranslateUIDToDevice, ScopeGlobal)
	var dev uint32
	size := uint32(4)
	if st := k.getProp(AudioObjectSystemObject, &addr, uint32(unsafe.Sizeof(str)), qual, &size, unsafe.Pointer(&dev)); st != 0 {
		return 0, StatusError("AudioObjectGetPropertyData(TranslateUIDToDevice)", st)
	}
	if dev == 0 {
		return 0, fmt.Errorf("coreaudio: no device for UID %q", uid)
	}
	return dev, nil
}

// CreateProcessTap builds a tap from a CATapDescription and returns its
// object id. description is an Objective-C object reference, passed as the
// pointer value it already is.
func CreateProcessTap(description ID) (uint32, error) {
	k, err := coreAudio()
	if err != nil {
		return 0, err
	}
	var tap uint32
	r, _, _ := purego.SyscallN(k.createTapAddr, uintptr(description), uintptr(unsafe.Pointer(&tap)))
	if st := int32(uint32(r)); st != 0 {
		return 0, StatusError("AudioHardwareCreateProcessTap", st)
	}
	return tap, nil
}

// DestroyProcessTap tears a tap down.
func DestroyProcessTap(tap uint32) error {
	k, err := coreAudio()
	if err != nil {
		return err
	}
	if st := k.destroyTap(tap); st != 0 {
		return StatusError("AudioHardwareDestroyProcessTap", st)
	}
	return nil
}

// CreateAggregateDevice builds a private aggregate device from a property
// dictionary and returns its object id. description is an Objective-C
// dictionary reference, passed as the pointer value it already is.
func CreateAggregateDevice(description ID) (uint32, error) {
	k, err := coreAudio()
	if err != nil {
		return 0, err
	}
	var dev uint32
	r, _, _ := purego.SyscallN(k.createAggAddr, uintptr(description), uintptr(unsafe.Pointer(&dev)))
	if st := int32(uint32(r)); st != 0 {
		return 0, StatusError("AudioHardwareCreateAggregateDevice", st)
	}
	return dev, nil
}

// DestroyAggregateDevice tears an aggregate device down.
func DestroyAggregateDevice(dev uint32) error {
	k, err := coreAudio()
	if err != nil {
		return err
	}
	if st := k.destroyAgg(dev); st != 0 {
		return StatusError("AudioHardwareDestroyAggregateDevice", st)
	}
	return nil
}

// DeviceIOProcID is the handle of one registered IOProc.
type DeviceIOProcID = unsafe.Pointer

// CreateDeviceIOProc registers proc as the IOProc of a device. proc must
// match the AudioDeviceIOProc C ABI; build it with purego.NewCallback, which
// returns the pointer as a uintptr.
func CreateDeviceIOProc(device uint32, proc uintptr) (DeviceIOProcID, error) {
	k, err := coreAudio()
	if err != nil {
		return nil, err
	}
	var procID unsafe.Pointer
	if st := k.createIOProc(device, proc, nil, &procID); st != 0 {
		return nil, StatusError("AudioDeviceCreateIOProcID", st)
	}
	return procID, nil
}

// DestroyDeviceIOProc unregisters an IOProc.
func DestroyDeviceIOProc(device uint32, procID DeviceIOProcID) error {
	k, err := coreAudio()
	if err != nil {
		return err
	}
	if st := k.destroyIOProc(device, procID); st != 0 {
		return StatusError("AudioDeviceDestroyIOProcID", st)
	}
	return nil
}

// StartDevice starts IO on a device for the given IOProc.
func StartDevice(device uint32, procID DeviceIOProcID) error {
	k, err := coreAudio()
	if err != nil {
		return err
	}
	if st := k.start(device, procID); st != 0 {
		return StatusError("AudioDeviceStart", st)
	}
	return nil
}

// StopDevice stops IO on a device for the given IOProc.
func StopDevice(device uint32, procID DeviceIOProcID) error {
	k, err := coreAudio()
	if err != nil {
		return err
	}
	if st := k.stop(device, procID); st != 0 {
		return StatusError("AudioDeviceStop", st)
	}
	return nil
}

// DeviceRunningSomewhere reports whether a device is running for any client.
func DeviceRunningSomewhere(device uint32) (bool, error) {
	var running uint32
	size := uint32(4)
	err := GetProperty(device, PropertyAddress(PropDeviceIsRunningSomewhere, ScopeGlobal), unsafe.Pointer(&running), &size)
	return err == nil && running != 0, err
}

// ImageIO binds for the app-icon PNG thumbnails.
var (
	cfDataCreate                        func(alloc uintptr, bytes unsafe.Pointer, length uintptr) uintptr
	cfDataGetBytePtr                    func(data uintptr) unsafe.Pointer
	cfDataGetLength                     func(data uintptr) uintptr
	cfDataCreateMutable                 func(alloc uintptr, length uintptr) uintptr
	cgImageSourceCreateWithData         func(data uintptr, options uintptr) uintptr
	cgImageSourceCreateThumbnailAtIndex func(src uintptr, index int, options uintptr) uintptr
	cgImageDestinationCreateWithData    func(data uintptr, fileType uintptr, count int, assets uintptr) uintptr
	cgImageDestinationAddImage          func(dest uintptr, image uintptr, properties uintptr) int32
	cgImageDestinationFinalize          func(dest uintptr) int32
	imageIOOnce                         sync.Once
	imageIOErr                          error
	imageIO                             uintptr
)

func ensureImageIO() error {
	imageIOOnce.Do(func() {
		if err := load(); err != nil {
			imageIOErr = err
			return
		}
		var err error
		if imageIO, err = mustDlopen("/System/Library/Frameworks/ImageIO.framework/ImageIO"); err != nil {
			imageIOErr = fmt.Errorf("imageio: %w", err)
			return
		}
		for _, e := range []struct {
			fn any
			nm string
		}{
			{&cfDataCreate, "CFDataCreate"},
			{&cfDataGetBytePtr, "CFDataGetBytePtr"},
			{&cfDataGetLength, "CFDataGetLength"},
			{&cfDataCreateMutable, "CFDataCreateMutable"},
			{&cgImageSourceCreateWithData, "CGImageSourceCreateWithData"},
			{&cgImageSourceCreateThumbnailAtIndex, "CGImageSourceCreateThumbnailAtIndex"},
			{&cgImageDestinationCreateWithData, "CGImageDestinationCreateWithData"},
			{&cgImageDestinationAddImage, "CGImageDestinationAddImage"},
			{&cgImageDestinationFinalize, "CGImageDestinationFinalize"},
		} {
			purego.RegisterLibFunc(e.fn, imageIO, e.nm)
		}
		if cfReleaseFn == nil {
			imageIOErr = errors.New("imageio: CFRelease unavailable")
		}
	})
	return imageIOErr
}

// pngThumbnail re-encodes image bytes (any format ImageIO reads) as a PNG
// thumbnail of at most maxPixel pixels on the larger side.
func pngThumbnail(data []byte) []byte {
	const maxPixelSize = 128
	if err := ensureImageIO(); err != nil || len(data) == 0 {
		return nil
	}
	srcData := cfDataCreate(0, unsafe.Pointer(&data[0]), uintptr(len(data)))
	if srcData == 0 {
		return nil
	}
	defer cfReleaseFn(srcData)
	src := cgImageSourceCreateWithData(srcData, 0)
	if src == 0 {
		return nil
	}
	defer cfReleaseFn(src)
	opts := Send(mustClass("NSMutableDictionary"), "dictionary")
	Send(opts, "setObject:forKey:", uintptr(NumberBool(true)),
		uintptr(NSString("kCGImageSourceCreateThumbnailFromImageAlways")))
	Send(opts, "setObject:forKey:", uintptr(Send(mustClass("NSNumber"), "numberWithInteger:", 128)),
		uintptr(NSString("kCGImageSourceThumbnailMaxPixelSize")))
	img := cgImageSourceCreateThumbnailAtIndex(src, 0, uintptr(opts))
	if img == 0 {
		return nil
	}
	defer cfReleaseFn(img)
	out := cfDataCreateMutable(0, 0)
	if out == 0 {
		return nil
	}
	defer cfReleaseFn(out)
	dest := cgImageDestinationCreateWithData(out, uintptr(NSString("public.png")), 1, 0)
	if dest == 0 {
		return nil
	}
	defer cfReleaseFn(dest)
	cgImageDestinationAddImage(dest, img, 0)
	if cgImageDestinationFinalize(dest) == 0 {
		return nil // finalize returns success as a Boolean
	}
	n := cfDataGetLength(out)
	if n == 0 {
		return nil
	}
	p := cfDataGetBytePtr(out)
	return append([]byte(nil), unsafe.Slice((*byte)(p), n)...)
}
