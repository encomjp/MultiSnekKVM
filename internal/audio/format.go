package audio

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
)

const (
	waveFormatExSize      = 18
	_waveFormatPcm        = 0x0001
	_waveFormatIEEEFloat  = 0x0003
	_waveFormatExtensible = 0xfffe
)

// waveFormatSubtypeTail is the shared tail of KSDATAFORMAT_SUBTYPE_* GUIDs;
// the first 4 bytes of the SubFormat GUID hold the plain format tag.
var waveFormatSubtypeTail = [12]byte{0x00, 0x00, 0x10, 0x00, 0x80, 0x00, 0x00, 0xaa, 0x00, 0x38, 0x9b, 0x71}

type waveFormatEx struct {
	FormatTag      uint16
	Channels       uint16
	SamplesPerSec  uint32
	AvgBytesPerSec uint32
	BlockAlign     uint16
	BitsPerSample  uint16
	CbSize         uint16
}

type audioSampleCodec int

const (
	audioSampleCodecUnknown audioSampleCodec = iota
	audioSampleCodecPCM16
	audioSampleCodecPCM24
	audioSampleCodecPCM32
	audioSampleCodecFloat32
)

// pcmSpec is a validated, decoded description of an interleaved PCM stream.
type pcmSpec struct {
	Rate       int
	Channels   int
	BlockAlign int
	Codec      audioSampleCodec
}

func (s pcmSpec) bytesPerSample() int {
	switch s.Codec {
	case audioSampleCodecPCM16:
		return 2
	case audioSampleCodecPCM24:
		return 3
	case audioSampleCodecPCM32, audioSampleCodecFloat32:
		return 4
	}
	return 0
}

func decodeWaveFormat(raw []byte) (waveFormatEx, bool) {
	if len(raw) < waveFormatExSize {
		return waveFormatEx{}, false
	}
	return waveFormatEx{
		FormatTag:      binary.LittleEndian.Uint16(raw[0:2]),
		Channels:       binary.LittleEndian.Uint16(raw[2:4]),
		SamplesPerSec:  binary.LittleEndian.Uint32(raw[4:8]),
		AvgBytesPerSec: binary.LittleEndian.Uint32(raw[8:12]),
		BlockAlign:     binary.LittleEndian.Uint16(raw[12:14]),
		BitsPerSample:  binary.LittleEndian.Uint16(raw[14:16]),
		CbSize:         binary.LittleEndian.Uint16(raw[16:18]),
	}, true
}

func describeWaveFormat(raw []byte) string {
	wfx, ok := decodeWaveFormat(raw)
	if !ok {
		return fmt.Sprintf("invalid format (%d bytes)", len(raw))
	}
	return fmt.Sprintf("%dHz %dch %dbit (blockAlign=%d, formatTag=0x%04x)",
		wfx.SamplesPerSec, wfx.Channels, wfx.BitsPerSample, wfx.BlockAlign, wfx.FormatTag)
}

func blockAlignForFormat(raw []byte) int {
	wfx, ok := decodeWaveFormat(raw)
	if !ok {
		return 0
	}
	return int(wfx.BlockAlign)
}

func sampleCodecForFormat(raw []byte) audioSampleCodec {
	wfx, ok := decodeWaveFormat(raw)
	if !ok {
		return audioSampleCodecUnknown
	}
	formatTag := wfx.FormatTag
	if formatTag == _waveFormatExtensible && len(raw) >= 40 && bytes.Equal(raw[28:40], waveFormatSubtypeTail[:]) {
		formatTag = uint16(binary.LittleEndian.Uint32(raw[24:28]))
	}
	switch formatTag {
	case _waveFormatPcm:
		switch wfx.BitsPerSample {
		case 16:
			return audioSampleCodecPCM16
		case 24:
			return audioSampleCodecPCM24
		case 32:
			return audioSampleCodecPCM32
		}
	case _waveFormatIEEEFloat:
		if wfx.BitsPerSample == 32 {
			return audioSampleCodecFloat32
		}
	}
	return audioSampleCodecUnknown
}

// parsePCMSpec validates a WAVEFORMATEX(TENSIBLE) blob. Formats we cannot
// decode sample-accurately are rejected rather than guessed at: playing
// misinterpreted bytes is what produces loud white noise.
func parsePCMSpec(raw []byte) (pcmSpec, error) {
	wfx, ok := decodeWaveFormat(raw)
	if !ok {
		return pcmSpec{}, fmt.Errorf("format too short (%d bytes)", len(raw))
	}
	spec := pcmSpec{
		Rate:       int(wfx.SamplesPerSec),
		Channels:   int(wfx.Channels),
		BlockAlign: int(wfx.BlockAlign),
		Codec:      sampleCodecForFormat(raw),
	}
	if spec.Codec == audioSampleCodecUnknown {
		return pcmSpec{}, fmt.Errorf("unsupported sample format: %s", describeWaveFormat(raw))
	}
	if spec.Rate < 8000 || spec.Rate > 384000 {
		return pcmSpec{}, fmt.Errorf("unsupported sample rate %d", spec.Rate)
	}
	if spec.Channels < 1 || spec.Channels > 8 {
		return pcmSpec{}, fmt.Errorf("unsupported channel count %d", spec.Channels)
	}
	if spec.BlockAlign < spec.Channels*spec.bytesPerSample() {
		return pcmSpec{}, fmt.Errorf("block align %d too small for %s", spec.BlockAlign, describeWaveFormat(raw))
	}
	return spec, nil
}

