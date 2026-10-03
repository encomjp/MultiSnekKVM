package app

import (
	"testing"

	"multisnekkvm/internal/protocol"
)

func TestAudioLaneDropsOldestDataFrameAndKeepsControl(t *testing.T) {
	var l audioLane
	l.push(Frame{Type: protocol.MsgAudioFormat})
	for i := 1; i < muxAudioCap; i++ {
		l.push(Frame{Type: protocol.MsgAudioData, Payload: []byte{byte(i)}})
	}
	if dropped := l.push(Frame{Type: protocol.MsgMicData, Payload: []byte{0xFF}}); dropped != 1 {
		t.Fatalf("dropped = %d, want 1", dropped)
	}
	if l.len() != muxAudioCap {
		t.Fatalf("len = %d", l.len())
	}
	first, _ := l.pop()
	if first.Type != protocol.MsgAudioFormat {
		t.Fatalf("format frame must survive and stay first, got 0x%02x", first.Type)
	}
	second, _ := l.pop()
	if second.Payload[0] != 2 {
		t.Fatalf("oldest data frame (1) must be dropped, next is %d", second.Payload[0])
	}
}

func TestAudioLaneNeverDropsControlFrames(t *testing.T) {
	var l audioLane
	for i := 0; i < muxAudioCap; i++ {
		l.push(Frame{Type: protocol.MsgMicTransport})
	}
	if dropped := l.push(Frame{Type: protocol.MsgAudioData}); dropped != 1 || l.len() != muxAudioCap {
		t.Fatalf("incoming data frame should be dropped when lane holds only control frames (dropped=%d len=%d)", dropped, l.len())
	}
	if dropped := l.push(Frame{Type: protocol.MsgAudioFormat}); dropped != 0 || l.len() != muxAudioCap+1 {
		t.Fatalf("control frame must be kept (dropped=%d len=%d)", dropped, l.len())
	}
}

func TestEnqueueSendRoutesAudioToAudioLane(t *testing.T) {
	a := &App{muxHigh: make(chan Frame, 4), muxMouse: make(chan Frame, 2), muxFile: make(chan Frame, 1)}
	for _, typ := range []byte{protocol.MsgAudioTransport, protocol.MsgAudioFormat, protocol.MsgAudioData,
		protocol.MsgMicTransport, protocol.MsgMicFormat, protocol.MsgMicData} {
		a.enqueueSend(Frame{Type: typ})
	}
	a.enqueueSend(Frame{Type: protocol.MsgAudioStart})
	a.enqueueSend(Frame{Type: protocol.MsgMicStop})
	if a.muxAudio.len() != 6 {
		t.Fatalf("audio lane len = %d, want 6", a.muxAudio.len())
	}
	if len(a.muxHigh) != 2 {
		t.Fatalf("start/stop requests must stay in the high lane, got %d", len(a.muxHigh))
	}
}

func TestEnqueueMouseMoveCoalescesDeltas(t *testing.T) {
	a := &App{muxMouse: make(chan Frame, 2)}
	for i := 0; i < 5; i++ {
		a.enqueueSend(Frame{Type: protocol.MsgMouseMove, Payload: protocol.MouseMoveMsg{DX: 1, DY: -2}.Encode()})
	}
	var dx, dy int32
	for len(a.muxMouse) > 0 {
		m, err := protocol.DecodeMouseMove((<-a.muxMouse).Payload)
		if err != nil {
			t.Fatal(err)
		}
		dx += m.DX
		dy += m.DY
	}
	if dx != 5 || dy != -10 {
		t.Fatalf("total displacement = (%d,%d), want (5,-10)", dx, dy)
	}
}
