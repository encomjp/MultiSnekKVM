package protocol

import (
	"bytes"
	"io"
	"testing"
)

type shortWriter struct{}

func (shortWriter) Write(p []byte) (int, error) {
	return len(p) - 1, nil
}

func TestWriteFrameRejectsShortWrite(t *testing.T) {
	err := WriteFrame(shortWriter{}, Frame{Type: MsgHeartbeat})
	if err != io.ErrShortWrite {
		t.Fatalf("WriteFrame error = %v, want io.ErrShortWrite", err)
	}
}

func TestReadFrameLimitRejectsOversizedPayload(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteFrame(&buf, Frame{Type: MsgHello, Payload: make([]byte, 100)}); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadFrameLimit(bytes.NewReader(buf.Bytes()), 99); err == nil {
		t.Fatal("expected oversized frame to be rejected")
	}
	f, err := ReadFrameLimit(bytes.NewReader(buf.Bytes()), 100)
	if err != nil || len(f.Payload) != 100 {
		t.Fatalf("ReadFrameLimit = %d bytes, %v", len(f.Payload), err)
	}
}

func TestWriteFrameDoesNotPoolLargeBuffers(t *testing.T) {
	if err := WriteFrame(io.Discard, Frame{Type: MsgFileChunk, Payload: make([]byte, maxPooledWriteBuffer*2)}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 8; i++ {
		bp := frameWritePool.Get().(*[]byte)
		if cap(*bp) > maxPooledWriteBuffer {
			t.Fatalf("pool retained a %d-byte buffer", cap(*bp))
		}
	}
}