func newPCM16WaveFormat(sampleRate, channels int) []byte {
	raw := make([]byte, waveFormatExSize)
	binary.LittleEndian.PutUint16(raw[0:2], _waveFormatPcm)
	binary.LittleEndian.PutUint16(raw[2:4], uint16(channels))
	binary.LittleEndian.PutUint32(raw[4:8], uint32(sampleRate))
	binary.LittleEndian.PutUint32(raw[8:12], uint32(sampleRate*channels*2))
	binary.LittleEndian.PutUint16(raw[12:14], uint16(channels*2))
	binary.LittleEndian.PutUint16(raw[14:16], 16)
	return raw
}

func newFloat32WaveFormat(sampleRate, channels int) []byte {
	raw := make([]byte, waveFormatExSize)
	binary.LittleEndian.PutUint16(raw[0:2], _waveFormatIEEEFloat)
	binary.LittleEndian.PutUint16(raw[2:4], uint16(channels))
	binary.LittleEndian.PutUint32(raw[4:8], uint32(sampleRate))
	binary.LittleEndian.PutUint32(raw[8:12], uint32(sampleRate*channels*4))
	binary.LittleEndian.PutUint16(raw[12:14], uint16(channels*4))
	binary.LittleEndian.PutUint16(raw[14:16], 32)
	return raw
}

// decodePCM appends the samples of whole frames in raw to dst as interleaved
// float32 in [-1, 1].
func decodePCM(dst []float32, raw []byte, spec pcmSpec) []float32 {
	frames := len(raw) / spec.BlockAlign
	bps := spec.bytesPerSample()
	for f := 0; f < frames; f++ {
		base := f * spec.BlockAlign
		for c := 0; c < spec.Channels; c++ {
			o := base + c*bps
			var v float32
			switch spec.Codec {
			case audioSampleCodecPCM16:
				v = float32(int16(binary.LittleEndian.Uint16(raw[o:]))) / 32768
			case audioSampleCodecPCM24:
				s := int32(raw[o]) | int32(raw[o+1])<<8 | int32(int8(raw[o+2]))<<16
				v = float32(s) / 8388608
			case audioSampleCodecPCM32:
				v = float32(float64(int32(binary.LittleEndian.Uint32(raw[o:]))) / 2147483648)
			case audioSampleCodecFloat32:
				v = math.Float32frombits(binary.LittleEndian.Uint32(raw[o:]))
				if v != v || v > 1e3 || v < -1e3 { // NaN or garbage
					v = 0
				}
			}
			dst = append(dst, clampSample(v))
		}
	}
	return dst
}

// encodePCM writes interleaved float32 samples into dst using spec's codec.
// dst must hold len(src)/spec.Channels frames.
func encodePCM(dst []byte, src []float32, spec pcmSpec) {
	frames := len(src) / spec.Channels
	bps := spec.bytesPerSample()
	for f := 0; f < frames; f++ {
		base := f * spec.BlockAlign
		for c := 0; c < spec.Channels; c++ {
			o := base + c*bps
			v := float64(clampSample(src[f*spec.Channels+c]))
			switch spec.Codec {
			case audioSampleCodecPCM16:
				binary.LittleEndian.PutUint16(dst[o:], uint16(int16(math.Round(v*32767))))
			case audioSampleCodecPCM24:
				s := int32(math.Round(v * 8388607))
				dst[o], dst[o+1], dst[o+2] = byte(s), byte(s>>8), byte(s>>16)
			case audioSampleCodecPCM32:
				binary.LittleEndian.PutUint32(dst[o:], uint32(int32(math.Round(v*2147483647))))
			case audioSampleCodecFloat32:
				binary.LittleEndian.PutUint32(dst[o:], math.Float32bits(float32(v)))
			}
		}
	}
}

func clampSample(v float32) float32 {
	if v > 1 {
		return 1
	}
	if v < -1 {
		return -1
	}
	return v
}

// mapChannels converts one interleaved frame layout to another. Surround
// layouts (standard WAVE order FL FR FC LFE BL BR SL SR) fold into stereo
// keeping the centre channel, which carries dialogue in most content.
func mapChannels(dst, src []float32, srcCh, dstCh int) []float32 {
	frames := len(src) / srcCh
	if srcCh == dstCh {
		return append(dst, src[:frames*srcCh]...)
	}
	for f := 0; f < frames; f++ {
		in := src[f*srcCh : (f+1)*srcCh]
		var l, r float32
		switch {
		case srcCh == 1:
			l, r = in[0], in[0]
		case srcCh == 2:
			l, r = in[0], in[1]
		default:
			const k = 0.7071
			l, r = in[0], in[1]
			if srcCh > 2 {
				l += k * in[2]
				r += k * in[2]
			}
			// in[3] is LFE; dropped like most stereo downmixes.
			for c := 4; c < srcCh; c++ {
				if c%2 == 0 {
					l += k * in[c]
				} else {
					r += k * in[c]
				}
			}
			norm := float32(1 / (1 + k + k*float64((srcCh-3)/2)))
			l, r = l*norm, r*norm
		}
		switch dstCh {
		case 1:
			dst = append(dst, (l+r)*0.5)
		default:
			dst = append(dst, l, r)
			for c := 2; c < dstCh; c++ {
				dst = append(dst, 0)
			}
		}
	}
	return dst
}
