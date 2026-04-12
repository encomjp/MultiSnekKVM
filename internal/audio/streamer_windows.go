//go:build windows

package audio

import (
	"encoding/binary"
	"fmt"
	"log"
	"sync"
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
	_CLSCTX_ALL                              = 0x17
	_COINIT_MULTITHREADED                    = 0x0
	_eRender                                 = 0
	_eCapture                                = 1
	_eConsole                                = 0
	_AUDCLNT_SHAREMODE_SHARED                = 0
	_AUDCLNT_STREAMFLAGS_LOOPBACK            = 0x00020000
	_AUDCLNT_STREAMFLAGS_AUTOCONVERTPCM      = 0x80000000
	_AUDCLNT_STREAMFLAGS_SRC_DEFAULT_QUALITY = 0x08000000
	_audioMaxPlayBuf                         = 1536000
	waveFormatExSize                         = 18
	_waveFormatPcm                           = 0x0001
	_waveFormatIEEEFloat                     = 0x0003
	_waveFormatExtensible                    = 0xfffe

	_AUDCLNT_E_DEVICE_INVALIDATED           uint32 = 0x88890004
	_AUDCLNT_BUFFERFLAGS_SILENT             uint32 = 0x2
	_AUDCLNT_BUFFERFLAGS_DATA_DISCONTINUITY uint32 = 0x1

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

	_IID_IMMDeviceCollection = wGUID{0x0BD7A1BE, 0x7A1A, 0x44DB, [8]byte{0x83, 0x97, 0xCC, 0x53, 0x92, 0x38, 0x7B, 0x5E}}
	_IID_IPropertyStore      = wGUID{0x886D8EEB, 0x8CF2, 0x4446, [8]byte{0x8D, 0x02, 0xCD, 0xBA, 0x1D, 0xBD, 0xCF, 0x99}}
	_PKEY_FriendlyName_GUID  = wGUID{0xa45c254e, 0xdf1c, 0x4efd, [8]byte{0x80, 0x20, 0x67, 0xd1, 0x46, 0xa8, 0x50, 0xe0}}
)

func comCall(obj uintptr, idx int, args ...uintptr) uintptr {
	vtbl := *(*uintptr)(unsafe.Pointer(obj))
	fn := *(*uintptr)(unsafe.Pointer(vtbl + uintptr(idx)*unsafe.Sizeof(uintptr(0))))
	r, _, _ := syscall.SyscallN(fn, append([]uintptr{obj}, args...)...)
	return r
}

func comRelease(obj uintptr) {
	if obj != 0 {
		comCall(obj, 2)
	}
}

func enterMmcss(label string) (handle uintptr) {
	taskName, _ := syscall.UTF16PtrFromString("Pro Audio")
	var taskIndex uint32
	h, _, err := pAvSetMmThreadCharacteristicsW.Call(
		uintptr(unsafe.Pointer(taskName)),
		uintptr(unsafe.Pointer(&taskIndex)),
	)
	if h == 0 {
		log.Printf("%s: MMCSS AvSetMmThreadCharacteristics failed: %v", label, err)
		return 0
	}
	return h
}

func leaveMmcss(handle uintptr) {
	if handle != 0 {
		pAvRevertMmThreadCharacteristics.Call(handle)
	}
}

type waveFormatEx struct {
	FormatTag      uint16
	Channels       uint16
	SamplesPerSec  uint32
	AvgBytesPerSec uint32
	BlockAlign     uint16
	BitsPerSample  uint16
	CbSize         uint16
}

type audioSampleCodec int

const (
	audioSampleCodecUnknown audioSampleCodec = iota
	audioSampleCodecPCM16
	audioSampleCodecPCM24
	audioSampleCodecPCM32
	audioSampleCodecFloat32
)

var waveFormatSubtypeTail = [12]byte{0x00, 0x00, 0x10, 0x00, 0x80, 0x00, 0x00, 0xaa, 0x00, 0x38, 0x9b, 0x71}

type AudioDevice struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Flow string `json:"flow"`
}

