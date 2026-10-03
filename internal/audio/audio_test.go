package audio

import (
	"encoding/binary"
	"math"
	"testing"
	"time"

	"multisnekkvm/internal/protocol"
)

func extensibleFloatFormat(rate, channels int) []byte {
	raw := make([]byte, 40)
	copy(raw, newFloat32WaveFormat(rate, channels))
	binary.LittleEndian.PutUint16(raw[0:2], _waveFormatExtensible)
	binary.LittleEndian.PutUint16(raw[16:18], 22)
	binary.LittleEndian.PutUint16(raw[18:20], 32)
	binary.LittleEndian.PutUint32(raw[24:28], _waveFormatIEEEFloat)
	copy(raw[28:40], waveFormatSubtypeTail[:])
	return raw
}

func sine(rate, channels, frames int, freq float64, phase *float64) []float32 {
	out := make([]float32, 0, frames*channels)
	for f := 0; f < frames; f++ {
		v := float32(0.5 * math.Sin(*phase))
		*phase += 2 * math.Pi * freq / float64(rate)
		for c := 0; c < channels; c++ {
			out = append(out, v)
		}
	}
	return out
}

func TestPCMRoundTripAllCodecs(t *testing.T) {
	in := []float32{0, 0.5, -0.5, 0.25, -1, 0.999}
	for _, tc := range []struct {
		name string
		raw  []byte
		tol  float64
	}{
		{"pcm16", newPCM16WaveFormat(48000, 2), 1.0 / 16000},
		{"float32", newFloat32WaveFormat(48000, 2), 1e-7},
		{"extensible-float", extensibleFloatFormat(48000, 2), 1e-7},
	} {
		spec, err := parsePCMSpec(tc.raw)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		buf := make([]byte, len(in)/2*spec.BlockAlign)
		encodePCM(buf, in, spec)
		out := decodePCM(nil, buf, spec)
		for i := range in {
			if math.Abs(float64(out[i]-in[i])) > tc.tol {
				t.Fatalf("%s: sample %d = %v, want %v", tc.name, i, out[i], in[i])
			}
		}
	}
}

func TestParsePCMSpecRejectsUndecodableFormats(t *testing.T) {
	bad := newPCM16WaveFormat(48000, 2)
	binary.LittleEndian.PutUint16(bad[14:16], 8) // 8-bit PCM
	if _, err := parsePCMSpec(bad); err == nil {
		t.Fatal("8-bit PCM must be rejected, not guessed")
	}
	ext := extensibleFloatFormat(48000, 2)
	ext[30] ^= 0xff // corrupt the SubFormat GUID
	if _, err := parsePCMSpec(ext); err == nil {
		t.Fatal("unknown extensible subformat must be rejected")
	}
}

func TestMapChannelsKeepsCentreWhenFoldingSurround(t *testing.T) {
	// 5.1 frame with only the centre channel active (dialogue).
	out := mapChannels(nil, []float32{0, 0, 1, 0, 0, 0}, 6, 2)
	if out[0] < 0.2 || out[1] < 0.2 || out[0] != out[1] {
		t.Fatalf("centre lost in downmix: %v", out)
	}
	mono := mapChannels(nil, []float32{1, 0}, 2, 1)
	if mono[0] != 0.5 {
		t.Fatalf("stereo→mono = %v", mono)
	}
	stereo := mapChannels(nil, []float32{0.3}, 1, 2)
	if stereo[0] != 0.3 || stereo[1] != 0.3 {
		t.Fatalf("mono→stereo = %v", stereo)
	}
}

