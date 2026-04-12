package protocol

import (
	"encoding/binary"
	"fmt"
	"io"
	"strings"
	"sync"
)

// MaxFramePayloadBytes is the maximum allowed payload size in a frame.
const MaxFramePayloadBytes = 1 << 20

// Message types
const (
	MsgHello          byte = 0x01
	MsgEdgeConfig     byte = 0x02
	MsgHeartbeat      byte = 0x03
	MsgMouseMove      byte = 0x10
	MsgMouseClick     byte = 0x11
	MsgMouseScroll    byte = 0x12
	MsgKeyDown        byte = 0x20
	MsgKeyUp          byte = 0x21
	MsgClipboard      byte = 0x30
	MsgSwitchBack     byte = 0x40
	MsgAudioStart     byte = 0x50
	MsgAudioStop      byte = 0x51
	MsgAudioData      byte = 0x52
	MsgAudioFormat    byte = 0x53
	MsgMicStart       byte = 0x54
	MsgMicStop        byte = 0x55
	MsgMicData        byte = 0x56
	MsgMicFormat      byte = 0x57
	MsgAudioTransport byte = 0x58
	MsgMicTransport   byte = 0x59

	// Latency measurement
	MsgPing byte = 0x04
	MsgPong byte = 0x05

	// File transfer
	MsgFileTransferOffer  byte = 0x60
	MsgFileTransferAccept byte = 0x61
	MsgFileChunk          byte = 0x62
	MsgFileTransferDone   byte = 0x63
	MsgFileTransferCancel byte = 0x64

	// Unicode text input (layout-independent character injection)
	MsgUnicodeText byte = 0x22
)

// Frame: [1 byte type][4 bytes length][payload]
type Frame struct {
	Type    byte
	Payload []byte
}

// frameWritePool recycles frame-write buffers to reduce allocations on the
// hot send path (mouse moves at 125 Hz, audio frames at 50 Hz).
// Pool stores *[]byte (a pointer), not []byte (a 3-word slice header).
// Pointers fit inline in interface{} so Pool.Put causes zero boxing allocs.
// Capacity 256 covers all control frames; audio frames grow the buffer on
// first use and that larger backing array is then recycled too.
var frameWritePool = sync.Pool{
	New: func() any { b := make([]byte, 0, 256); return &b },
}

func WriteFrame(w io.Writer, f Frame) error {
	// Combine header and payload into a single buffer so that the underlying
	// Write call (→ TLS record → TCP segment) is one syscall, not two.
	// With TCP_NODELAY enabled this avoids a second TCP segment for the header.
	size := 5 + len(f.Payload)
	bp := frameWritePool.Get().(*[]byte)
	buf := *bp
	if cap(buf) < size {
		buf = make([]byte, size)
	} else {
		buf = buf[:size]
	}
	buf[0] = f.Type
	binary.BigEndian.PutUint32(buf[1:], uint32(len(f.Payload)))
	copy(buf[5:], f.Payload)
	_, err := w.Write(buf)
	*bp = buf[:0]
	frameWritePool.Put(bp)
	return err
}

// WriteFrameMouseMove writes a MsgMouseMove frame directly from DX/DY values,
// encoding into the pool buffer in one step and avoiding the MouseMoveMsg.Encode()
// intermediate allocation. Use this on the hot send path instead of WriteFrame.
func WriteFrameMouseMove(w io.Writer, dx, dy int32) error {
	const size = 5 + 8 // 1 type + 4 length + 4 DX + 4 DY = 13 bytes
	bp := frameWritePool.Get().(*[]byte)
	buf := *bp
	if cap(buf) < size {
		buf = make([]byte, size)
	} else {
		buf = buf[:size]
	}
	buf[0] = MsgMouseMove
	binary.BigEndian.PutUint32(buf[1:], 8) // payload length
	binary.BigEndian.PutUint32(buf[5:], uint32(dx))
	binary.BigEndian.PutUint32(buf[9:], uint32(dy))
	_, err := w.Write(buf)
	*bp = buf[:0]
	frameWritePool.Put(bp)
	return err
}

