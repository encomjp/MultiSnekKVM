//go:build windows

package audio

import (
	"log"
	"runtime"
	"syscall"
	"unsafe"
)

func lpwstrToString(ptr uintptr) string {
	if ptr == 0 {
		return ""
	}
	var buf []uint16
	for i := 0; ; i++ {
		wc := *(*uint16)(unsafe.Pointer(ptr + uintptr(i*2)))
		if wc == 0 {
			break
		}
		buf = append(buf, wc)
	}
	return syscall.UTF16ToString(buf)
}

func listAudioEndpoints(flow uintptr) []AudioDevice {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	hr, _, _ := pCoInitializeEx.Call(0, _COINIT_MULTITHREADED)
	if hr != 0 && hr != 1 {
		return nil
	}
	defer pCoUninitialize.Call()

	var enum uintptr
	hr, _, _ = pCoCreateInstance.Call(
		uintptr(unsafe.Pointer(&_CLSID_MMDeviceEnumerator)),
		0, _CLSCTX_ALL,
		uintptr(unsafe.Pointer(&_IID_IMMDeviceEnumerator)),
		uintptr(unsafe.Pointer(&enum)),
	)
	if int32(hr) < 0 {
		return nil
	}
	defer comRelease(enum)

	var coll uintptr
	hr = comCall(enum, 3, flow, _DEVICE_STATE_ACTIVE, uintptr(unsafe.Pointer(&coll)))
	if int32(hr) < 0 {
		return nil
	}
	defer comRelease(coll)

	var count uint32
	hr = comCall(coll, 3, uintptr(unsafe.Pointer(&count)))
	if int32(hr) < 0 {
		return nil
	}

	flowName := "render"
	if flow == _eCapture {
		flowName = "capture"
	}

	var devices []AudioDevice
	for i := uint32(0); i < count; i++ {
		var dev uintptr
		hr = comCall(coll, 4, uintptr(i), uintptr(unsafe.Pointer(&dev)))
		if int32(hr) < 0 {
			continue
		}

		var idPtr uintptr
		hr = comCall(dev, 5, uintptr(unsafe.Pointer(&idPtr)))
		devID := ""
		if int32(hr) >= 0 && idPtr != 0 {
			devID = lpwstrToString(idPtr)
			pCoTaskMemFree.Call(idPtr)
		}

		var store uintptr
		name := devID
		hr = comCall(dev, 4, _STGM_READ, uintptr(unsafe.Pointer(&store)))
		if int32(hr) >= 0 && store != 0 {
			type propKey struct {
				fmtID wGUID
				pid   uint32
			}
			key := propKey{fmtID: _PKEY_FriendlyName_GUID, pid: _PKEY_FriendlyName_PID}

			var pv [32]byte
			hr = comCall(store, 5,
				uintptr(unsafe.Pointer(&key)),
				uintptr(unsafe.Pointer(&pv[0])),
			)
			if int32(hr) >= 0 {
				vt := *(*uint16)(unsafe.Pointer(&pv[0]))
				if vt == _VT_LPWSTR {
					strPtr := *(*uintptr)(unsafe.Pointer(&pv[8]))
					if strPtr != 0 {
						name = lpwstrToString(strPtr)
					}
				}
				pPropVariantClear.Call(uintptr(unsafe.Pointer(&pv[0])))
			}
			comRelease(store)
		}

		comRelease(dev)
		if devID != "" {
			devices = append(devices, AudioDevice{ID: devID, Name: name, Flow: flowName})
		}
	}
	return devices
}

func ListRenderDevices() []AudioDevice {
	return listAudioEndpoints(_eRender)
}

func ListCaptureDevices() []AudioDevice {
	return listAudioEndpoints(_eCapture)
}

func (a *AudioStreamer) SetCaptureDeviceID(id string) {
	a.mu.Lock()
	changed := a.captureDeviceID != id
	a.captureDeviceID = id
	a.mu.Unlock()
	if changed && a.IsCapturing() {
		a.StopCapture()
	}
}

func (a *AudioStreamer) SetPlaybackDeviceID(id string) {
	a.mu.Lock()
	changed := a.playbackDeviceID != id
	a.playbackDeviceID = id
	a.mu.Unlock()
	if changed {
		a.StopPlayback()
	}
}

func (a *AudioStreamer) SetMicDeviceID(id string) {
	a.mu.Lock()
	changed := a.micDeviceID != id
	a.micDeviceID = id
	a.mu.Unlock()
	if changed {
		a.StopMicCapture()
	}
}

func (a *AudioStreamer) SetMicPlaybackDeviceID(id string) {
	a.mu.Lock()
	changed := a.micPlaybackDeviceID != id
	a.micPlaybackDeviceID = id
	a.mu.Unlock()
	if changed {
		a.StopMicPlayback()
	}
}

func (a *AudioStreamer) GetMicPlaybackDeviceID() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.micPlaybackDeviceID
}

func (a *AudioStreamer) GetCaptureDeviceID() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.captureDeviceID
}

func (a *AudioStreamer) GetPlaybackDeviceID() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.playbackDeviceID
}

func (a *AudioStreamer) GetMicDeviceID() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.micDeviceID
}

func getEndpointByID(enum uintptr, flow uintptr, id string) (uintptr, bool) {
	if id != "" {
		idUTF16, err := syscall.UTF16PtrFromString(id)
		if err == nil {
			var dev uintptr
			hr := comCall(enum, 5, uintptr(unsafe.Pointer(idUTF16)), uintptr(unsafe.Pointer(&dev)))
			if int32(hr) >= 0 && dev != 0 {
				return dev, true
			}
			log.Printf("audio: device %q not found, falling back to default", id)
		}
	}
	var dev uintptr
	hr := comCall(enum, 4, flow, _eConsole, uintptr(unsafe.Pointer(&dev)))
	return dev, int32(hr) >= 0
}