func TestStreamResamplerIsContinuousAcrossChunks(t *testing.T) {
	r := newStreamResampler(44100, 48000, 1)
	var phase float64
	var out []float32
	for _, n := range []int{441, 17, 1000, 3, 882, 4410} {
		out = r.process(out, sine(44100, 1, n, 1000, &phase))
	}
	// Compare with an ideal 1 kHz sine at 48 kHz (output i samples input time i*step).
	maxErr := 0.0
	for i := 200; i < len(out); i++ {
		tSrc := float64(i) * 44100 / 48000
		want := 0.5 * math.Sin(2*math.Pi*1000*tSrc/44100)
		maxErr = math.Max(maxErr, math.Abs(float64(out[i])-want))
	}
	if maxErr > 0.01 {
		t.Fatalf("resampled sine deviates by %.4f", maxErr)
	}
	total := 441 + 17 + 1000 + 3 + 882 + 4410
	wantLen := total * 48000 / 44100
	if d := len(out) - wantLen; d < -4 || d > 4 {
		t.Fatalf("got %d frames, want ≈%d", len(out), wantLen)
	}
}

type fakeClock struct{ t time.Time }

func (c *fakeClock) now() time.Time { return c.t }

func newTestPlayout(cfg playoutConfig) (*Playout, *fakeClock) {
	clk := &fakeClock{t: time.Unix(1000, 0)}
	p := newPlayout(cfg)
	p.now = clk.now
	return p, clk
}

func TestPlayoutPrimesBeforePlaying(t *testing.T) {
	p, _ := newTestPlayout(playoutConfig{Target: 40 * time.Millisecond, MaxTarget: 100 * time.Millisecond, Max: 300 * time.Millisecond})
	p.SetSource(48000, 1)
	var phase float64
	p.Push(sine(48000, 1, 960, 440, &phase)) // 20 ms < 40 ms target
	out := make([]float32, 480)
	p.Render(out, 480, 48000, 1)
	for _, v := range out {
		if v != 0 {
			t.Fatal("must stay silent until the target is buffered")
		}
	}
	p.Push(sine(48000, 1, 1920, 440, &phase))
	p.Render(out, 480, 48000, 1)
	if out[len(out)-1] == 0 {
		t.Fatal("expected audio once primed")
	}
}

// Steady streaming with a sender clock 0.2% fast must neither overflow
// (chunks cut out) nor underrun (silence inserted): drift control absorbs it.
func TestPlayoutAbsorbsClockDriftWithoutGaps(t *testing.T) {
	p, clk := newTestPlayout(playoutConfigForProfile(audioProfileBalanced))
	p.SetSource(48000, 2)
	var phase float64
	out := make([]float32, 480*2)
	carry := 0.0
	for i := 0; i < 3000; i++ { // 30 s of 10 ms ticks
		carry += 480 * 1.002
		n := int(carry)
		carry -= float64(n)
		p.Push(sine(48000, 2, n, 440, &phase))
		p.Render(out, 480, 48000, 2)
		clk.t = clk.t.Add(10 * time.Millisecond)
	}
	s := p.Stats()
	if s.Overflows != 0 || s.Underruns != 0 {
		t.Fatalf("drift caused overflows=%d underruns=%d", s.Overflows, s.Underruns)
	}
	if s.BufferedMs > 80 {
		t.Fatalf("buffer grew to %d ms; drift not compensated", s.BufferedMs)
	}
}

func TestPlayoutUnderrunFadesInsteadOfClicking(t *testing.T) {
	p, _ := newTestPlayout(playoutConfig{Target: 20 * time.Millisecond, MaxTarget: 60 * time.Millisecond, Max: 200 * time.Millisecond})
	p.SetSource(48000, 1)
	full := make([]float32, 2400)
	for i := range full {
		full[i] = 0.8 // DC level makes a hard cut obvious
	}
	p.Push(full)
	out := make([]float32, 4800)
	p.Render(out, 4800, 48000, 1) // asks for 100 ms, only ~50 ms buffered
	maxStep := float32(0)
	for i := 1; i < len(out); i++ {
		if d := float32(math.Abs(float64(out[i] - out[i-1]))); d > maxStep {
			maxStep = d
		}
	}
	if maxStep > 0.05 {
		t.Fatalf("output jumps by %.3f: click on underrun or start", maxStep)
	}
	if out[len(out)-1] != 0 {
		t.Fatal("expected silence after the buffered audio ran out")
	}
	s := p.Stats()
	if s.Underruns != 1 || s.TargetMs <= 20 {
		t.Fatalf("underrun should be counted and raise the target: %+v", s)
	}
}

