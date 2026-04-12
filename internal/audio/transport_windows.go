//go:build windows

package audio

import (
	"fmt"

	"multisnekkvm/internal/protocol"
)

type windowsRealtimeOutboundStream struct {
	label         string
	transportMsg  byte
	formatMsg     byte
	dataMsg       byte
	transportMode string
	profile       string
	encoder       *opusRealtimeEncoder
}

type windowsRealtimeInboundStream struct {
	label         string
	transportMode string
	decoder       *opusRealtimeDecoder
}

func NewOutboundRealtimeStream(label string, transportMsg, formatMsg, dataMsg byte) OutboundRealtimeStream {
	return &windowsRealtimeOutboundStream{
		label:         label,
		transportMsg:  transportMsg,
		formatMsg:     formatMsg,
		dataMsg:       dataMsg,
		transportMode: audioTransportPCM,
		profile:       audioProfileBalanced,
	}
}

func NewInboundRealtimeStream(label string) InboundRealtimeStream {
	return &windowsRealtimeInboundStream{label: label, transportMode: audioTransportPCM}
}

func (s *windowsRealtimeOutboundStream) Configure(transportMode, profile string, format []byte) ([]protocol.Frame, error) {
	transportMode = NormalizeTransportMode(transportMode)
	profile = NormalizeProfile(profile)
	s.Reset()
	s.transportMode = transportMode
	s.profile = profile

	frames := []protocol.Frame{{Type: s.transportMsg, Payload: encodeAudioTransportMode(transportMode)}}
	if transportMode == audioTransportPCM {
		frames = append(frames, protocol.Frame{Type: s.formatMsg, Payload: cloneBytes(format)})
		return frames, nil
	}

	encoder, playbackFormat, err := newOpusRealtimeEncoder(s.label, profile, format)
	if err != nil {
		return nil, err
	}
	s.encoder = encoder
	frames = append(frames, protocol.Frame{Type: s.formatMsg, Payload: playbackFormat})
	return frames, nil
}

func (s *windowsRealtimeOutboundStream) ProcessData(payload []byte) ([]protocol.Frame, error) {
	if len(payload) == 0 {
		return nil, nil
	}
	if s.transportMode != audioTransportOpus {
		return []protocol.Frame{{Type: s.dataMsg, Payload: payload}}, nil
	}
	if s.encoder == nil {
		return nil, fmt.Errorf("%s transport encoder not configured", s.label)
	}
	packets, err := s.encoder.Encode(payload)
	if err != nil {
		return nil, err
	}
	frames := make([]protocol.Frame, 0, len(packets))
	for _, packet := range packets {
		if len(packet) == 0 {
			continue
		}
		frames = append(frames, protocol.Frame{Type: s.dataMsg, Payload: packet})
	}
	return frames, nil
}

func (s *windowsRealtimeOutboundStream) Reset() {
	if s.encoder != nil {
		s.encoder.Reset()
		s.encoder = nil
	}
	s.transportMode = audioTransportPCM
	if s.profile == "" {
		s.profile = audioProfileBalanced
	}
}

func (s *windowsRealtimeInboundStream) SetTransport(transportMode string) error {
	transportMode = NormalizeTransportMode(transportMode)
	if s.transportMode != transportMode {
		if s.decoder != nil {
			s.decoder.Reset()
			s.decoder = nil
		}
	}
	s.transportMode = transportMode
	return nil
}

func (s *windowsRealtimeInboundStream) ProcessFormat(payload []byte) ([]byte, error) {
	if s.transportMode != audioTransportOpus {
		if s.decoder != nil {
			s.decoder.Reset()
			s.decoder = nil
		}
		return cloneBytes(payload), nil
	}

	decoder, playbackFormat, err := newOpusRealtimeDecoder(s.label, payload)
	if err != nil {
		return nil, err
	}
	s.decoder = decoder
	return playbackFormat, nil
}

func (s *windowsRealtimeInboundStream) ProcessData(payload []byte) ([]byte, error) {
	if len(payload) == 0 {
		return nil, nil
	}
	if s.transportMode != audioTransportOpus {
		return payload, nil
	}
	if s.decoder == nil {
		return nil, fmt.Errorf("%s transport decoder not configured", s.label)
	}
	return s.decoder.Decode(payload)
}

func (s *windowsRealtimeInboundStream) Reset() {
	if s.decoder != nil {
		s.decoder.Reset()
		s.decoder = nil
	}
	s.transportMode = audioTransportPCM
}
