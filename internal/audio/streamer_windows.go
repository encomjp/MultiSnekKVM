//go:build windows

package audio

import (
	"encoding/binary"
	"fmt"
	"log"
	"runtime"
	"syscall"
	"time"
	"unsafe"
)

var (
	ole32             = syscall.NewLazyDLL("ole32.dll")
	pCoInitializeEx   = ole32.NewProc("CoInitializeEx")
	pCoUninitialize   = ole32.NewProc("CoUninitialize")
	pCoCreateInstance = ole32.NewProc("CoCreateInstance")
	pCoTaskMemFree    = ole32.NewProc("CoTaskMemFree")
	pPropVariantClear = ole32.NewProc("PropVariantClear")

	avrt                             = syscall.NewLazyDLL("avrt.dll")
	pAvSetMmThreadCharacteristicsW   = avrt.NewProc("AvSetMmThreadCharacteristicsW")
	pAvRevertMmThreadCharacteristics = avrt.NewProc("AvRevertMmThreadCharacteristics")
)

type wGUID struct {
	Data1 uint32
	Data2 uint16
	Data3 uint16
	Data4 [8]byte
}

const (
	_CLSCTX_ALL                   = 0x17
	_COINIT_MULTITHREADED         = 0x0
	_eRender                      = 0
	_eCapture                     = 1
	_eConsole                     = 0
	_AUDCLNT_SHAREMODE_SHARED     = 0
	_AUDCLNT_STREAMFLAGS_LOOPBACK = 0x00020000

	_AUDCLNT_BUFFERFLAGS_SILENT uint32 = 0x2

	_DEVICE_STATE_ACTIVE          = 0x1
	_STGM_READ                    = 0
	_VT_LPWSTR                    = 31
	_PKEY_FriendlyName_PID uint32 = 14
)

var (
	_CLSID_MMDeviceEnumerator = wGUID{0xBCDE0395, 0xE52F, 0x467C, [8]byte{0x8E, 0x3D, 0xC4, 0x57, 0x92, 0x91, 0x69, 0x2E}}
	_IID_IMMDeviceEnumerator  = wGUID{0xA95664D2, 0x9614, 0x4F35, [8]byte{0xA7, 0x46, 0xDE, 0x8D, 0xB6, 0x36, 0x17, 0xE6}}
	_IID_IAudioClient         = wGUID{0x1CB9AD4C, 0xDBFA, 0x4C32, [8]byte{0xB1, 0x78, 0xC2, 0xF5, 0x68, 0xA7, 0x03, 0xB2}}
	_IID_IAudioCaptureClient  = wGUID{0xC8ADBD64, 0xE71E, 0x48A0, [8]byte{0xA4, 0xDE, 0x18, 0x5C, 0x39, 0x5C, 0xD3, 0x17}}
	_IID_IAudioRenderClient   = wGUID{0xF294ACFC, 0x3146, 0x4483, [8]byte{0xA7, 0xBF, 0xAD, 0xDC, 0xA7, 0xC2, 0x60, 0xE2}}
	_IID_IAudioEndpointVolume = wGUID{0x5CDF2C82, 0x841E, 0x4546, [8]byte{0x97, 0x22, 0x0C, 0xF7, 0x40, 0x78, 0x22, 0x9A}}

	_IID_IMMDeviceCollection = wGUID{0x0BD7A1BE, 0x7A1A, 0x44DB, [8]byte{0x83, 0x97, 0xCC, 0x53, 0x92, 0x38, 0x7B, 0x5E}}
	_IID_IPropertyStore      = wGUID{0x886D8EEB, 0x8CF2, 0x4446, [8]byte{0x8D, 0x02, 0xCD, 0xBA, 0x1D, 0xBD, 0xCF, 0x99}}
	_PKEY_FriendlyName_GUID  = wGUID{0xa45c254e, 0xdf1c, 0x4efd, [8]byte{0x80, 0x20, 0x67, 0xd1, 0x46, 0xa8, 0x50, 0xe0}}
)

// COM vtable indices.
const (
	vtRelease              = 2
	vtDeviceActivate       = 3
	vtClientInitialize     = 3
	vtClientGetBufferSize  = 4
	vtClientGetPadding     = 6
	vtClientGetMixFormat   = 8
	vtClientStart          = 10
	vtClientStop           = 11
	vtClientGetService     = 14
	vtCaptureGetBuffer     = 3
	vtCaptureReleaseBuffer = 4
	vtCaptureNextPacket    = 5
	vtRenderGetBuffer      = 3
	vtRenderReleaseBuffer  = 4
	vtEndpointSetMute      = 14
	vtEndpointGetMute      = 15
)

