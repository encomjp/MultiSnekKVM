//go:build !(windows && cgo)

package audio

import "errors"

var errOpusUnavailable = errors.New("opus requires the Windows cgo build with libopus")

func newOpusEncoder(opusEncoderConfig) (opusEncoder, error) { return nil, errOpusUnavailable }

func newOpusDecoder(int, int) (opusDecoder, error) { return nil, errOpusUnavailable }