func TestPlayoutDropsHugeBacklogToTarget(t *testing.T) {
	p, _ := newTestPlayout(playoutConfig{Target: 40 * time.Millisecond, MaxTarget: 100 * time.Millisecond, Max: 200 * time.Millisecond})
	p.SetSource(48000, 1)
	p.Push(make([]float32, 48000)) // 1 s burst after a stall
	s := p.Stats()
	if s.Overflows != 1 || s.BufferedMs > 45 {
		t.Fatalf("backlog not trimmed to target: %+v", s)
	}
}

type fakeOpusEncoder struct{ calls int }

func (e *fakeOpusEncoder) EncodeFloat32(pcm []float32, data []byte) (int, error) {
	e.calls++
	binary.LittleEndian.PutUint32(data, uint32(len(pcm)))
	return 4, nil
}

type fakeOpusDecoder struct{ channels int }

func (d *fakeOpusDecoder) DecodeFloat32(data []byte, pcm []float32) (int, error) {
	n := int(binary.LittleEndian.Uint32(data)) / d.channels
	for i := 0; i < n*d.channels; i++ {
		pcm[i] = 0.25
	}
	return n, nil
}

func withFakeOpus(t *testing.T) *fakeOpusEncoder {
	enc := &fakeOpusEncoder{}
	oldE, oldD := opusEncoderFactory, opusDecoderFactory
	opusEncoderFactory = func(cfg opusEncoderConfig) (opusEncoder, error) { return enc, nil }
	opusDecoderFactory = func(rate, ch int) (opusDecoder, error) { return &fakeOpusDecoder{channels: ch}, nil }
	t.Cleanup(func() { opusEncoderFactory, opusDecoderFactory = oldE, oldD })
	return enc
}

func TestOutboundMicSendsMonoPCM16Format(t *testing.T) {
	s := NewOutboundStream(StreamMic)
	frames, mode, err := s.Configure(TransportPCM, audioProfileBalanced, extensibleFloatFormat(48000, 2))
	if err != nil || mode != TransportPCM {
		t.Fatalf("configure: %v %q", err, mode)
	}
	if frames[0].Type != protocol.MsgMicTransport || frames[1].Type != protocol.MsgMicFormat {
		t.Fatalf("unexpected announce frames %+v", frames)
	}
	spec, err := parsePCMSpec(frames[1].Payload)
	if err != nil || spec.Channels != 1 || spec.Codec != audioSampleCodecPCM16 || spec.Rate != 48000 {
		t.Fatalf("wire format %+v %v", spec, err)
	}
	dev, _ := parsePCMSpec(extensibleFloatFormat(48000, 2))
	var phase float64
	raw := make([]byte, 480*dev.BlockAlign)
	encodePCM(raw, sine(48000, 2, 480, 440, &phase), dev)
	data, err := s.Process(raw)
	if err != nil || len(data) != 1 || len(data[0].Payload) != 480*2 {
		t.Fatalf("data frames %v err=%v", len(data), err)
	}
}

func TestOutboundOpusMicFramesAndReannounce(t *testing.T) {
	enc := withFakeOpus(t)
	s := NewOutboundStream(StreamMic)
	if _, mode, err := s.Configure(TransportOpus, audioProfileBalanced, newPCM16WaveFormat(48000, 2)); err != nil || mode != TransportOpus {
		t.Fatalf("configure: %v %q", err, mode)
	}
	dev, _ := parsePCMSpec(newPCM16WaveFormat(48000, 2))
	raw := make([]byte, 1500*dev.BlockAlign) // 31.25 ms
	frames, err := s.Process(raw)
	if err != nil || len(frames) != 1 || enc.calls != 1 {
		t.Fatalf("expected one 20 ms packet, got %d (calls=%d, err=%v)", len(frames), enc.calls, err)
	}
	if got := binary.LittleEndian.Uint32(frames[0].Payload); got != 960 {
		t.Fatalf("mic opus frame should be 960 mono samples, got %d", got)
	}
	again, err := s.Reannounce()
	if err != nil || len(again) != 2 || again[0].Payload[0] != 1 {
		t.Fatalf("reannounce: %+v %v", again, err)
	}
}