type AudioStreamer struct {
	mu        sync.Mutex
	capturing bool
	playing   bool
	playReady bool
	playFmt   []byte
	capStop   chan struct{}
	capWg     sync.WaitGroup
	playStop  chan struct{}
	playWg    sync.WaitGroup

	playMu       sync.Mutex
	playBuf      []byte
	overflowLogs int64

	micCapturing bool
	micPlaying   bool
	micPlayReady bool
	micPlayFmt   []byte
	micCapStop   chan struct{}
	micCapWg     sync.WaitGroup
	micPlayStop  chan struct{}
	micPlayWg    sync.WaitGroup

	micPlayMu       sync.Mutex
	micPlayBuf      []byte
	micOverflowLogs int64

	captureDeviceID     string
	playbackDeviceID    string
	micDeviceID         string
	micPlaybackDeviceID string
	qualityProfile      string
}

func NewAudioStreamer() (*AudioStreamer, error) {
	return &AudioStreamer{qualityProfile: audioProfileBalanced}, nil
}

func cloneBytes(src []byte) []byte {
	if len(src) == 0 {
		return nil
	}
	dup := make([]byte, len(src))
	copy(dup, src)
	return dup
}

func decodeWaveFormat(raw []byte) (waveFormatEx, bool) {
	if len(raw) < waveFormatExSize {
		return waveFormatEx{}, false
	}
	return waveFormatEx{
		FormatTag:      binary.LittleEndian.Uint16(raw[0:2]),
		Channels:       binary.LittleEndian.Uint16(raw[2:4]),
		SamplesPerSec:  binary.LittleEndian.Uint32(raw[4:8]),
		AvgBytesPerSec: binary.LittleEndian.Uint32(raw[8:12]),
		BlockAlign:     binary.LittleEndian.Uint16(raw[12:14]),
		BitsPerSample:  binary.LittleEndian.Uint16(raw[14:16]),
		CbSize:         binary.LittleEndian.Uint16(raw[16:18]),
	}, true
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

func describeWaveFormat(raw []byte) string {
	wfx, ok := decodeWaveFormat(raw)
	if !ok {
		return fmt.Sprintf("invalid format (%d bytes)", len(raw))
	}
	return fmt.Sprintf("%dHz %dch %dbit (blockAlign=%d, formatTag=0x%04x)",
		wfx.SamplesPerSec, wfx.Channels, wfx.BitsPerSample, wfx.BlockAlign, wfx.FormatTag)
}

func encodeSenderWaveFormat(raw []byte) []byte {
	wfx, ok := decodeWaveFormat(raw)
	if !ok {
		return nil
	}
	buf := make([]byte, waveFormatExSize)
	binary.LittleEndian.PutUint16(buf[0:2], wfx.FormatTag)
	binary.LittleEndian.PutUint16(buf[2:4], wfx.Channels)
	binary.LittleEndian.PutUint32(buf[4:8], wfx.SamplesPerSec)
	binary.LittleEndian.PutUint32(buf[8:12], wfx.AvgBytesPerSec)
	binary.LittleEndian.PutUint16(buf[12:14], wfx.BlockAlign)
	binary.LittleEndian.PutUint16(buf[14:16], wfx.BitsPerSample)
	binary.LittleEndian.PutUint16(buf[16:18], 0)
	return buf
}

func blockAlignForFormat(raw []byte) int {
	wfx, ok := decodeWaveFormat(raw)
	if !ok {
		return 0
	}
	return int(wfx.BlockAlign)
}

func referenceTimeFromDuration(d time.Duration) int64 {
	if d <= 0 {
		return 0
	}
	return d.Nanoseconds() / 100
}

func describeSharedBufferDuration(d time.Duration) string {
	if d <= 0 {
		return "engine default"
	}
	return d.String()
}

func initializeSharedAudioClient(client uintptr, streamFlags uintptr, targetDuration time.Duration, pwfx uintptr, label string) error {
	attempts := []time.Duration{targetDuration, 40 * time.Millisecond, 0, 200 * time.Millisecond}
	var lastHR uintptr
	for index, duration := range attempts {
		hr := comCall(client, 3,
			_AUDCLNT_SHAREMODE_SHARED,
			streamFlags,
			uintptr(referenceTimeFromDuration(duration)),
			0, pwfx, 0,
		)
		if int32(hr) >= 0 {
			if index > 0 {
				log.Printf("%s: shared buffer fallback to %s", label, describeSharedBufferDuration(duration))
			}
			return nil
		}
		lastHR = hr
	}
	return fmt.Errorf("0x%08x", lastHR)
}

func (a *AudioStreamer) SetQualityProfile(profile string) error {
	if !ValidProfile(profile) {
		return fmt.Errorf("invalid audio quality profile %q", profile)
	}
	a.mu.Lock()
	a.qualityProfile = NormalizeProfile(profile)
	a.mu.Unlock()
	return nil
}

func (a *AudioStreamer) currentQualityProfileSpec() audioProfileSpec {
	a.mu.Lock()
	profile := a.qualityProfile
	a.mu.Unlock()
	return audioProfileSpecForName(profile)
}

func queuedAudioBytesForDuration(raw []byte, duration time.Duration) int {
	wfx, ok := decodeWaveFormat(raw)
	if !ok || wfx.AvgBytesPerSec == 0 {
		return 0
	}
	bytes := int((int64(wfx.AvgBytesPerSec) * int64(duration)) / int64(time.Second))
	blockAlign := int(wfx.BlockAlign)
	if blockAlign > 0 {
		bytes -= bytes % blockAlign
		if bytes == 0 && duration > 0 {
			bytes = blockAlign
		}
	}
	return bytes
}

func loopbackSilentKeepalivePayload(formatRaw []byte, interval time.Duration, lastEmitAt, now time.Time) []byte {
	if interval <= 0 || lastEmitAt.IsZero() || !now.After(lastEmitAt) || now.Sub(lastEmitAt) < interval {
		return nil
	}
	bytes := queuedAudioBytesForDuration(formatRaw, interval)
	if bytes <= 0 {
		return nil
	}
	return make([]byte, bytes)
}

func maxQueuedAudioBytes(raw []byte, maxBufferedDuration time.Duration) int {
	maxBytes := queuedAudioBytesForDuration(raw, maxBufferedDuration)
	if maxBytes <= 0 || maxBytes > _audioMaxPlayBuf {
		return _audioMaxPlayBuf
	}
	return maxBytes
}

const playbackStartupBufferedDuration = 500 * time.Millisecond

func effectiveMaxQueuedAudioBytes(raw []byte, maxBufferedDuration time.Duration, ready bool) int {
	maxBytes := maxQueuedAudioBytes(raw, maxBufferedDuration)
	if ready {
		return maxBytes
	}
	startupBytes := queuedAudioBytesForDuration(raw, playbackStartupBufferedDuration)
	if startupBytes > maxBytes && startupBytes <= _audioMaxPlayBuf {
		return startupBytes
	}
	return maxBytes
}

func trimQueuedAudio(buf []byte, blockAlign, maxBytes int) ([]byte, int) {
	if maxBytes <= 0 || len(buf) <= maxBytes {
		return buf, 0
	}
	trim := len(buf) - maxBytes
	if blockAlign > 0 {
		trim += (blockAlign - (trim % blockAlign)) % blockAlign
	}
	if trim >= len(buf) {
		trim = len(buf)
	}
	remaining := buf[trim:]
	n := copy(buf[:len(remaining)], remaining)
	return buf[:n], trim
}

func writeRenderBuffer(renderClient uintptr, frames, bytesPerFrame int, payload []byte, flags uint32, label string) bool {
	if frames <= 0 {
		return true
	}

	var bufPtr uintptr
	hr := comCall(renderClient, 3, uintptr(frames), uintptr(unsafe.Pointer(&bufPtr)))
	if int32(hr) < 0 {
		log.Printf("%s: GetBuffer failed: 0x%08x", label, hr)
		return false
	}

	if bufPtr != 0 && flags&_AUDCLNT_BUFFERFLAGS_SILENT == 0 {
		target := unsafe.Slice((*byte)(unsafe.Pointer(bufPtr)), frames*bytesPerFrame)
		copied := copy(target, payload)
		if copied < len(target) {
			clear(target[copied:])
		}
	}

	hr = comCall(renderClient, 4, uintptr(frames), uintptr(flags))
	if int32(hr) < 0 {
		log.Printf("%s: ReleaseBuffer failed: 0x%08x", label, hr)
		return false
	}
	return true
}