// comCall invokes a COM method without allocating (up to 7 arguments).
func comCall(obj uintptr, idx int, args ...uintptr) uintptr {
	vtbl := *(*uintptr)(unsafe.Pointer(obj))
	fn := *(*uintptr)(unsafe.Pointer(vtbl + uintptr(idx)*unsafe.Sizeof(uintptr(0))))
	var a [8]uintptr
	a[0] = obj
	n := copy(a[1:], args)
	r, _, _ := syscall.SyscallN(fn, a[:n+1]...)
	return r
}

func comRelease(obj uintptr) {
	if obj != 0 {
		comCall(obj, vtRelease)
	}
}

func failed(hr uintptr) bool { return int32(hr) < 0 }

func enterMmcss(label string) uintptr {
	taskName, _ := syscall.UTF16PtrFromString("Pro Audio")
	var taskIndex uint32
	h, _, err := pAvSetMmThreadCharacteristicsW.Call(uintptr(unsafe.Pointer(taskName)), uintptr(unsafe.Pointer(&taskIndex)))
	if h == 0 {
		log.Printf("%s: MMCSS registration failed: %v", label, err)
	}
	return h
}

func leaveMmcss(handle uintptr) {
	if handle != 0 {
		pAvRevertMmThreadCharacteristics.Call(handle)
	}
}

func waveFormatFromPointer(ptr uintptr) []byte {
	if ptr == 0 {
		return nil
	}
	header := unsafe.Slice((*byte)(unsafe.Pointer(ptr)), waveFormatExSize)
	total := waveFormatExSize + int(binary.LittleEndian.Uint16(header[16:18]))
	raw := make([]byte, total)
	copy(raw, unsafe.Slice((*byte)(unsafe.Pointer(ptr)), total))
	return raw
}

func initializeSharedAudioClient(client uintptr, streamFlags uintptr, target time.Duration, pwfx uintptr) error {
	var lastHR uintptr
	for _, d := range []time.Duration{target, 40 * time.Millisecond, 0} {
		hr := comCall(client, vtClientInitialize, _AUDCLNT_SHAREMODE_SHARED, streamFlags, uintptr(d.Nanoseconds()/100), 0, pwfx, 0)
		if !failed(hr) {
			return nil
		}
		lastHR = hr
	}
	return fmt.Errorf("IAudioClient::Initialize: 0x%08x", lastHR)
}

// endpoint is an activated WASAPI device and its shared-mode mix format.
type endpoint struct {
	enum, dev, client, pwfx uintptr
	format                  []byte
	spec                    pcmSpec
}

func openEndpoint(flow uintptr, deviceID string) (*endpoint, error) {
	e := &endpoint{}
	hr, _, _ := pCoCreateInstance.Call(uintptr(unsafe.Pointer(&_CLSID_MMDeviceEnumerator)), 0, _CLSCTX_ALL,
		uintptr(unsafe.Pointer(&_IID_IMMDeviceEnumerator)), uintptr(unsafe.Pointer(&e.enum)))
	if failed(hr) {
		return nil, fmt.Errorf("create device enumerator: 0x%08x", hr)
	}
	dev, ok := getEndpointByID(e.enum, flow, deviceID)
	if !ok {
		e.close()
		return nil, fmt.Errorf("no audio endpoint available")
	}
	e.dev = dev
	if hr := comCall(e.dev, vtDeviceActivate, uintptr(unsafe.Pointer(&_IID_IAudioClient)), _CLSCTX_ALL, 0, uintptr(unsafe.Pointer(&e.client))); failed(hr) {
		e.close()
		return nil, fmt.Errorf("activate audio client: 0x%08x", hr)
	}
	if hr := comCall(e.client, vtClientGetMixFormat, uintptr(unsafe.Pointer(&e.pwfx))); failed(hr) {
		e.close()
		return nil, fmt.Errorf("get mix format: 0x%08x", hr)
	}
	e.format = waveFormatFromPointer(e.pwfx)
	spec, err := parsePCMSpec(e.format)
	if err != nil {
		e.close()
		return nil, err
	}
	e.spec = spec
	return e, nil
}

func (e *endpoint) close() {
	if e.pwfx != 0 {
		pCoTaskMemFree.Call(e.pwfx)
	}
	comRelease(e.client)
	comRelease(e.dev)
	comRelease(e.enum)
}

