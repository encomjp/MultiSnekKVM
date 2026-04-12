package protocol

import (
	"bytes"
	"io"
	"runtime"
	"testing"
)

var sink []byte // prevent compiler elision of Encode() results

// ---- Frame write benchmarks ----

// BenchmarkWriteFrameMouseMove measures WriteFrame for a small control frame (13 B total).
func BenchmarkWriteFrameMouseMove(b *testing.B) {
	f := Frame{
		Type:    MsgMouseMove,
		Payload: MouseMoveMsg{DX: 5, DY: -3}.Encode(),
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = WriteFrame(io.Discard, f)
	}
}

// BenchmarkWriteFrameMouseMoveDirect measures WriteFrameMouseMove (no payload allocation).
func BenchmarkWriteFrameMouseMoveDirect(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = WriteFrameMouseMove(io.Discard, 5, -3)
	}
}

// BenchmarkWriteFrameAudio measures WriteFrame for a large audio frame (~4 KB payload).
func BenchmarkWriteFrameAudio(b *testing.B) {
	payload := make([]byte, 4000)
	f := Frame{Type: MsgAudioData, Payload: payload}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = WriteFrame(io.Discard, f)
	}
}

// BenchmarkWriteFrameKey measures WriteFrame for a key event (17 B total).
func BenchmarkWriteFrameKey(b *testing.B) {
	f := Frame{
		Type:    MsgKeyDown,
		Payload: KeyMsg{VKCode: 65, ScanCode: 30, Flags: 0}.Encode(),
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = WriteFrame(io.Discard, f)
	}
}

// ---- Frame read benchmarks ----

// BenchmarkReadFrameMouseMove measures ReadFrame for a small frame.
func BenchmarkReadFrameMouseMove(b *testing.B) {
	var buf bytes.Buffer
	_ = WriteFrame(&buf, Frame{Type: MsgMouseMove, Payload: MouseMoveMsg{DX: 5, DY: -3}.Encode()})
	wire := buf.Bytes()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r := bytes.NewReader(wire)
		_, _ = ReadFrame(r)
	}
}

// BenchmarkReadFrameReuse measures ReadFrame with a recycled reader (no bytes.NewReader alloc).
func BenchmarkReadFrameReuse(b *testing.B) {
	var buf bytes.Buffer
	_ = WriteFrame(&buf, Frame{Type: MsgMouseMove, Payload: MouseMoveMsg{DX: 5, DY: -3}.Encode()})
	wire := buf.Bytes()

	r := bytes.NewReader(wire)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r.Reset(wire)
		_, _ = ReadFrame(r)
	}
}

// BenchmarkReadFrameAudio measures ReadFrame for a large audio frame.
func BenchmarkReadFrameAudio(b *testing.B) {
	var buf bytes.Buffer
	_ = WriteFrame(&buf, Frame{Type: MsgAudioData, Payload: make([]byte, 4000)})
	wire := buf.Bytes()
	r := bytes.NewReader(wire)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r.Reset(wire)
		_, _ = ReadFrame(r)
	}
}

// ---- Encode / decode benchmarks ----

// BenchmarkMouseMoveEncodeEscaped forces heap allocation so the result is realistic.
func BenchmarkMouseMoveEncodeEscaped(b *testing.B) {
	m := MouseMoveMsg{DX: 100, DY: -50}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sink = m.Encode()
	}
	runtime.KeepAlive(sink)
}

func BenchmarkMouseMoveDecode(b *testing.B) {
	raw := MouseMoveMsg{DX: 100, DY: -50}.Encode()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = DecodeMouseMove(raw)
	}
}

func BenchmarkKeyEncodeEscaped(b *testing.B) {
	k := KeyMsg{VKCode: 65, ScanCode: 30, Flags: 0}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sink = k.Encode()
	}
	runtime.KeepAlive(sink)
}

func BenchmarkKeyDecode(b *testing.B) {
	raw := KeyMsg{VKCode: 65, ScanCode: 30, Flags: 0}.Encode()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = DecodeKey(raw)
	}
}

// BenchmarkFrameRoundTrip measures the full encode→write→read→decode cycle for a mouse move.
func BenchmarkFrameRoundTrip(b *testing.B) {
	var buf bytes.Buffer
	r := bytes.NewReader(nil)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.Reset()
		m := MouseMoveMsg{DX: int32(i), DY: int32(-i)}
		f := Frame{Type: MsgMouseMove, Payload: m.Encode()}
		_ = WriteFrame(&buf, f)
		r.Reset(buf.Bytes())
		f2, _ := ReadFrame(r)
		_, _ = DecodeMouseMove(f2.Payload)
	}
}

// BenchmarkFrameRoundTripDirect uses WriteFrameMouseMove for the hot path — no Encode alloc.
func BenchmarkFrameRoundTripDirect(b *testing.B) {
	var buf bytes.Buffer
	r := bytes.NewReader(nil)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.Reset()
		_ = WriteFrameMouseMove(&buf, int32(i), int32(-i))
		r.Reset(buf.Bytes())
		f2, _ := ReadFrame(r)
		_, _ = DecodeMouseMove(f2.Payload)
	}
}