func TestInboundDropsDataUntilFormatKnown(t *testing.T) {
	in := NewInboundStream(StreamMic)
	if out, _ := in.HandleData(make([]byte, 64)); out != nil {
		t.Fatal("data before any format must be dropped")
	}
	if err := in.HandleTransport([]byte{0}); err != nil {
		t.Fatal(err)
	}
	if out, _ := in.HandleData(make([]byte, 64)); out != nil {
		t.Fatal("data between transport and format must be dropped")
	}
	if _, _, err := in.HandleFormat(newPCM16WaveFormat(48000, 1)); err != nil {
		t.Fatal(err)
	}
	if out, _ := in.HandleData(make([]byte, 64)); len(out) != 32 {
		t.Fatalf("decoded %d samples, want 32", len(out))
	}
}

// Regression: after a transport change without a matching format, Opus
// packets used to be played as raw PCM — loud white noise.
func TestInboundNeverPlaysOpusPacketsAsPCM(t *testing.T) {
	withFakeOpus(t)
	in := NewInboundStream(StreamMic)
	_ = in.HandleTransport([]byte{1})
	if _, _, err := in.HandleFormat(newPCM16WaveFormat(48000, 1)); err != nil {
		t.Fatal(err)
	}
	pkt := make([]byte, 4)
	binary.LittleEndian.PutUint32(pkt, 960)
	if out, err := in.HandleData(pkt); err != nil || len(out) != 960 {
		t.Fatalf("opus decode: %d %v", len(out), err)
	}
	_ = in.HandleTransport([]byte{0})
	if out, _ := in.HandleData(pkt); out != nil {
		t.Fatal("packet after transport switch was decoded before its format arrived")
	}
}

func TestOutboundDesktopFoldsSurroundAndCapsRate(t *testing.T) {
	s := NewOutboundStream(StreamDesktop)
	frames, _, err := s.Configure(TransportPCM, audioProfileBalanced, newFloat32WaveFormat(96000, 6))
	if err != nil {
		t.Fatal(err)
	}
	spec, _ := parsePCMSpec(frames[1].Payload)
	if spec.Channels != 2 || spec.Rate != 48000 {
		t.Fatalf("wire format should be 48 kHz stereo, got %+v", spec)
	}
}

func TestResolveTransport(t *testing.T) {
	cases := []struct {
		kind  StreamKind
		mode  string
		route string
		want  string
	}{
		{StreamMic, TransportPCM, "ethernet", TransportOpus},
		{StreamDesktop, TransportAuto, "usb4", TransportPCM},
		{StreamDesktop, TransportAuto, "ethernet", TransportPCM},
		{StreamDesktop, TransportAuto, "tailscale", TransportOpus},
		{StreamDesktop, TransportAuto, "wifi", TransportOpus},
		{StreamDesktop, TransportAuto, "", TransportOpus},
		{StreamDesktop, TransportPCM, "tailscale", TransportPCM},
		{StreamDesktop, "bogus", "usb4", TransportPCM},
	}
	for _, c := range cases {
		if got := ResolveTransport(c.kind, c.mode, c.route); got != c.want {
			t.Errorf("ResolveTransport(%v,%q,%q) = %q, want %q", c.kind, c.mode, c.route, got, c.want)
		}
	}
}

func TestWorkerStopWaitsAndRestarts(t *testing.T) {
	var w worker
	exited := make(chan struct{}, 2)
	run := func(stop <-chan struct{}) { <-stop; exited <- struct{}{} }
	if !w.start(run) || w.start(run) {
		t.Fatal("second start while running must be a no-op")
	}
	w.stopAndWait()
	select {
	case <-exited:
	default:
		t.Fatal("stopAndWait returned before the loop exited")
	}
	if w.running() || !w.start(run) {
		t.Fatal("worker must be restartable")
	}
	w.stopAndWait()
}