// frameReadHeaderPool recycles the 5-byte header buffer used by ReadFrame.
var frameReadHeaderPool = sync.Pool{
	New: func() any { b := [5]byte{}; return &b },
}

func ReadFrame(r io.Reader) (Frame, error) {
	hp := frameReadHeaderPool.Get().(*[5]byte)
	_, err := io.ReadFull(r, hp[:])
	if err != nil {
		frameReadHeaderPool.Put(hp)
		return Frame{}, err
	}
	length := binary.BigEndian.Uint32(hp[1:])
	msgType := hp[0]
	frameReadHeaderPool.Put(hp)

	if length > MaxFramePayloadBytes {
		return Frame{}, fmt.Errorf("frame too large: %d", length)
	}
	payload := make([]byte, length)
	if length > 0 {
		if _, err := io.ReadFull(r, payload); err != nil {
			return Frame{}, err
		}
	}
	return Frame{Type: msgType, Payload: payload}, nil
}

// MouseMove: [4 bytes DX][4 bytes DY] (relative deltas)
type MouseMoveMsg struct {
	DX, DY int32
}

func (m MouseMoveMsg) Encode() []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint32(b[0:], uint32(m.DX))
	binary.BigEndian.PutUint32(b[4:], uint32(m.DY))
	return b
}

func DecodeMouseMove(b []byte) (MouseMoveMsg, error) {
	if len(b) != 8 {
		return MouseMoveMsg{}, fmt.Errorf("mouse move payload length=%d, want 8", len(b))
	}
	return MouseMoveMsg{
		DX: int32(binary.BigEndian.Uint32(b[0:])),
		DY: int32(binary.BigEndian.Uint32(b[4:])),
	}, nil
}

// MouseClick: [1 byte button][1 byte pressed]
type MouseClickMsg struct {
	Button  byte // 0=left, 1=right, 2=middle
	Pressed bool
}

func (m MouseClickMsg) Encode() []byte {
	b := make([]byte, 2)
	b[0] = m.Button
	if m.Pressed {
		b[1] = 1
	}
	return b
}

func DecodeMouseClick(b []byte) (MouseClickMsg, error) {
	if len(b) != 2 {
		return MouseClickMsg{}, fmt.Errorf("mouse click payload length=%d, want 2", len(b))
	}
	return MouseClickMsg{Button: b[0], Pressed: b[1] == 1}, nil
}

// MouseScroll: [4 bytes delta]
type MouseScrollMsg struct {
	Delta int32
}

func (m MouseScrollMsg) Encode() []byte {
	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b[0:], uint32(m.Delta))
	return b
}

func DecodeMouseScroll(b []byte) (MouseScrollMsg, error) {
	if len(b) != 4 {
		return MouseScrollMsg{}, fmt.Errorf("mouse scroll payload length=%d, want 4", len(b))
	}
	return MouseScrollMsg{Delta: int32(binary.BigEndian.Uint32(b[0:]))}, nil
}

// Key: [4 bytes vkCode][4 bytes scanCode][4 bytes flags]
type KeyMsg struct {
	VKCode   uint32
	ScanCode uint32
	Flags    uint32
}

func (m KeyMsg) Encode() []byte {
	b := make([]byte, 12)
	binary.BigEndian.PutUint32(b[0:], m.VKCode)
	binary.BigEndian.PutUint32(b[4:], m.ScanCode)
	binary.BigEndian.PutUint32(b[8:], m.Flags)
	return b
}

func DecodeKey(b []byte) (KeyMsg, error) {
	if len(b) != 12 {
		return KeyMsg{}, fmt.Errorf("key payload length=%d, want 12", len(b))
	}
	return KeyMsg{
		VKCode:   binary.BigEndian.Uint32(b[0:]),
		ScanCode: binary.BigEndian.Uint32(b[4:]),
		Flags:    binary.BigEndian.Uint32(b[8:]),
	}, nil
}

