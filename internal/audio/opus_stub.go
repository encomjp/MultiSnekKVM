//go:build !cgo

package audio

import "fmt"

type opusRealtimeEncoder struct{}

type opusRealtimeDecoder struct{}

func newOpusRealtimeEncoder(label, profile string, sourceFormat []byte) (*opusRealtimeEncoder, []byte, error) {
	_ = label
	_ = profile
	_ = sourceFormat
	return nil, nil, fmt.Errorf("opus transport requires cgo and libopus")
}

func (e *opusRealtimeEncoder) Encode(payload []byte) ([][]byte, error) {
	_ = payload
	return nil, fmt.Errorf("opus transport requires cgo and libopus")
}

func (e *opusRealtimeEncoder) Reset() {}

func newOpusRealtimeDecoder(label string, playbackFormat []byte) (*opusRealtimeDecoder, []byte, error) {
	_ = label
	_ = playbackFormat
	return nil, nil, fmt.Errorf("opus transport requires cgo and libopus")
}

func (d *opusRealtimeDecoder) Decode(payload []byte) ([]byte, error) {
	_ = payload
	return nil, fmt.Errorf("opus transport requires cgo and libopus")
}

func (d *opusRealtimeDecoder) Reset() {}
