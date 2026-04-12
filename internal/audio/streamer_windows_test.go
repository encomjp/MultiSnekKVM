//go:build windows

package audio

import (
	"encoding/binary"
	"math"
	"testing"
	"time"
)

func testPCM16WaveFormat(sampleRate, channels int) []byte {
	raw := make([]byte, waveFormatExSize)
	binary.LittleEndian.PutUint16(raw[0:2], _waveFormatPcm)
	binary.LittleEndian.PutUint16(raw[2:4], uint16(channels))
	binary.LittleEndian.PutUint32(raw[4:8], uint32(sampleRate))
	blockAlign := uint16(channels * 2)
	binary.LittleEndian.PutUint32(raw[8:12], uint32(sampleRate)*uint32(blockAlign))
	binary.LittleEndian.PutUint16(raw[12:14], blockAlign)
	binary.LittleEndian.PutUint16(raw[14:16], 16)
	return raw
}

func testFloat32WaveFormat(sampleRate, channels int) []byte {
	raw := make([]byte, waveFormatExSize)
	binary.LittleEndian.PutUint16(raw[0:2], _waveFormatIEEEFloat)
	binary.LittleEndian.PutUint16(raw[2:4], uint16(channels))
	binary.LittleEndian.PutUint32(raw[4:8], uint32(sampleRate))
	blockAlign := uint16(channels * 4)
	binary.LittleEndian.PutUint32(raw[8:12], uint32(sampleRate)*uint32(blockAlign))
	binary.LittleEndian.PutUint16(raw[12:14], blockAlign)
	binary.LittleEndian.PutUint16(raw[14:16], 32)
	return raw
}

func TestLoopbackSilentKeepalivePayloadWaitsForInterval(t *testing.T) {
	format := testPCM16WaveFormat(48000, 2)
	lastEmitAt := time.Unix(100, 0)
	now := lastEmitAt.Add(10 * time.Millisecond)

	if payload := loopbackSilentKeepalivePayload(format, 20*time.Millisecond, lastEmitAt, now); payload != nil {
		t.Fatalf("expected no keepalive before interval, got %d bytes", len(payload))
	}
}

func TestLoopbackSilentKeepalivePayloadBuildsAlignedSilence(t *testing.T) {
	format := testPCM16WaveFormat(48000, 2)
	lastEmitAt := time.Unix(100, 0)
	now := lastEmitAt.Add(25 * time.Millisecond)

	payload := loopbackSilentKeepalivePayload(format, 20*time.Millisecond, lastEmitAt, now)
	if len(payload) != 3840 {
		t.Fatalf("expected 3840-byte keepalive payload, got %d", len(payload))
	}
	for i, sample := range payload {
		if sample != 0 {
			t.Fatalf("expected zeroed payload, got byte %d=%d", i, sample)
		}
	}
}

func TestLoopbackSilentKeepalivePayloadRejectsInvalidFormat(t *testing.T) {
	lastEmitAt := time.Unix(100, 0)
	now := lastEmitAt.Add(25 * time.Millisecond)

	if payload := loopbackSilentKeepalivePayload([]byte{1, 2, 3}, 20*time.Millisecond, lastEmitAt, now); payload != nil {
		t.Fatalf("expected invalid format to skip keepalive, got %d bytes", len(payload))
	}
}

func TestConvertAudioPCMConvertsPCM16ToFloat32(t *testing.T) {
	srcFmt := testPCM16WaveFormat(48000, 2)
	dstFmt := testFloat32WaveFormat(48000, 2)
	src := make([]byte, 4)
	left := int16(16384)
	right := int16(-16384)
	binary.LittleEndian.PutUint16(src[0:2], uint16(left))
	binary.LittleEndian.PutUint16(src[2:4], uint16(right))

	converted := convertAudioPCM(src, srcFmt, dstFmt)
	if len(converted) != 8 {
		t.Fatalf("converted len = %d, want 8", len(converted))
	}
	leftSample := math.Float32frombits(binary.LittleEndian.Uint32(converted[0:4]))
	rightSample := math.Float32frombits(binary.LittleEndian.Uint32(converted[4:8]))
	if leftSample < 0.49 || leftSample > 0.51 {
		t.Fatalf("left sample = %f, want about 0.5", leftSample)
	}
	if rightSample > -0.49 || rightSample < -0.51 {
		t.Fatalf("right sample = %f, want about -0.5", rightSample)
	}
}

