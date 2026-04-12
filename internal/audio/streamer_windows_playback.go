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

func (a *AudioStreamer) currentPlaybackBlockAlign() int {
	a.mu.Lock()
	format := cloneBytes(a.playFmt)
	a.mu.Unlock()
	return blockAlignForFormat(format)
}

func (a *AudioStreamer) SetPlaybackFormat(format []byte) error {
	if _, ok := decodeWaveFormat(format); !ok {
		return fmt.Errorf("invalid audio format payload (%d bytes)", len(format))
	}
	next := cloneBytes(format)

	a.mu.Lock()
	same := bytes.Equal(a.playFmt, next)
	wasPlaying := a.playing
	a.playFmt = next
	a.mu.Unlock()

	if same {
		return nil
	}

	a.playMu.Lock()
	a.playBuf = nil
	a.playMu.Unlock()

	if wasPlaying {
		a.StopPlayback()
	}

	log.Printf("audio playback format set: %s", describeWaveFormat(next))
	return nil
}

func (a *AudioStreamer) StartCapture(sendFn func(protocol.Frame)) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.capturing {
		return nil
	}
	a.capturing = true
	a.capStop = make(chan struct{})
	a.capWg.Add(1)
	go a.captureLoop(sendFn)
	return nil
}

func (a *AudioStreamer) StopCapture() {
	a.mu.Lock()
	if !a.capturing {
		a.mu.Unlock()
		return
	}
	close(a.capStop)
	a.capturing = false
	a.mu.Unlock()
	a.capWg.Wait()
}

func (a *AudioStreamer) StartPlayback() error {
	a.mu.Lock()
	if a.playing {
		a.mu.Unlock()
		return nil
	}
	format := cloneBytes(a.playFmt)
	a.mu.Unlock()
	if len(format) == 0 {
		return fmt.Errorf("audio playback format not received yet")
	}
	bpf := blockAlignForFormat(format)
	if bpf == 0 {
		return fmt.Errorf("invalid audio playback block alignment")
	}

	a.mu.Lock()
	if a.playing {
		a.mu.Unlock()
		return nil
	}
	a.playing = true
	a.playReady = false
	a.mu.Unlock()
	a.playMu.Lock()
	a.playBuf = nil
	a.overflowLogs = 0
	a.playMu.Unlock()
	a.playStop = make(chan struct{})
	a.playWg.Add(1)
	go a.playbackLoop(format)
	return nil
}

func (a *AudioStreamer) StopPlayback() {
	a.mu.Lock()
	if !a.playing {
		a.mu.Unlock()
		return
	}
	close(a.playStop)
	a.playing = false
	a.playReady = false
	a.mu.Unlock()
	a.playWg.Wait()
	a.playMu.Lock()
	a.playBuf = nil
	a.playMu.Unlock()
}

func (a *AudioStreamer) EnqueueAudio(data []byte) {
	if len(data) == 0 {
		return
	}
	profile := a.currentQualityProfileSpec()
	a.mu.Lock()
	format := cloneBytes(a.playFmt)
	ready := a.playReady
	a.mu.Unlock()
	bpf := blockAlignForFormat(format)
	if bpf > 0 && len(data)%bpf != 0 {
		trimmed := len(data) - (len(data) % bpf)
		if trimmed == 0 {
			log.Printf("audio playback: dropped misaligned packet (%d bytes, blockAlign=%d)", len(data), bpf)
			return
		}
		log.Printf("audio playback: trimming misaligned packet from %d to %d bytes", len(data), trimmed)
		data = data[:trimmed]
	}

	a.playMu.Lock()
	defer a.playMu.Unlock()
	a.playBuf = append(a.playBuf, data...)
	maxQueuedBytes := effectiveMaxQueuedAudioBytes(format, profile.MaxBufferedDuration, ready)
	if len(a.playBuf) > maxQueuedBytes {
		var trim int
		a.playBuf, trim = trimQueuedAudio(a.playBuf, bpf, maxQueuedBytes)
		a.overflowLogs++
		if a.overflowLogs <= 3 || a.overflowLogs%500 == 0 {
			log.Printf("audio playback: buffer overflow, trimming %d bytes (count=%d)", trim, a.overflowLogs)
		}
	}
}

