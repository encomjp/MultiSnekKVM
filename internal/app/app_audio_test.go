package app

import (
	"encoding/binary"
	"testing"

	"multisnekkvm/internal/audio"
	"multisnekkvm/internal/protocol"
)

func pcm16Format(rate, channels int) []byte {
	raw := make([]byte, 18)
	binary.LittleEndian.PutUint16(raw[0:2], 1)
	binary.LittleEndian.PutUint16(raw[2:4], uint16(channels))
	binary.LittleEndian.PutUint32(raw[4:8], uint32(rate))
	binary.LittleEndian.PutUint32(raw[8:12], uint32(rate*channels*2))
	binary.LittleEndian.PutUint16(raw[12:14], uint16(channels*2))
	binary.LittleEndian.PutUint16(raw[14:16], 16)
	return raw
}

func newAudioTestApp(t *testing.T, role, micMode string) *App {
	t.Helper()
	streamer, err := audio.NewAudioStreamer()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(streamer.Close)
	a := &App{audio: streamer, sessionRole: role, micMode: micMode, audioMode: "off"}
	for _, kind := range []audio.StreamKind{audio.StreamDesktop, audio.StreamMic} {
		a.audioIn[kind] = audio.NewInboundStream(kind)
		a.audioOut[kind] = audio.NewOutboundStream(kind)
	}
	return a
}

func TestInboundMicDataPlaysOnlyAfterItsFormat(t *testing.T) {
	a := newAudioTestApp(t, "controlled", "off")
	data := Frame{Type: protocol.MsgMicData, Payload: make([]byte, 9600)} // 100 ms mono

	a.handleInboundAudioFrame(audio.StreamMic, data)
	if s := a.audio.Player(audio.StreamMic).Stats(); s.BufferedMs != 0 {
		t.Fatalf("data before format must be dropped, buffered %d ms", s.BufferedMs)
	}

	a.handleInboundAudioFrame(audio.StreamMic, Frame{Type: protocol.MsgMicTransport, Payload: []byte{0}})
	a.handleInboundAudioFrame(audio.StreamMic, Frame{Type: protocol.MsgMicFormat, Payload: pcm16Format(48000, 1)})
	a.handleInboundAudioFrame(audio.StreamMic, data)
	if s := a.audio.Player(audio.StreamMic).Stats(); s.BufferedMs < 90 {
		t.Fatalf("expected ~100 ms buffered after format, got %d ms", s.BufferedMs)
	}
}

func TestControllerIgnoresStreamsItDidNotRequest(t *testing.T) {
	a := newAudioTestApp(t, "controller", "off")
	a.handleInboundAudioFrame(audio.StreamMic, Frame{Type: protocol.MsgMicTransport, Payload: []byte{0}})
	a.handleInboundAudioFrame(audio.StreamMic, Frame{Type: protocol.MsgMicFormat, Payload: pcm16Format(48000, 1)})
	a.handleInboundAudioFrame(audio.StreamMic, Frame{Type: protocol.MsgMicData, Payload: make([]byte, 4800)})
	if a.audio.IsPlaying(audio.StreamMic) || a.audio.Player(audio.StreamMic).Stats().BufferedMs != 0 {
		t.Fatal("late mic frames after turning the mic off must not restart playback")
	}
	if a.acceptsInboundAudio(audio.StreamDesktop) {
		t.Fatal("controller with audio off must not accept desktop audio")
	}
}

func TestResetAudioStreamsOnlyClearsSessionState(t *testing.T) {
	a := newAudioTestApp(t, "controlled", "off")
	a.handleInboundAudioFrame(audio.StreamMic, Frame{Type: protocol.MsgMicTransport, Payload: []byte{0}})
	a.handleInboundAudioFrame(audio.StreamMic, Frame{Type: protocol.MsgMicFormat, Payload: pcm16Format(48000, 1)})
	a.resetAudioStreams()
	a.handleInboundAudioFrame(audio.StreamMic, Frame{Type: protocol.MsgMicData, Payload: make([]byte, 4800)})
	if a.audio.Player(audio.StreamMic).Stats().BufferedMs != 0 {
		t.Fatal("after a session reset, data must wait for the new session's format")
	}
}