func TestEffectiveMaxQueuedAudioBytesUsesStartupGraceBeforePlaybackReady(t *testing.T) {
	format := testPCM16WaveFormat(48000, 2)
	steady := maxQueuedAudioBytes(format, 100*time.Millisecond)
	startup := effectiveMaxQueuedAudioBytes(format, 100*time.Millisecond, false)
	ready := effectiveMaxQueuedAudioBytes(format, 100*time.Millisecond, true)

	if steady != 19200 {
		t.Fatalf("steady max = %d, want 19200", steady)
	}
	if startup != 96000 {
		t.Fatalf("startup max = %d, want 96000", startup)
	}
	if ready != steady {
		t.Fatalf("ready max = %d, want %d", ready, steady)
	}
}

func TestReadNormalizedSampleBoundsCheck(t *testing.T) {
	format := testPCM16WaveFormat(48000, 2)
	wfx, _ := decodeWaveFormat(format)
	// frame=0, channel=0, but buffer only has 2 bytes (need 4 for stereo PCM16)
	sample := readNormalizedSample([]byte{0, 0}, 0, 1, wfx, audioSampleCodecPCM16)
	if sample != 0 {
		t.Fatalf("out-of-bounds read should return 0, got %f", sample)
	}
	// frame 100 in a 4-byte buffer
	sample = readNormalizedSample([]byte{0, 0, 0, 0}, 100, 0, wfx, audioSampleCodecPCM16)
	if sample != 0 {
		t.Fatalf("out-of-bounds frame should return 0, got %f", sample)
	}
}

func TestWriteNormalizedSampleBoundsCheck(t *testing.T) {
	format := testPCM16WaveFormat(48000, 2)
	wfx, _ := decodeWaveFormat(format)
	// This should not panic - writing out of bounds should be a no-op
	dst := make([]byte, 2)
	writeNormalizedSample(dst, 0, 1, wfx, audioSampleCodecPCM16, 0.5)
	// Verify the buffer wasn't corrupted
	if dst[0] != 0 || dst[1] != 0 {
		t.Fatal("out-of-bounds write should not modify buffer")
	}
}

func TestConvertAudioPCMNearestRejectsBitDepthMismatch(t *testing.T) {
	srcFmt := testPCM16WaveFormat(48000, 2)
	dstFmt := testFloat32WaveFormat(48000, 2)
	srcWfx, _ := decodeWaveFormat(srcFmt)
	dstWfx, _ := decodeWaveFormat(dstFmt)
	src := []byte{0, 0, 0, 0}
	result := convertAudioPCMNearest(src, srcWfx, dstWfx)
	// Should return original src since bit depths differ
	if &result[0] != &src[0] {
		t.Fatal("expected original src returned on bit depth mismatch")
	}
}

func TestTrimQueuedAudioReusesBuffer(t *testing.T) {
	buf := make([]byte, 100)
	for i := range buf {
		buf[i] = byte(i)
	}
	result, trim := trimQueuedAudio(buf, 4, 40)
	if trim != 60 {
		t.Fatalf("trim = %d, want 60", trim)
	}
	if len(result) != 40 {
		t.Fatalf("result len = %d, want 40", len(result))
	}
	// Verify data is correct (should be bytes 60..99)
	if result[0] != 60 {
		t.Fatalf("result[0] = %d, want 60", result[0])
	}
}