// setEndpointMute mutes or unmutes a render endpoint and returns the previous state.
func setEndpointMute(dev uintptr, mute bool) (previous bool, err error) {
	var vol uintptr
	if hr := comCall(dev, vtDeviceActivate, uintptr(unsafe.Pointer(&_IID_IAudioEndpointVolume)), _CLSCTX_ALL, 0, uintptr(unsafe.Pointer(&vol))); failed(hr) {
		return false, fmt.Errorf("activate endpoint volume: 0x%08x", hr)
	}
	defer comRelease(vol)
	var cur int32
	comCall(vol, vtEndpointGetMute, uintptr(unsafe.Pointer(&cur)))
	var flag uintptr
	if mute {
		flag = 1
	}
	if hr := comCall(vol, vtEndpointSetMute, flag, 0); failed(hr) {
		return cur != 0, fmt.Errorf("set mute: 0x%08x", hr)
	}
	return cur != 0, nil
}

// runWithRetries runs device sessions on a locked, MMCSS-boosted COM thread.
// A session that ran for a while and then failed (device unplugged, default
// device changed) is reopened quickly; repeated quick failures back off and
// eventually give up.
func runWithRetries(label string, stop <-chan struct{}, session func() (stopped bool, err error)) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if hr, _, _ := pCoInitializeEx.Call(0, _COINIT_MULTITHREADED); hr == 0 || hr == 1 {
		defer pCoUninitialize.Call()
	}
	defer leaveMmcss(enterMmcss(label))

	delay := 250 * time.Millisecond
	failures := 0
	for {
		started := time.Now()
		stopped, err := session()
		if stopped {
			return
		}
		if time.Since(started) > 5*time.Second {
			failures, delay = 0, 250*time.Millisecond
		}
		failures++
		if failures > 8 {
			log.Printf("%s: giving up: %v", label, err)
			return
		}
		log.Printf("%s: %v; reopening in %v", label, err, delay)
		select {
		case <-stop:
			return
		case <-time.After(delay):
		}
		if delay < 4*time.Second {
			delay *= 2
		}
	}
}

func (a *AudioStreamer) runCapture(kind StreamKind, h CaptureHandler, stop <-chan struct{}) {
	runWithRetries(kind.String()+" capture", stop, func() (bool, error) { return a.captureSession(kind, h, stop) })
}

func (a *AudioStreamer) captureSession(kind StreamKind, h CaptureHandler, stop <-chan struct{}) (bool, error) {
	flow, flags := uintptr(_eCapture), uintptr(0)
	if kind == StreamDesktop {
		flow, flags = _eRender, _AUDCLNT_STREAMFLAGS_LOOPBACK
	}
	e, err := openEndpoint(flow, a.captureDeviceID(kind))
	if err != nil {
		return false, err
	}
	defer e.close()

	if err := initializeSharedAudioClient(e.client, flags, a.qualityProfile().TargetClientDuration, e.pwfx); err != nil {
		return false, err
	}
	var capClient uintptr
	if hr := comCall(e.client, vtClientGetService, uintptr(unsafe.Pointer(&_IID_IAudioCaptureClient)), uintptr(unsafe.Pointer(&capClient))); failed(hr) {
		return false, fmt.Errorf("get capture client: 0x%08x", hr)
	}
	defer comRelease(capClient)

	if kind == StreamDesktop && a.muteSourceEnabled() {
		if prev, err := setEndpointMute(e.dev, true); err != nil {
			log.Printf("audio capture: mute source: %v", err)
		} else if !prev {
			defer func() { _, _ = setEndpointMute(e.dev, false) }()
		}
	}

	h.OnCaptureFormat(e.format)
	if hr := comCall(e.client, vtClientStart); failed(hr) {
		return false, fmt.Errorf("start: 0x%08x", hr)
	}
	defer comCall(e.client, vtClientStop)
	log.Printf("%s capture active: %s", kind, describeWaveFormat(e.format))

	bpf := e.spec.BlockAlign
	// Loopback delivers no packets while nothing plays. Send 20 ms of silence
	// at the real-time rate so the receiver stays primed instead of
	// re-buffering (and clipping the start) every time a sound begins.
	const keepaliveEvery = 20 * time.Millisecond
	keepalive := make([]byte, e.spec.Rate/50*bpf)
	var silent []byte
	lastData := time.Now()

	for {
		select {
		case <-stop:
			return true, nil
		default:
		}
		var pkt uint32
		if hr := comCall(capClient, vtCaptureNextPacket, uintptr(unsafe.Pointer(&pkt))); failed(hr) {
			return false, fmt.Errorf("GetNextPacketSize: 0x%08x", hr)
		}
		if pkt == 0 {
			if kind == StreamDesktop && time.Since(lastData) >= keepaliveEvery {
				h.OnCaptureData(keepalive)
				lastData = lastData.Add(keepaliveEvery)
				if time.Since(lastData) > 5*keepaliveEvery {
					lastData = time.Now()
				}
				continue
			}
			time.Sleep(3 * time.Millisecond)
			continue
		}
		var data uintptr
		var frames, bufFlags uint32
		if hr := comCall(capClient, vtCaptureGetBuffer, uintptr(unsafe.Pointer(&data)), uintptr(unsafe.Pointer(&frames)), uintptr(unsafe.Pointer(&bufFlags)), 0, 0); failed(hr) {
			return false, fmt.Errorf("GetBuffer: 0x%08x", hr)
		}
		if frames > 0 {
			n := int(frames) * bpf
			if bufFlags&_AUDCLNT_BUFFERFLAGS_SILENT != 0 || data == 0 {
				if len(silent) < n {
					silent = make([]byte, n)
				}
				h.OnCaptureData(silent[:n])
			} else {
				// DATA_DISCONTINUITY only marks a glitch before this packet;
				// its samples are valid and are kept.
				h.OnCaptureData(unsafe.Slice((*byte)(unsafe.Pointer(data)), n))
			}
			lastData = time.Now()
		}
		comCall(capClient, vtCaptureReleaseBuffer, uintptr(frames))
	}
}

