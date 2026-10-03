package audio

import (
	"fmt"
	"sync"
	"time"

	"multisnekkvm/internal/protocol"
)

// StreamKind identifies one of the two independent realtime audio streams.
type StreamKind int

const (
	StreamDesktop StreamKind = iota
	StreamMic
)

func (k StreamKind) String() string {
	if k == StreamMic {
		return "mic"
	}
	return "audio"
}

func (k StreamKind) messages() (transport, format, data byte) {
	if k == StreamMic {
		return protocol.MsgMicTransport, protocol.MsgMicFormat, protocol.MsgMicData
	}
	return protocol.MsgAudioTransport, protocol.MsgAudioFormat, protocol.MsgAudioData
}

const (
	opusSampleRate    = 48000
	opusMaxPacket     = 4000
	opusMaxFrameMs    = 120
	pcmWireMaxRate    = 48000
	micOpusBitrate    = 64000
	micOpusComplexity = 8
)

type opusEncoderConfig struct {
	Channels   int
	Bitrate    int
	Complexity int
	LowDelay   bool
}

type opusEncoder interface {
	EncodeFloat32(pcm []float32, data []byte) (int, error)
}

type opusDecoder interface {
	DecodeFloat32(data []byte, pcm []float32) (int, error)
}

// Codec constructors; replaced in tests.
var (
	opusEncoderFactory = newOpusEncoder
	opusDecoderFactory = newOpusDecoder
)

// OutboundStream turns captured device audio into wire frames. Desktop audio
// is sent as stereo (surround folded down), the microphone as mono. PCM is
// sent as 16-bit at <=48 kHz; Opus at 48 kHz with DTX and FEC off: the link is
// TCP, so packets are never lost, and DTX would chop quiet speech.
type OutboundStream struct {
	mu   sync.Mutex
	kind StreamKind

	mode     string
	profile  string
	devRaw   []byte
	dev      pcmSpec
	wireCh   int
	wireRate int
	wireSpec pcmSpec

	resampler    *streamResampler
	enc          opusEncoder
	frameSamples int
	pending      []float32

	decoded, mapped, resampled []float32
	packet                     []byte
}

func NewOutboundStream(kind StreamKind) *OutboundStream {
	return &OutboundStream{kind: kind}
}

// Configure (re)starts the stream for a device format and returns the
// transport and format frames that must precede its data. If Opus is not
// available it falls back to PCM; the returned mode is the one in effect.
func (s *OutboundStream) Configure(mode, profile string, deviceFormat []byte) ([]protocol.Frame, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.configureLocked(mode, profile, deviceFormat)
}

// Reannounce repeats the transport and format frames (resetting the encoder)
// so a receiver that lost its state can resynchronise.
func (s *OutboundStream) Reannounce() ([]protocol.Frame, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.devRaw == nil {
		return nil, fmt.Errorf("%s stream not configured", s.kind)
	}
	frames, _, err := s.configureLocked(s.mode, s.profile, s.devRaw)
	return frames, err
}

// Reconfigure switches transport/profile for the current device format.
func (s *OutboundStream) Reconfigure(mode, profile string) ([]protocol.Frame, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.devRaw == nil {
		return nil, "", fmt.Errorf("%s stream not configured", s.kind)
	}
	return s.configureLocked(mode, profile, s.devRaw)
}

func (s *OutboundStream) configureLocked(mode, profile string, deviceFormat []byte) ([]protocol.Frame, string, error) {
	dev, err := parsePCMSpec(deviceFormat)
	if err != nil {
		return nil, "", fmt.Errorf("%s capture format: %w", s.kind, err)
	}
	s.dev = dev
	s.devRaw = append([]byte(nil), deviceFormat...)
	s.profile = NormalizeProfile(profile)
	s.wireCh = min(dev.Channels, 2)
	if s.kind == StreamMic {
		s.wireCh = 1
	}
	s.pending = s.pending[:0]
	s.enc = nil

	if mode == audioTransportOpus {
		if err := s.setupOpusLocked(); err == nil {
			s.mode = audioTransportOpus
		} else {
			mode = audioTransportPCM
		}
	}
	if mode != audioTransportOpus {
		s.mode = audioTransportPCM
		s.wireRate = min(dev.Rate, pcmWireMaxRate)
	}
	s.wireSpec = pcmSpec{Rate: s.wireRate, Channels: s.wireCh, BlockAlign: s.wireCh * 2, Codec: audioSampleCodecPCM16}
	s.resampler = newStreamResampler(dev.Rate, s.wireRate, s.wireCh)

	transportMsg, formatMsg, _ := s.kind.messages()
	return []protocol.Frame{
		{Type: transportMsg, Payload: encodeAudioTransportMode(s.mode)},
		{Type: formatMsg, Payload: newPCM16WaveFormat(s.wireRate, s.wireCh)},
	}, s.mode, nil
}

func (s *OutboundStream) setupOpusLocked() error {
	spec := audioProfileSpecForName(s.profile)
	cfg := opusEncoderConfig{
		Channels:   s.wireCh,
		Bitrate:    spec.OpusBitrate,
		Complexity: spec.OpusComplexity,
		LowDelay:   spec.OpusRestrictedDelay,
	}
	if s.kind == StreamMic {
		cfg.Bitrate, cfg.Complexity, cfg.LowDelay = micOpusBitrate, micOpusComplexity, false
	}
	enc, err := opusEncoderFactory(cfg)
	if err != nil {
		return err
	}
	s.enc = enc
	s.wireRate = opusSampleRate
	s.frameSamples = int(int64(opusSampleRate) * int64(spec.OpusFrameDuration) / int64(time.Second))
	if s.packet == nil {
		s.packet = make([]byte, opusMaxPacket)
	}
	return nil
}