// Clipboard: raw UTF-8 text (legacy)
type ClipboardMsg struct {
	Text string
}

func (m ClipboardMsg) Encode() []byte {
	return []byte(m.Text)
}

func DecodeClipboard(b []byte) ClipboardMsg {
	return ClipboardMsg{Text: string(b)}
}

// UnicodeText: [4 bytes char] — layout-independent character injection.
// Used when the controller has a different keyboard layout than the host.
// Only sent for printable characters with no Ctrl/Alt/Win modifier held.
type UnicodeTextMsg struct {
	Char uint32
}

func (m UnicodeTextMsg) Encode() []byte {
	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b, m.Char)
	return b
}

func DecodeUnicodeText(b []byte) (UnicodeTextMsg, error) {
	if len(b) != 4 {
		return UnicodeTextMsg{}, fmt.Errorf("unicode text payload length=%d, want 4", len(b))
	}
	return UnicodeTextMsg{Char: binary.BigEndian.Uint32(b)}, nil
}

// ClipboardSync: [4 bytes seq][UTF-8 text]
type ClipboardSyncMsg struct {
	Seq  uint32
	Text string
}

func (m ClipboardSyncMsg) Encode() []byte {
	b := make([]byte, 4+len(m.Text))
	binary.BigEndian.PutUint32(b[0:], m.Seq)
	copy(b[4:], m.Text)
	return b
}

func DecodeClipboardSync(b []byte) (ClipboardSyncMsg, error) {
	if len(b) < 4 {
		return ClipboardSyncMsg{}, fmt.Errorf("clipboard sync payload too short: %d", len(b))
	}
	return ClipboardSyncMsg{
		Seq:  binary.BigEndian.Uint32(b[0:]),
		Text: string(b[4:]),
	}, nil
}

// Hello: [device_id string + \n + name string + \n + fingerprint string [+ \n + pairing_code]]
type HelloMsg struct {
	DeviceID    string
	Name        string
	Fingerprint string
	PairingCode string
}

func (m HelloMsg) Encode() []byte {
	if m.PairingCode != "" {
		return []byte(m.DeviceID + "\n" + m.Name + "\n" + m.Fingerprint + "\n" + m.PairingCode)
	}
	return []byte(m.DeviceID + "\n" + m.Name + "\n" + m.Fingerprint)
}

func DecodeHello(b []byte) (HelloMsg, error) {
	parts := strings.SplitN(string(b), "\n", 4)
	if len(parts) != 3 && len(parts) != 4 {
		return HelloMsg{}, fmt.Errorf("hello payload malformed")
	}
	h := HelloMsg{}
	if len(parts) > 0 {
		h.DeviceID = parts[0]
	}
	if len(parts) > 1 {
		h.Name = parts[1]
	}
	if len(parts) > 2 {
		h.Fingerprint = parts[2]
	}
	if len(parts) > 3 {
		h.PairingCode = parts[3]
	}
	return h, nil
}

// EdgeConfig: each peer advertises which local edge starts a control handoff
type EdgeConfigMsg struct {
	EdgeSide string
}

func (m EdgeConfigMsg) Encode() []byte {
	return []byte(m.EdgeSide)
}

func DecodeEdgeConfig(b []byte) EdgeConfigMsg {
	return EdgeConfigMsg{EdgeSide: string(b)}
}

// Ping: [8 bytes timestamp nanos]
type PingMsg struct {
	TimestampNano uint64
}

func (m PingMsg) Encode() []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, m.TimestampNano)
	return b
}

func DecodePing(b []byte) (PingMsg, error) {
	if len(b) != 8 {
		return PingMsg{}, fmt.Errorf("ping payload length=%d, want 8", len(b))
	}
	return PingMsg{TimestampNano: binary.BigEndian.Uint64(b)}, nil
}