func (a *AudioStreamer) IsCapturing() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.capturing
}

func (a *AudioStreamer) IsPlaying() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.playing
}

func (a *AudioStreamer) Close() {
	a.StopCapture()
	a.StopPlayback()
	a.StopMicCapture()
	a.StopMicPlayback()
}

func (a *AudioStreamer) captureLoop(sendFn func(protocol.Frame)) {
	defer a.capWg.Done()
	defer func() {
		a.mu.Lock()
		a.capturing = false
		a.mu.Unlock()
	}()
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	mmcssHandle := enterMmcss("audio capture")
	defer leaveMmcss(mmcssHandle)

	const maxRetries = 5
	retryDelay := 500 * time.Millisecond

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			log.Printf("audio capture: reconnecting (attempt %d/%d)", attempt, maxRetries)
			select {
			case <-a.capStop:
				return
			case <-time.After(retryDelay):
			}
			if retryDelay < 3*time.Second {
				retryDelay *= 2
			}
		}
		if a.captureLoopInner(sendFn) {
			return
		}
	}
	log.Printf("audio capture: giving up after %d reconnection attempts", maxRetries)
}

func (a *AudioStreamer) captureLoopInner(sendFn func(protocol.Frame)) (stopped bool) {
	hr, _, _ := pCoInitializeEx.Call(0, _COINIT_MULTITHREADED)
	if hr != 0 && hr != 1 {
		log.Printf("audio capture: CoInitializeEx: 0x%08x", hr)
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
		log.Printf("audio capture: create enumerator: 0x%08x", hr)
		return
	}
	defer comRelease(enum)

	a.mu.Lock()
	captureDevID := a.captureDeviceID
	a.mu.Unlock()
	dev, ok := getEndpointByID(enum, _eRender, captureDevID)
	if !ok {
		log.Printf("audio capture: get endpoint failed")
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
		log.Printf("audio capture: activate: 0x%08x", hr)
		return
	}
	defer comRelease(client)

	var pwfx uintptr
	hr = comCall(client, 8, uintptr(unsafe.Pointer(&pwfx)))
	if int32(hr) < 0 {
		log.Printf("audio capture: get mix format: 0x%08x", hr)
		return
	}
	defer pCoTaskMemFree.Call(pwfx)

	formatRaw := waveFormatFromPointer(pwfx)
	bpf := blockAlignForFormat(formatRaw)
	if bpf == 0 {
		log.Printf("audio capture: invalid mix format (%d bytes)", len(formatRaw))
		return
	}
	log.Printf("audio capture: %s", describeWaveFormat(formatRaw))
	sendFn(protocol.Frame{Type: protocol.MsgAudioFormat, Payload: formatRaw})

	profile := a.currentQualityProfileSpec()
	if err := initializeSharedAudioClient(client, uintptr(_AUDCLNT_STREAMFLAGS_LOOPBACK), profile.TargetClientDuration, pwfx, "audio capture"); err != nil {
		log.Printf("audio capture: initialize: %v", err)
		return
	}
	silenceKeepaliveInterval := profile.TargetClientDuration
	if silenceKeepaliveInterval <= 0 {
		silenceKeepaliveInterval = 20 * time.Millisecond
	}

	var capClient uintptr
	hr = comCall(client, 14,
		uintptr(unsafe.Pointer(&_IID_IAudioCaptureClient)),
		uintptr(unsafe.Pointer(&capClient)),
	)
	if int32(hr) < 0 {
		log.Printf("audio capture: get capture client: 0x%08x", hr)
		return
	}
	defer comRelease(capClient)

	var capBufSize uint32
	comCall(client, 4, uintptr(unsafe.Pointer(&capBufSize)))

	hr = comCall(client, 10)
	if int32(hr) < 0 {
		log.Printf("audio capture: start: 0x%08x", hr)
		return
	}
	defer comCall(client, 11)

	log.Println("audio capture active (WASAPI loopback)")
	lastEmitAt := time.Now()
	capBufBytes := int(capBufSize) * bpf
	if capBufBytes <= 0 {
		capBufBytes = 19200
	}
	reuseBuf := make([]byte, capBufBytes)

	for {
		select {
		case <-a.capStop:
			return true
		default:
		}

		var pktSize uint32
		hr = comCall(capClient, 5, uintptr(unsafe.Pointer(&pktSize)))
		if int32(hr) < 0 {
			if uint32(hr) == _AUDCLNT_E_DEVICE_INVALIDATED {
				log.Printf("audio capture: device invalidated, stopping")
				return
			}
			time.Sleep(5 * time.Millisecond)
			continue
		}
		if pktSize == 0 {
			if payload := loopbackSilentKeepalivePayload(formatRaw, silenceKeepaliveInterval, lastEmitAt, time.Now()); len(payload) > 0 {
				sendFn(protocol.Frame{Type: protocol.MsgAudioData, Payload: payload})
				lastEmitAt = time.Now()
				continue
			}
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
				log.Printf("audio capture: device invalidated during GetBuffer, stopping")
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
			sendFn(protocol.Frame{Type: protocol.MsgAudioData, Payload: sendPayload})
			lastEmitAt = time.Now()
		}

		comCall(capClient, 4, uintptr(frames))
	}
}

