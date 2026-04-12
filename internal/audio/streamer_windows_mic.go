//go:build windows

package audio

import (
	"bytes"
	"fmt"
	"log"
	"runtime"
	"time"
	"unsafe"

	"multisnekkvm/internal/protocol"
)

func (a *AudioStreamer) StartMicCapture(sendFn func(protocol.Frame)) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.micCapturing {
		return nil
	}
	a.micCapturing = true
	a.micCapStop = make(chan struct{})
	a.micCapWg.Add(1)
	go a.micCaptureLoop(sendFn)
	return nil
}

func (a *AudioStreamer) StopMicCapture() {
	a.mu.Lock()
	if !a.micCapturing {
		a.mu.Unlock()
		return
	}
	close(a.micCapStop)
	a.micCapturing = false
	a.mu.Unlock()
	a.micCapWg.Wait()
}

func (a *AudioStreamer) SetMicPlaybackFormat(format []byte) error {
	if _, ok := decodeWaveFormat(format); !ok {
		return fmt.Errorf("invalid mic format payload (%d bytes)", len(format))
	}
	next := cloneBytes(format)

	a.mu.Lock()
	same := bytes.Equal(a.micPlayFmt, next)
	wasPlaying := a.micPlaying
	a.micPlayFmt = next
	a.mu.Unlock()

	if same {
		return nil
	}

	a.micPlayMu.Lock()
	a.micPlayBuf = nil
	a.micPlayMu.Unlock()

	if wasPlaying {
		a.StopMicPlayback()
	}

	log.Printf("mic playback format set: %s", describeWaveFormat(next))
	return nil
}

func (a *AudioStreamer) StartMicPlayback() error {
	a.mu.Lock()
	if a.micPlaying {
		a.mu.Unlock()
		return nil
	}
	format := cloneBytes(a.micPlayFmt)
	a.mu.Unlock()
	if len(format) == 0 {
		return fmt.Errorf("mic playback format not received yet")
	}
	bpf := blockAlignForFormat(format)
	if bpf == 0 {
		return fmt.Errorf("invalid mic playback block alignment")
	}

	a.mu.Lock()
	if a.micPlaying {
		a.mu.Unlock()
		return nil
	}
	a.micPlaying = true
	a.micPlayReady = false
	a.mu.Unlock()
	a.micPlayMu.Lock()
	a.micPlayBuf = nil
	a.micOverflowLogs = 0
	a.micPlayMu.Unlock()
	a.micPlayStop = make(chan struct{})
	a.micPlayWg.Add(1)
	go a.micPlaybackLoop(format)
	return nil
}

func (a *AudioStreamer) StopMicPlayback() {
	a.mu.Lock()
	if !a.micPlaying {
		a.mu.Unlock()
		return
	}
	close(a.micPlayStop)
	a.micPlaying = false
	a.micPlayReady = false
	a.mu.Unlock()
	a.micPlayWg.Wait()
	a.micPlayMu.Lock()
	a.micPlayBuf = nil
	a.micPlayMu.Unlock()
}

func (a *AudioStreamer) EnqueueMicAudio(data []byte) {
	if len(data) == 0 {
		return
	}
	profile := a.currentQualityProfileSpec()
	a.mu.Lock()
	format := cloneBytes(a.micPlayFmt)
	ready := a.micPlayReady
	a.mu.Unlock()
	bpf := blockAlignForFormat(format)
	if bpf > 0 && len(data)%bpf != 0 {
		trimmed := len(data) - (len(data) % bpf)
		if trimmed == 0 {
			return
		}
		data = data[:trimmed]
	}

	a.micPlayMu.Lock()
	defer a.micPlayMu.Unlock()
	a.micPlayBuf = append(a.micPlayBuf, data...)
	maxQueuedBytes := effectiveMaxQueuedAudioBytes(format, profile.MaxBufferedDuration, ready)
	if len(a.micPlayBuf) > maxQueuedBytes {
		var trim int
		a.micPlayBuf, trim = trimQueuedAudio(a.micPlayBuf, bpf, maxQueuedBytes)
		a.micOverflowLogs++
		if a.micOverflowLogs <= 3 || a.micOverflowLogs%500 == 0 {
			log.Printf("mic playback: buffer overflow, trimming %d bytes (count=%d)", trim, a.micOverflowLogs)
		}
	}
}

func (a *AudioStreamer) IsMicCapturing() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.micCapturing
}

func (a *AudioStreamer) IsMicPlaying() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.micPlaying
}