func (a *AudioStreamer) runRender(kind StreamKind, stop <-chan struct{}) {
	runWithRetries(kind.String()+" playback", stop, func() (bool, error) { return a.renderSession(kind, stop) })
}

func (a *AudioStreamer) renderSession(kind StreamKind, stop <-chan struct{}) (bool, error) {
	label := kind.String() + " playback"
	e, err := openEndpoint(_eRender, a.renderDeviceID(kind))
	if err != nil {
		return false, err
	}
	defer e.close()

	if err := initializeSharedAudioClient(e.client, 0, a.qualityProfile().TargetClientDuration, e.pwfx); err != nil {
		return false, err
	}
	var bufFrames uint32
	if hr := comCall(e.client, vtClientGetBufferSize, uintptr(unsafe.Pointer(&bufFrames))); failed(hr) {
		return false, fmt.Errorf("get buffer size: 0x%08x", hr)
	}
	var render uintptr
	if hr := comCall(e.client, vtClientGetService, uintptr(unsafe.Pointer(&_IID_IAudioRenderClient)), uintptr(unsafe.Pointer(&render))); failed(hr) {
		return false, fmt.Errorf("get render client: 0x%08x", hr)
	}
	defer comRelease(render)

	spec := e.spec
	player := a.players[kind]
	samples := make([]float32, int(bufFrames)*spec.Channels)
	if !writeRender(render, player, samples, int(bufFrames), spec) {
		return false, fmt.Errorf("initial GetBuffer failed")
	}
	if hr := comCall(e.client, vtClientStart); failed(hr) {
		return false, fmt.Errorf("start: 0x%08x", hr)
	}
	defer comCall(e.client, vtClientStop)
	log.Printf("%s active: %s, %d-frame device buffer", label, describeWaveFormat(e.format), bufFrames)

	statsAt := time.Now()
	last := player.Stats()
	for {
		select {
		case <-stop:
			return true, nil
		default:
		}
		var padding uint32
		if hr := comCall(e.client, vtClientGetPadding, uintptr(unsafe.Pointer(&padding))); failed(hr) {
			return false, fmt.Errorf("GetCurrentPadding: 0x%08x", hr)
		}
		if avail := int(bufFrames) - int(padding); avail > 0 {
			if !writeRender(render, player, samples, avail, spec) {
				return false, fmt.Errorf("render GetBuffer/ReleaseBuffer failed")
			}
		}
		if time.Since(statsAt) >= 30*time.Second {
			s := player.Stats()
			if s.Underruns != last.Underruns || s.Overflows != last.Overflows {
				log.Printf("%s: buffered=%dms target=%dms underruns=+%d overflows=+%d",
					label, s.BufferedMs, s.TargetMs, s.Underruns-last.Underruns, s.Overflows-last.Overflows)
			}
			last, statsAt = s, time.Now()
		}
		time.Sleep(2 * time.Millisecond)
	}
}

// writeRender always fills exactly `frames` frames; the jitter buffer supplies
// faded silence for anything it doesn't have, so the device never starves and
// a short network gap never turns into a whole buffer of silence.
func writeRender(render uintptr, player *Playout, samples []float32, frames int, spec pcmSpec) bool {
	var ptr uintptr
	if hr := comCall(render, vtRenderGetBuffer, uintptr(frames), uintptr(unsafe.Pointer(&ptr))); failed(hr) || ptr == 0 {
		return false
	}
	player.Render(samples, frames, spec.Rate, spec.Channels)
	encodePCM(unsafe.Slice((*byte)(unsafe.Pointer(ptr)), frames*spec.BlockAlign), samples[:frames*spec.Channels], spec)
	return !failed(comCall(render, vtRenderReleaseBuffer, uintptr(frames), 0))
}
