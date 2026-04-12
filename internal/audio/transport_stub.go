//go:build !windows

package audio

import (
	"fmt"

	"multisnekkvm/internal/protocol"
)

type rawRealtimeOutboundStream struct {
	transportMsg byte
	formatMsg    byte
	dataMsg      byte
}

type rawRealtimeInboundStream struct{}

func NewOutboundRealtimeStream(_ string, transportMsg, formatMsg, dataMsg byte) OutboundRealtimeStream {
	return &rawRealtimeOutboundStream{transportMsg: transportMsg, formatMsg: formatMsg, dataMsg: dataMsg}
}

func NewInboundRealtimeStream(_ string) InboundRealtimeStream {
	return &rawRealtimeInboundStream{}
}

func (s *rawRealtimeOutboundStream) Configure(transportMode, _ string, format []byte) ([]protocol.Frame, error) {
	if NormalizeTransportMode(transportMode) == audioTransportOpus {
		return nil, fmt.Errorf("opus transport requires the Windows audio backend")
	}
	return []protocol.Frame{
		{Type: s.transportMsg, Payload: encodeAudioTransportMode(audioTransportPCM)},
		{Type: s.formatMsg, Payload: append([]byte(nil), format...)},
	}, nil
}

func (s *rawRealtimeOutboundStream) ProcessData(payload []byte) ([]protocol.Frame, error) {
	if len(payload) == 0 {
		return nil, nil
	}
	return []protocol.Frame{{Type: s.dataMsg, Payload: append([]byte(nil), payload...)}}, nil
}

func (s *rawRealtimeOutboundStream) Reset() {}

func (s *rawRealtimeInboundStream) SetTransport(transportMode string) error {
	if NormalizeTransportMode(transportMode) == audioTransportOpus {
		return fmt.Errorf("opus transport requires the Windows audio backend")
	}
	return nil
}

func (s *rawRealtimeInboundStream) ProcessFormat(payload []byte) ([]byte, error) {
	return append([]byte(nil), payload...), nil
}

func (s *rawRealtimeInboundStream) ProcessData(payload []byte) ([]byte, error) {
	return append([]byte(nil), payload...), nil
}

func (s *rawRealtimeInboundStream) Reset() {}