func (a *AudioStreamer) micCaptureLoop(sendFn func(protocol.Frame)) {
	defer a.micCapWg.Done()
	defer func() {
		a.mu.Lock()
		a.micCapturing = false
		a.mu.Unlock()
	}()
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	mmcssHandle := enterMmcss("mic capture")
	defer leaveMmcss(mmcssHandle)

	const maxRetries = 5
	retryDelay := 500 * time.Millisecond

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			log.Printf("mic capture: reconnecting (attempt %d/%d)", attempt, maxRetries)
			select {
			case <-a.micCapStop:
				return
			case <-time.After(retryDelay):
			}
			if retryDelay < 3*time.Second {
				retryDelay *= 2
			}
		}
		if a.micCaptureLoopInner(sendFn) {
			return
		}
	}
	log.Printf("mic capture: giving up after %d reconnection attempts", maxRetries)
}

func (a *AudioStreamer) micCaptureLoopInner(sendFn func(protocol.Frame)) (stopped bool) {
	hr, _, _ := pCoInitializeEx.Call(0, _COINIT_MULTITHREADED)
	if hr != 0 && hr != 1 {
		log.Printf("mic capture: CoInitializeEx: 0x%08x", hr)
		return
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
		log.Printf("mic capture: create enumerator: 0x%08x", hr)
		return
	}
	defer comRelease(enum)

	a.mu.Lock()
	micDevID := a.micDeviceID
	a.mu.Unlock()
	dev, ok := getEndpointByID(enum, _eCapture, micDevID)
	if !ok {
		log.Printf("mic capture: get endpoint failed")
		return
	}
	defer comRelease(dev)

	var client uintptr
	hr = comCall(dev, 3,
		uintptr(unsafe.Pointer(&_IID_IAudioClient)),
		uintptr(_CLSCTX_ALL), 0,
		uintptr(unsafe.Pointer(&client)),
	)
	if int32(hr) < 0 {
		log.Printf("mic capture: activate: 0x%08x", hr)
		return
	}
	defer comRelease(client)

	var pwfx uintptr
	hr = comCall(client, 8, uintptr(unsafe.Pointer(&pwfx)))
	if int32(hr) < 0 {
		log.Printf("mic capture: get mix format: 0x%08x", hr)
		return
	}
	defer pCoTaskMemFree.Call(pwfx)

	formatRaw := waveFormatFromPointer(pwfx)
	bpf := blockAlignForFormat(formatRaw)
	if bpf == 0 {
		log.Printf("mic capture: invalid mix format (%d bytes)", len(formatRaw))
		return
	}
	log.Printf("mic capture: %s", describeWaveFormat(formatRaw))
	sendFn(protocol.Frame{Type: protocol.MsgMicFormat, Payload: formatRaw})

	profile := a.currentQualityProfileSpec()
	if err := initializeSharedAudioClient(client, 0, profile.TargetClientDuration, pwfx, "mic capture"); err != nil {
		log.Printf("mic capture: initialize: %v", err)
		return
	}

	var capClient uintptr
	hr = comCall(client, 14,
		uintptr(unsafe.Pointer(&_IID_IAudioCaptureClient)),
		uintptr(unsafe.Pointer(&capClient)),
	)
	if int32(hr) < 0 {
		log.Printf("mic capture: get capture client: 0x%08x", hr)
		return
	}
	defer comRelease(capClient)

	var micCapBufSize uint32
	comCall(client, 4, uintptr(unsafe.Pointer(&micCapBufSize)))

	hr = comCall(client, 10)
	if int32(hr) < 0 {
		log.Printf("mic capture: start: 0x%08x", hr)
		return
	}
	defer comCall(client, 11)

	log.Println("mic capture active (WASAPI input device)")
	micCapBufBytes := int(micCapBufSize) * bpf
	if micCapBufBytes <= 0 {
		micCapBufBytes = 19200
	}
	reuseBuf := make([]byte, micCapBufBytes)

	for {
		select {
		case <-a.micCapStop:
			return true
		default:
		}

		var pktSize uint32
		hr = comCall(capClient, 5, uintptr(unsafe.Pointer(&pktSize)))
		if int32(hr) < 0 {
			if uint32(hr) == _AUDCLNT_E_DEVICE_INVALIDATED {
				log.Printf("mic capture: device invalidated, restarting")
				return
			}
			time.Sleep(5 * time.Millisecond)
			continue
		}
		if pktSize == 0 {
			time.Sleep(5 * time.Millisecond)
			continue
		}

		var dataPtr uintptr
		var frames uint32
		var flags uint32
		hr = comCall(capClient, 3,
			uintptr(unsafe.Pointer(&dataPtr)),
			uintptr(unsafe.Pointer(&frames)),
			uintptr(unsafe.Pointer(&flags)),
			0, 0,
		)
		if int32(hr) < 0 {
			if uint32(hr) == _AUDCLNT_E_DEVICE_INVALIDATED {
				log.Printf("mic capture: device invalidated during GetBuffer, restarting")
				return
			}
			time.Sleep(5 * time.Millisecond)
			continue
		}

		if frames > 0 {
			sz := int(frames) * bpf
			if sz > len(reuseBuf) {
				reuseBuf = make([]byte, sz)
			}
			buf := reuseBuf[:sz]
			if dataPtr != 0 && flags&_AUDCLNT_BUFFERFLAGS_SILENT == 0 && flags&_AUDCLNT_BUFFERFLAGS_DATA_DISCONTINUITY == 0 {
				copy(buf, unsafe.Slice((*byte)(unsafe.Pointer(dataPtr)), sz))
			} else {
				clear(buf)
			}
			sendPayload := make([]byte, sz)
			copy(sendPayload, buf)
			sendFn(protocol.Frame{Type: protocol.MsgMicData, Payload: sendPayload})
		}

		comCall(capClient, 4, uintptr(frames))
	}
}