// Mode reports the transport in effect ("" before Configure).
func (s *OutboundStream) Mode() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.mode
}

// Process converts one captured device buffer into zero or more data frames.
// raw is not retained, so callers may reuse it.
func (s *OutboundStream) Process(raw []byte) ([]protocol.Frame, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.resampler == nil || len(raw) < s.dev.BlockAlign {
		return nil, nil
	}
	_, _, dataMsg := s.kind.messages()
	s.decoded = decodePCM(s.decoded[:0], raw, s.dev)
	s.mapped = mapChannels(s.mapped[:0], s.decoded, s.dev.Channels, s.wireCh)
	s.resampled = s.resampler.process(s.resampled[:0], s.mapped)

	if s.mode != audioTransportOpus {
		if len(s.resampled) < s.wireCh {
			return nil, nil
		}
		payload := make([]byte, len(s.resampled)/s.wireCh*s.wireSpec.BlockAlign)
		encodePCM(payload, s.resampled, s.wireSpec)
		return []protocol.Frame{{Type: dataMsg, Payload: payload}}, nil
	}

	s.pending = append(s.pending, s.resampled...)
	chunk := s.frameSamples * s.wireCh
	var frames []protocol.Frame
	consumed := 0
	for len(s.pending)-consumed >= chunk {
		n, err := s.enc.EncodeFloat32(s.pending[consumed:consumed+chunk], s.packet)
		consumed += chunk
		if err != nil {
			s.pending = s.pending[:0]
			return frames, fmt.Errorf("%s opus encode: %w", s.kind, err)
		}
		if n > 0 {
			frames = append(frames, protocol.Frame{Type: dataMsg, Payload: append([]byte(nil), s.packet[:n]...)})
		}
	}
	rest := copy(s.pending, s.pending[consumed:])
	s.pending = s.pending[:rest]
	return frames, nil
}

// Reset forgets the configuration; the next Configure starts fresh.
func (s *OutboundStream) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.devRaw = nil
	s.resampler = nil
	s.enc = nil
	s.pending = s.pending[:0]
	s.mode = ""
}

// InboundStream decodes received frames for one stream. It only accepts data
// after a format for the current transport has been received; data in an
// unknown state is dropped instead of being guessed at.
type InboundStream struct {
	kind  StreamKind
	mode  string
	ready bool
	spec  pcmSpec
	dec   opusDecoder
	rate  int
	ch    int
	buf   []float32

	Dropped uint64
}

func NewInboundStream(kind StreamKind) *InboundStream {
	return &InboundStream{kind: kind}
}

func (s *InboundStream) HandleTransport(payload []byte) error {
	mode, err := DecodeTransportMode(payload)
	if err != nil {
		return err
	}
	s.mode = mode
	s.ready = false
	s.dec = nil
	return nil
}

// HandleFormat prepares decoding and returns the decoded output format.
func (s *InboundStream) HandleFormat(payload []byte) (rate, channels int, err error) {
	s.ready = false
	s.dec = nil
	spec, err := parsePCMSpec(payload)
	if err != nil {
		return 0, 0, fmt.Errorf("%s format: %w", s.kind, err)
	}
	if s.mode == "" {
		// Peers always announce the transport first; PCM is the protocol default.
		s.mode = audioTransportPCM
	}
	if s.mode == audioTransportOpus {
		if spec.Channels > 2 {
			return 0, 0, fmt.Errorf("%s opus format has %d channels", s.kind, spec.Channels)
		}
		dec, err := opusDecoderFactory(spec.Rate, spec.Channels)
		if err != nil {
			return 0, 0, fmt.Errorf("%s opus decoder: %w", s.kind, err)
		}
		s.dec = dec
		s.buf = make([]float32, spec.Rate*opusMaxFrameMs/1000*spec.Channels)
	}
	s.spec = spec
	s.rate, s.ch = spec.Rate, spec.Channels
	s.ready = true
	return s.rate, s.ch, nil
}

// HandleData decodes one data frame. The returned slice is reused by the next
// call. It returns nil when the frame is dropped.
func (s *InboundStream) HandleData(payload []byte) ([]float32, error) {
	if !s.ready || len(payload) == 0 {
		s.Dropped++
		return nil, nil
	}
	if s.mode == audioTransportOpus {
		n, err := s.dec.DecodeFloat32(payload, s.buf)
		if err != nil {
			s.Dropped++
			return nil, fmt.Errorf("%s opus decode: %w", s.kind, err)
		}
		return s.buf[:n*s.ch], nil
	}
	if len(payload)%s.spec.BlockAlign != 0 {
		s.Dropped++
		return nil, fmt.Errorf("%s pcm frame of %d bytes is not a multiple of %d", s.kind, len(payload), s.spec.BlockAlign)
	}
	s.buf = decodePCM(s.buf[:0], payload, s.spec)
	return s.buf, nil
}

func (s *InboundStream) Format() (rate, channels int, ok bool) {
	return s.rate, s.ch, s.ready
}

func (s *InboundStream) Reset() {
	s.mode = ""
	s.ready = false
	s.dec = nil
}