func (a *AudioStreamer) playbackLoop(format []byte) {
	defer a.playWg.Done()
	defer func() {
		a.mu.Lock()
		a.playing = false
		a.playReady = false
		a.mu.Unlock()
	}()
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	mmcssHandle := enterMmcss("audio playback")
	defer leaveMmcss(mmcssHandle)

	bpf := blockAlignForFormat(format)
	if bpf == 0 {
		log.Printf("audio playback: invalid format payload (%d bytes)", len(format))
		return
	}

	const maxRetries = 5
	retryDelay := 500 * time.Millisecond

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			log.Printf("audio playback: reconnecting (attempt %d/%d)", attempt, maxRetries)
			select {
			case <-a.playStop:
				return
			case <-time.After(retryDelay):
			}
			if retryDelay < 3*time.Second {
				retryDelay *= 2
			}
			a.playMu.Lock()
			a.playBuf = nil
			a.playMu.Unlock()
		}
		if a.playbackLoopInner(format, bpf) {
			return
		}
	}
	log.Printf("audio playback: giving up after %d reconnection attempts", maxRetries)
}

func (a *AudioStreamer) playbackLoopInner(format []byte, bpf int) (stopped bool) {
	hr, _, _ := pCoInitializeEx.Call(0, _COINIT_MULTITHREADED)
	if hr != 0 && hr != 1 {
		log.Printf("audio playback: CoInitializeEx: 0x%08x", hr)
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
		log.Printf("audio playback: create enumerator: 0x%08x", hr)
		return
	}
	defer comRelease(enum)

	a.mu.Lock()
	playbackDevID := a.playbackDeviceID
	a.mu.Unlock()
	dev, ok := getEndpointByID(enum, _eRender, playbackDevID)
	if !ok {
		log.Printf("audio playback: get endpoint failed")
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
		log.Printf("audio playback: activate: 0x%08x", hr)
		return
	}
	defer func() { comRelease(client) }()

	var pwfx uintptr
	hr = comCall(client, 8, uintptr(unsafe.Pointer(&pwfx)))
	if int32(hr) < 0 {
		log.Printf("audio playback: get mix format: 0x%08x", hr)
		return
	}
	defer pCoTaskMemFree.Call(pwfx)

	localFormat := waveFormatFromPointer(pwfx)
	localBpf := blockAlignForFormat(localFormat)
	senderFmt, _ := decodeWaveFormat(format)
	localFmtParsed, _ := decodeWaveFormat(localFormat)
	needConvert := !bytes.Equal(format, localFormat)
	autoConverted := false

	log.Printf("audio playback: sender format %s", describeWaveFormat(format))
	log.Printf("audio playback: local  format %s", describeWaveFormat(localFormat))

	profile := a.currentQualityProfileSpec()

	// Try WASAPI auto-conversion with sender format first (higher quality sinc resampler)
	if needConvert {
		senderPwfx := encodeSenderWaveFormat(format)
		if senderPwfx != nil {
			autoFlags := uintptr(_AUDCLNT_STREAMFLAGS_AUTOCONVERTPCM | _AUDCLNT_STREAMFLAGS_SRC_DEFAULT_QUALITY)
			err := initializeSharedAudioClient(client, autoFlags, profile.TargetClientDuration, uintptr(unsafe.Pointer(&senderPwfx[0])), "audio playback (autoconvert)")
			if err == nil {
				log.Printf("audio playback: using WASAPI auto-conversion")
				needConvert = false
				autoConverted = true
				localBpf = bpf
			} else {
				log.Printf("audio playback: WASAPI auto-conversion not available (%v), falling back to manual", err)
				// Need to re-activate client since Initialize failed
				comRelease(client)
				client = 0
				hr = comCall(dev, 3,
					uintptr(unsafe.Pointer(&_IID_IAudioClient)),
					uintptr(_CLSCTX_ALL), 0,
					uintptr(unsafe.Pointer(&client)),
				)
				if int32(hr) < 0 {
					log.Printf("audio playback: re-activate: 0x%08x", hr)
					return
				}
			}
		}
	}

	if !autoConverted {
		if needConvert {
			log.Printf("audio playback: will convert %dHz/%dch/%dbit → %dHz/%dch/%dbit",
				senderFmt.SamplesPerSec, senderFmt.Channels, senderFmt.BitsPerSample,
				localFmtParsed.SamplesPerSec, localFmtParsed.Channels, localFmtParsed.BitsPerSample)
		}
		if err := initializeSharedAudioClient(client, 0, profile.TargetClientDuration, pwfx, "audio playback"); err != nil {
			log.Printf("audio playback: initialize: %v", err)
			return
		}
	}

	if localBpf == 0 {
		localBpf = bpf
	}

	var bufSize uint32
	hr = comCall(client, 4, uintptr(unsafe.Pointer(&bufSize)))
	if int32(hr) < 0 {
		log.Printf("audio playback: get buffer size: 0x%08x", hr)
		return
	}

	var renderClient uintptr
	hr = comCall(client, 14,
		uintptr(unsafe.Pointer(&_IID_IAudioRenderClient)),
		uintptr(unsafe.Pointer(&renderClient)),
	)
	if int32(hr) < 0 {
		log.Printf("audio playback: get render client: 0x%08x", hr)
		return
	}
	defer comRelease(renderClient)

	hr = comCall(client, 10)
	if int32(hr) < 0 {
		log.Printf("audio playback: start: 0x%08x", hr)
		return
	}
	defer comCall(client, 11)

	senderBpf := bpf
	steadyMaxQueuedBytes := maxQueuedAudioBytes(format, profile.MaxBufferedDuration)
	if steadyMaxQueuedBytes > 0 {
		a.playMu.Lock()
		if len(a.playBuf) > steadyMaxQueuedBytes {
			var trim int
			a.playBuf, trim = trimQueuedAudio(a.playBuf, senderBpf, steadyMaxQueuedBytes)
			log.Printf("audio playback: startup backlog trimmed %d bytes before live playback", trim)
		}
		a.playMu.Unlock()
	}
	a.mu.Lock()
	a.playReady = true
	a.mu.Unlock()

	log.Printf("audio playback active (WASAPI, bufSize=%d frames, bpf=%d)", bufSize, localBpf)

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
		case <-a.playStop:
			return true
		default:
		}

		var padding uint32
		hr = comCall(client, 6, uintptr(unsafe.Pointer(&padding)))
		if int32(hr) < 0 {
			if uint32(hr) == _AUDCLNT_E_DEVICE_INVALIDATED {
				log.Printf("audio playback: device invalidated, restarting")
				a.playMu.Lock()
				a.playBuf = nil
				a.playMu.Unlock()
				return false
			}
			padErrCount++
			if padErrCount <= 3 || padErrCount%1000 == 0 {
				log.Printf("audio playback: GetCurrentPadding failed: 0x%08x (count=%d)", hr, padErrCount)
			}
			time.Sleep(5 * time.Millisecond)
			continue
		}

		avail := bufSize - padding
		if avail == 0 || padding > bufSize {
			time.Sleep(2 * time.Millisecond)
			continue
		}

		a.playMu.Lock()
		dataLen := len(a.playBuf)
		a.playMu.Unlock()

		if !primed && dataLen < prebufferBytes {
			if !writeRenderBuffer(renderClient, int(avail), localBpf, nil, _AUDCLNT_BUFFERFLAGS_SILENT, "audio playback") {
				time.Sleep(5 * time.Millisecond)
			}
			continue
		}

		if dataLen == 0 {
			if underrunSince.IsZero() {
				underrunSince = time.Now()
			} else if time.Since(underrunSince) >= reprimeThreshold {
				log.Printf("audio playback: underrun for %v, repriming", time.Since(underrunSince).Round(time.Millisecond))
				primed = false
			}
			if !writeRenderBuffer(renderClient, int(avail), localBpf, nil, _AUDCLNT_BUFFERFLAGS_SILENT, "audio playback") {
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

			a.playMu.Lock()
			if bytesToRead > len(a.playBuf) {
				bytesToRead = len(a.playBuf)
				senderFramesNeeded = bytesToRead / senderBpf
				bytesToRead = senderFramesNeeded * senderBpf
			}
			if senderFramesNeeded == 0 {
				a.playMu.Unlock()
				primed = false
				if !writeRenderBuffer(renderClient, framesRequested, localBpf, nil, _AUDCLNT_BUFFERFLAGS_SILENT, "audio playback") {
					time.Sleep(5 * time.Millisecond)
				}
				continue
			}
			chunk := make([]byte, bytesToRead)
			copy(chunk, a.playBuf[:bytesToRead])
			a.playBuf = a.playBuf[bytesToRead:]
			if len(a.playBuf) == 0 || cap(a.playBuf) > 2*len(a.playBuf) {
				a.playBuf = cloneBytes(a.playBuf)
			}
			a.playMu.Unlock()

			converted := convertAudioPCM(chunk, format, localFormat)
			framesToWrite := len(converted) / localBpf
			if framesToWrite == 0 {
				if !writeRenderBuffer(renderClient, framesRequested, localBpf, nil, _AUDCLNT_BUFFERFLAGS_SILENT, "audio playback") {
					time.Sleep(5 * time.Millisecond)
				}
				continue
			}
			if framesToWrite > framesRequested {
				framesToWrite = framesRequested
			}
			if !writeRenderBuffer(renderClient, framesToWrite, localBpf, converted, 0, "audio playback") {
				time.Sleep(5 * time.Millisecond)
			}
		} else {
			framesRequested := int(avail)
			bytesToRead := framesRequested * localBpf

			a.playMu.Lock()
			if bytesToRead > len(a.playBuf) {
				bytesToRead = len(a.playBuf)
				framesAvailable := bytesToRead / localBpf
				bytesToRead = framesAvailable * localBpf
			}
			if bytesToRead == 0 {
				a.playMu.Unlock()
				primed = false
				if !writeRenderBuffer(renderClient, framesRequested, localBpf, nil, _AUDCLNT_BUFFERFLAGS_SILENT, "audio playback") {
					time.Sleep(5 * time.Millisecond)
				}
				continue
			}
			chunk := make([]byte, bytesToRead)
			copy(chunk, a.playBuf[:bytesToRead])
			a.playBuf = a.playBuf[bytesToRead:]
			if len(a.playBuf) == 0 || cap(a.playBuf) > 2*len(a.playBuf) {
				a.playBuf = cloneBytes(a.playBuf)
			}
			a.playMu.Unlock()

			framesToWrite := bytesToRead / localBpf
			if !writeRenderBuffer(renderClient, framesToWrite, localBpf, chunk, 0, "audio playback") {
				time.Sleep(5 * time.Millisecond)
			}
		}
	}
}