func (a *AudioStreamer) micPlaybackLoop(format []byte) {
	defer a.micPlayWg.Done()
	defer func() {
		a.mu.Lock()
		a.micPlaying = false
		a.micPlayReady = false
		a.mu.Unlock()
	}()
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	mmcssHandle := enterMmcss("mic playback")
	defer leaveMmcss(mmcssHandle)

	bpf := blockAlignForFormat(format)
	if bpf == 0 {
		log.Printf("mic playback: invalid format payload (%d bytes)", len(format))
		return
	}

	const maxRetries = 5
	retryDelay := 500 * time.Millisecond

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			log.Printf("mic playback: reconnecting (attempt %d/%d)", attempt, maxRetries)
			select {
			case <-a.micPlayStop:
				return
			case <-time.After(retryDelay):
			}
			if retryDelay < 3*time.Second {
				retryDelay *= 2
			}
			a.micPlayMu.Lock()
			a.micPlayBuf = nil
			a.micPlayMu.Unlock()
		}
		if a.micPlaybackLoopInner(format, bpf) {
			return
		}
	}
	log.Printf("mic playback: giving up after %d reconnection attempts", maxRetries)
}

func (a *AudioStreamer) micPlaybackLoopInner(format []byte, bpf int) (stopped bool) {
	hr, _, _ := pCoInitializeEx.Call(0, _COINIT_MULTITHREADED)
	if hr != 0 && hr != 1 {
		log.Printf("mic playback: CoInitializeEx: 0x%08x", hr)
		return
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
		log.Printf("mic playback: create enumerator: 0x%08x", hr)
		return
	}
	defer comRelease(enum)

	a.mu.Lock()
	micPbDevID := a.micPlaybackDeviceID
	if micPbDevID == "" {
		micPbDevID = a.playbackDeviceID
	}
	a.mu.Unlock()
	dev, ok := getEndpointByID(enum, _eRender, micPbDevID)
	if !ok {
		log.Printf("mic playback: get endpoint failed")
		return
	}
	defer comRelease(dev)

	var client uintptr
	hr = comCall(dev, 3,
		uintptr(unsafe.Pointer(&_IID_IAudioClient)),
		uintptr(_CLSCTX_ALL), 0,
		uintptr(unsafe.Pointer(&client)),
	)
	if int32(hr) < 0 {
		log.Printf("mic playback: activate: 0x%08x", hr)
		return
	}
	defer func() { comRelease(client) }()

	var pwfx uintptr
	hr = comCall(client, 8, uintptr(unsafe.Pointer(&pwfx)))
	if int32(hr) < 0 {
		log.Printf("mic playback: get mix format: 0x%08x", hr)
		return
	}
	defer pCoTaskMemFree.Call(pwfx)

	localFormat := waveFormatFromPointer(pwfx)
	localBpf := blockAlignForFormat(localFormat)
	senderFmt, _ := decodeWaveFormat(format)
	localFmtParsed, _ := decodeWaveFormat(localFormat)
	needConvert := !bytes.Equal(format, localFormat)
	autoConverted := false

	log.Printf("mic playback: sender format %s", describeWaveFormat(format))
	log.Printf("mic playback: local  format %s", describeWaveFormat(localFormat))

	profile := a.currentQualityProfileSpec()

	if needConvert {
		senderPwfx := encodeSenderWaveFormat(format)
		if senderPwfx != nil {
			autoFlags := uintptr(_AUDCLNT_STREAMFLAGS_AUTOCONVERTPCM | _AUDCLNT_STREAMFLAGS_SRC_DEFAULT_QUALITY)
			err := initializeSharedAudioClient(client, autoFlags, profile.TargetClientDuration, uintptr(unsafe.Pointer(&senderPwfx[0])), "mic playback (autoconvert)")
			if err == nil {
				log.Printf("mic playback: using WASAPI auto-conversion")
				needConvert = false
				autoConverted = true
				localBpf = bpf
			} else {
				log.Printf("mic playback: WASAPI auto-conversion not available (%v), falling back to manual", err)
				comRelease(client)
				client = 0
				hr = comCall(dev, 3,
					uintptr(unsafe.Pointer(&_IID_IAudioClient)),
					uintptr(_CLSCTX_ALL), 0,
					uintptr(unsafe.Pointer(&client)),
				)
				if int32(hr) < 0 {
					log.Printf("mic playback: re-activate: 0x%08x", hr)
					return
				}
			}
		}
	}

	if !autoConverted {
		if needConvert {
			log.Printf("mic playback: will convert %dHz/%dch/%dbit → %dHz/%dch/%dbit",
				senderFmt.SamplesPerSec, senderFmt.Channels, senderFmt.BitsPerSample,
				localFmtParsed.SamplesPerSec, localFmtParsed.Channels, localFmtParsed.BitsPerSample)
		}
		if err := initializeSharedAudioClient(client, 0, profile.TargetClientDuration, pwfx, "mic playback"); err != nil {
			log.Printf("mic playback: initialize: %v", err)
			return
		}
	}

	if localBpf == 0 {
		localBpf = bpf
	}

	var bufSize uint32
	hr = comCall(client, 4, uintptr(unsafe.Pointer(&bufSize)))
	if int32(hr) < 0 {
		log.Printf("mic playback: get buffer size: 0x%08x", hr)
		return
	}

	var renderClient uintptr
	hr = comCall(client, 14,
		uintptr(unsafe.Pointer(&_IID_IAudioRenderClient)),
		uintptr(unsafe.Pointer(&renderClient)),
	)
	if int32(hr) < 0 {
		log.Printf("mic playback: get render client: 0x%08x", hr)
		return
	}
	defer comRelease(renderClient)

	hr = comCall(client, 10)
	if int32(hr) < 0 {
		log.Printf("mic playback: start: 0x%08x", hr)
		return
	}
	defer comCall(client, 11)

	senderBpf := bpf
	steadyMaxQueuedBytes := maxQueuedAudioBytes(format, profile.MaxBufferedDuration)
	if steadyMaxQueuedBytes > 0 {
		a.micPlayMu.Lock()
		if len(a.micPlayBuf) > steadyMaxQueuedBytes {
			var trim int
			a.micPlayBuf, trim = trimQueuedAudio(a.micPlayBuf, senderBpf, steadyMaxQueuedBytes)
			log.Printf("mic playback: startup backlog trimmed %d bytes before live playback", trim)
		}
		a.micPlayMu.Unlock()
	}
	a.mu.Lock()
	a.micPlayReady = true
	a.mu.Unlock()

	log.Printf("mic playback active (WASAPI, bufSize=%d frames, bpf=%d)", bufSize, localBpf)

	prebufferBytes := queuedAudioBytesForDuration(format, profile.PrebufferDuration)
	if prebufferBytes == 0 {
		prebufferBytes = senderBpf
	}

	padErrCount := 0
	primed := false
	var underrunSince time.Time
	reprimeThreshold := profile.ReprimeThreshold
	for {
		select {
		case <-a.micPlayStop:
			return true
		default:
		}

		var padding uint32
		hr = comCall(client, 6, uintptr(unsafe.Pointer(&padding)))
		if int32(hr) < 0 {
			if uint32(hr) == _AUDCLNT_E_DEVICE_INVALIDATED {
				log.Printf("mic playback: device invalidated, restarting")
				a.micPlayMu.Lock()
				a.micPlayBuf = nil
				a.micPlayMu.Unlock()
				return false
			}
			padErrCount++
			if padErrCount <= 3 || padErrCount%1000 == 0 {
				log.Printf("mic playback: GetCurrentPadding failed: 0x%08x (count=%d)", hr, padErrCount)
			}
			time.Sleep(5 * time.Millisecond)
			continue
		}

		avail := bufSize - padding
		if avail == 0 || padding > bufSize {
			time.Sleep(2 * time.Millisecond)
			continue
		}

		a.micPlayMu.Lock()
		dataLen := len(a.micPlayBuf)
		a.micPlayMu.Unlock()

		if !primed && dataLen < prebufferBytes {
			if !writeRenderBuffer(renderClient, int(avail), localBpf, nil, _AUDCLNT_BUFFERFLAGS_SILENT, "mic playback") {
				time.Sleep(5 * time.Millisecond)
			}
			continue
		}

		if dataLen == 0 {
			if underrunSince.IsZero() {
				underrunSince = time.Now()
			} else if time.Since(underrunSince) >= reprimeThreshold {
				log.Printf("mic playback: underrun for %v, repriming", time.Since(underrunSince).Round(time.Millisecond))
				primed = false
			}
			if !writeRenderBuffer(renderClient, int(avail), localBpf, nil, _AUDCLNT_BUFFERFLAGS_SILENT, "mic playback") {
				time.Sleep(5 * time.Millisecond)
			}
			continue
		}

		primed = true
		underrunSince = time.Time{}

		if needConvert {
			framesRequested := int(avail)
			senderFramesNeeded := int(int64(framesRequested) * int64(senderFmt.SamplesPerSec) / int64(localFmtParsed.SamplesPerSec))
			if senderFramesNeeded == 0 {
				senderFramesNeeded = 1
			}
			bytesToRead := senderFramesNeeded * senderBpf

			a.micPlayMu.Lock()
			if bytesToRead > len(a.micPlayBuf) {
				bytesToRead = len(a.micPlayBuf)
				senderFramesNeeded = bytesToRead / senderBpf
				bytesToRead = senderFramesNeeded * senderBpf
			}
			if senderFramesNeeded == 0 {
				a.micPlayMu.Unlock()
				primed = false
				if !writeRenderBuffer(renderClient, framesRequested, localBpf, nil, _AUDCLNT_BUFFERFLAGS_SILENT, "mic playback") {
					time.Sleep(5 * time.Millisecond)
				}
				continue
			}
			chunk := make([]byte, bytesToRead)
			copy(chunk, a.micPlayBuf[:bytesToRead])
			a.micPlayBuf = a.micPlayBuf[bytesToRead:]
			if len(a.micPlayBuf) == 0 || cap(a.micPlayBuf) > 2*len(a.micPlayBuf) {
				a.micPlayBuf = cloneBytes(a.micPlayBuf)
			}
			a.micPlayMu.Unlock()

			converted := convertAudioPCM(chunk, format, localFormat)
			framesToWrite := len(converted) / localBpf
			if framesToWrite == 0 {
				if !writeRenderBuffer(renderClient, framesRequested, localBpf, nil, _AUDCLNT_BUFFERFLAGS_SILENT, "mic playback") {
					time.Sleep(5 * time.Millisecond)
				}
				continue
			}
			if framesToWrite > framesRequested {
				framesToWrite = framesRequested
			}
			if !writeRenderBuffer(renderClient, framesToWrite, localBpf, converted, 0, "mic playback") {
				time.Sleep(5 * time.Millisecond)
			}
		} else {
			framesRequested := int(avail)
			bytesToRead := framesRequested * localBpf

			a.micPlayMu.Lock()
			if bytesToRead > len(a.micPlayBuf) {
				bytesToRead = len(a.micPlayBuf)
				framesAvailable := bytesToRead / localBpf
				bytesToRead = framesAvailable * localBpf
			}
			if bytesToRead == 0 {
				a.micPlayMu.Unlock()
				primed = false
				if !writeRenderBuffer(renderClient, framesRequested, localBpf, nil, _AUDCLNT_BUFFERFLAGS_SILENT, "mic playback") {
					time.Sleep(5 * time.Millisecond)
				}
				continue
			}
			chunk := make([]byte, bytesToRead)
			copy(chunk, a.micPlayBuf[:bytesToRead])
			a.micPlayBuf = a.micPlayBuf[bytesToRead:]
			if len(a.micPlayBuf) == 0 || cap(a.micPlayBuf) > 2*len(a.micPlayBuf) {
				a.micPlayBuf = cloneBytes(a.micPlayBuf)
			}
			a.micPlayMu.Unlock()

			framesToWrite := bytesToRead / localBpf
			if !writeRenderBuffer(renderClient, framesToWrite, localBpf, chunk, 0, "mic playback") {
				time.Sleep(5 * time.Millisecond)
			}
		}
	}
}
