//go:build windows && cgo

package audio

import (
	"fmt"

	"github.com/hraban/opus"
)

func newOpusEncoder(cfg opusEncoderConfig) (opusEncoder, error) {
	app := opus.AppAudio
	if cfg.LowDelay {
		app = opus.AppRestrictedLowdelay
	}
	enc, err := opus.NewEncoder(opusSampleRate, cfg.Channels, app)
	if err != nil {
		return nil, fmt.Errorf("init: %w", err)
	}
	if err := enc.SetBitrate(cfg.Bitrate); err != nil {
		return nil, fmt.Errorf("bitrate: %w", err)
	}
	if err := enc.SetComplexity(cfg.Complexity); err != nil {
		return nil, fmt.Errorf("complexity: %w", err)
	}
	// TCP never loses packets: FEC only wastes bits, and DTX stops sending
	// during quiet passages, which made speech sound gated.
	if err := enc.SetDTX(false); err != nil {
		return nil, fmt.Errorf("dtx: %w", err)
	}
	if err := enc.SetInBandFEC(false); err != nil {
		return nil, fmt.Errorf("fec: %w", err)
	}
	return enc, nil
}

func newOpusDecoder(rate, channels int) (opusDecoder, error) {
	return opus.NewDecoder(rate, channels)
}
