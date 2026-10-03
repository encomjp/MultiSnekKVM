package protocol

import (
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

func TestWriteFrameMouseMoveRejectsShortWrite(t *testing.T) {
	err := WriteFrameMouseMove(shortWriter{}, 3, 7)
	if err != io.ErrShortWrite {
		t.Fatalf("WriteFrameMouseMove error = %v, want io.ErrShortWrite", err)
	}
}
