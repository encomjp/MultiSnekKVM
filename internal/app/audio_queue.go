package app

import (
	"log"
	"sync/atomic"
	"time"

	"multisnekkvm/internal/audio"
	"multisnekkvm/internal/protocol"
)

const realtimeAudioQueueCapacity = 16

type queuedRealtimeFrame struct {
	generation uint64
	frame      protocol.Frame
}

func (a *App) initRealtimeAudioQueue() {
	if a.realtimeSendCh != nil {
		return
	}
	a.realtimeSendCh = make(chan queuedRealtimeFrame, realtimeAudioQueueCapacity)
	a.realtimeSendWg.Add(1)
	go a.realtimeSendLoop()
}

func (a *App) realtimeSendLoop() {
	defer a.realtimeSendWg.Done()
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	var lastDropCount uint64
	for {
		select {
		case <-a.ctx.Done():
			return
		case <-ticker.C:
			current := atomic.LoadUint64(&a.realtimeDropCount)
			delta := current - lastDropCount
			lastDropCount = current
			if delta > 0 {
				log.Printf("audio queue: dropped %d frame(s) in last 30s (total=%d, depth=%d/%d)",
					delta, current, len(a.realtimeSendCh), cap(a.realtimeSendCh))
			}
		case item := <-a.realtimeSendCh:
			if item.generation != a.currentRealtimeSendGeneration() {
				continue
			}
			a.enqueueSend(item.frame)
		}
	}
}

func (a *App) currentRealtimeSendGeneration() uint64 {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.realtimeSendGeneration
}

func (a *App) bumpRealtimeSendGeneration() {
	a.mu.Lock()
	a.realtimeSendGeneration++
	a.mu.Unlock()
}

func (a *App) queueRealtimeFrame(frame protocol.Frame) {
	if a.realtimeSendCh == nil {
		return
	}
	generation := a.currentRealtimeSendGeneration()
	if generation == 0 {
		return
	}
	queued := queuedRealtimeFrame{generation: generation, frame: frame}
	select {
	case a.realtimeSendCh <- queued:
	default:
		select {
		case <-a.realtimeSendCh:
			atomic.AddUint64(&a.realtimeDropCount, 1)
		default:
		}
		select {
		case a.realtimeSendCh <- queued:
		default:
		}
	}
}

func (a *App) handleCapturedAudioFrame(frame protocol.Frame) {
	transportMode, profile := a.currentAudioTransportSettings()
	if a.audioOutbound == nil {
		if frame.Type == protocol.MsgAudioFormat {
			a.enqueueSend(frame)
			return
		}
		if frame.Type == protocol.MsgAudioData {
			a.queueRealtimeFrame(frame)
		}
		return
	}

	switch frame.Type {
	case protocol.MsgAudioFormat:
		frames, err := a.audioOutbound.Configure(transportMode, profile, frame.Payload)
		if err != nil {
			log.Printf("audio transport configure failed, falling back to PCM: %v", err)
			frames, err = a.audioOutbound.Configure(audio.TransportPCM, profile, frame.Payload)
			if err != nil {
				log.Printf("audio transport fallback failed: %v", err)
				return
			}
		}
		for _, outgoing := range frames {
			a.enqueueSend(outgoing)
		}
	case protocol.MsgAudioData:
		frames, err := a.audioOutbound.ProcessData(frame.Payload)
		if err != nil {
			log.Printf("audio transport encode failed: %v", err)
			return
		}
		for _, outgoing := range frames {
			a.queueRealtimeFrame(outgoing)
		}
	}
}

func (a *App) handleCapturedMicFrame(frame protocol.Frame) {
	transportMode, profile := a.currentAudioTransportSettings()
	if a.micOutbound == nil {
		if frame.Type == protocol.MsgMicFormat {
			a.enqueueSend(frame)
			return
		}
		if frame.Type == protocol.MsgMicData {
			a.queueRealtimeFrame(frame)
		}
		return
	}

	switch frame.Type {
	case protocol.MsgMicFormat:
		frames, err := a.micOutbound.Configure(transportMode, profile, frame.Payload)
		if err != nil {
			log.Printf("mic transport configure failed, falling back to PCM: %v", err)
			frames, err = a.micOutbound.Configure(audio.TransportPCM, profile, frame.Payload)
			if err != nil {
				log.Printf("mic transport fallback failed: %v", err)
				return
			}
		}
		for _, outgoing := range frames {
			a.enqueueSend(outgoing)
		}
	case protocol.MsgMicData:
		frames, err := a.micOutbound.ProcessData(frame.Payload)
		if err != nil {
			log.Printf("mic transport encode failed: %v", err)
			return
		}
		for _, outgoing := range frames {
			a.queueRealtimeFrame(outgoing)
		}
	}
}