//go:build windows && cgo

package audio

import (
	"encoding/binary"
	"fmt"
	"time"

	"github.com/hraban/opus"
)

const (
	opusRealtimeSampleRate = 48000
	opusMaxFrameDuration   = 60 * time.Millisecond
	opusMaxPacketBytes     = 4000
)

type opusRealtimeEncoder struct {
	encoder      *opus.Encoder
	sourceFormat []byte
	targetFormat []byte
	frameBytes   int
	channels     int
	buffer       []byte
}

type opusRealtimeDecoder struct {
	decoder         *opus.Decoder
	playbackFormat  []byte
	channels        int
	maxDecodeBuffer int
}

func newOpusRealtimeEncoder(label, profile string, sourceFormat []byte) (*opusRealtimeEncoder, []byte, error) {
	sourceFmt, ok := decodeWaveFormat(sourceFormat)
	if !ok {
		return nil, nil, fmt.Errorf("%s transport source format invalid", label)
	}

	channels := 2
	if sourceFmt.Channels <= 1 {
		channels = 1
	}
	playbackFormat := newPCM16WaveFormat(opusRealtimeSampleRate, channels)
	spec := audioProfileSpecForName(profile)

	application := opus.AppAudio
	if label == "mic" {
		application = opus.AppVoIP
	} else if spec.OpusRestrictedDelay {
		application = opus.AppRestrictedLowdelay
	}

	encoder, err := opus.NewEncoder(opusRealtimeSampleRate, channels, application)
	if err != nil {
		return nil, nil, fmt.Errorf("%s transport encoder init: %w", label, err)
	}
	if err := encoder.SetBitrate(spec.OpusBitrate); err != nil {
		return nil, nil, fmt.Errorf("%s transport bitrate: %w", label, err)
	}
	if err := encoder.SetComplexity(spec.OpusComplexity); err != nil {
		return nil, nil, fmt.Errorf("%s transport complexity: %w", label, err)
	}

	if label == "mic" {
		if err := encoder.SetDTX(spec.OpusEnableDTX); err != nil {
			return nil, nil, fmt.Errorf("%s transport dtx: %w", label, err)
		}
		if err := encoder.SetInBandFEC(spec.OpusEnableFEC); err != nil {
			return nil, nil, fmt.Errorf("%s transport fec: %w", label, err)
		}
		if spec.OpusEnableFEC {
			if err := encoder.SetPacketLossPerc(spec.OpusPacketLossPct); err != nil {
				return nil, nil, fmt.Errorf("%s transport packet loss: %w", label, err)
			}
		}
	} else {
		if err := encoder.SetDTX(false); err != nil {
			return nil, nil, fmt.Errorf("%s transport dtx: %w", label, err)
		}
		if err := encoder.SetInBandFEC(false); err != nil {
			return nil, nil, fmt.Errorf("%s transport fec: %w", label, err)
		}
	}

	frameSamples := int((int64(opusRealtimeSampleRate) * int64(spec.OpusFrameDuration)) / int64(time.Second))
	if frameSamples <= 0 {
		frameSamples = opusRealtimeSampleRate / 50
	}

	return &opusRealtimeEncoder{
		encoder:      encoder,
		sourceFormat: cloneBytes(sourceFormat),
		targetFormat: playbackFormat,
		frameBytes:   frameSamples * channels * 2,
		channels:     channels,
	}, playbackFormat, nil
}

func (e *opusRealtimeEncoder) Encode(payload []byte) ([][]byte, error) {
	converted := convertAudioPCM(payload, e.sourceFormat, e.targetFormat)
	if len(converted) == 0 {
		return nil, nil
	}

	e.buffer = append(e.buffer, converted...)
	packets := make([][]byte, 0, len(e.buffer)/e.frameBytes+1)
	for len(e.buffer) >= e.frameBytes {
		chunk := e.buffer[:e.frameBytes]
		pcm := pcmBytesToInt16(chunk)
		packet := make([]byte, opusMaxPacketBytes)
		n, err := e.encoder.Encode(pcm, packet)
		if err != nil {
			return nil, err
		}
		packet = cloneBytes(packet[:n])
		if len(packet) > 0 {
			packets = append(packets, packet)
		}
		e.buffer = e.buffer[e.frameBytes:]
		if len(e.buffer) == 0 || cap(e.buffer) > 2*len(e.buffer) {
			e.buffer = cloneBytes(e.buffer)
		}
	}
	return packets, nil
}

func (e *opusRealtimeEncoder) Reset() {
	e.buffer = nil
	if e.encoder != nil {
		_ = e.encoder.Reset()
	}
}

func newOpusRealtimeDecoder(label string, playbackFormat []byte) (*opusRealtimeDecoder, []byte, error) {
	decodedFmt, ok := decodeWaveFormat(playbackFormat)
	if !ok {
		return nil, nil, fmt.Errorf("%s transport playback format invalid", label)
	}
	channels := int(decodedFmt.Channels)
	if channels != 1 && channels != 2 {
		return nil, nil, fmt.Errorf("%s transport channel count unsupported: %d", label, channels)
	}
	decoder, err := opus.NewDecoder(int(decodedFmt.SamplesPerSec), channels)
	if err != nil {
		return nil, nil, fmt.Errorf("%s transport decoder init: %w", label, err)
	}
	maxDecodeSamples := int((int64(decodedFmt.SamplesPerSec) * int64(opusMaxFrameDuration)) / int64(time.Second))
	if maxDecodeSamples <= 0 {
		maxDecodeSamples = int(decodedFmt.SamplesPerSec / 50)
	}
	return &opusRealtimeDecoder{
		decoder:         decoder,
		playbackFormat:  cloneBytes(playbackFormat),
		channels:        channels,
		maxDecodeBuffer: maxDecodeSamples * channels,
	}, cloneBytes(playbackFormat), nil
}

func (d *opusRealtimeDecoder) Decode(payload []byte) ([]byte, error) {
	pcm := make([]int16, d.maxDecodeBuffer)
	n, err := d.decoder.Decode(payload, pcm)
	if err != nil {
		return nil, err
	}
	return int16ToPCMBytes(pcm[:n*d.channels]), nil
}

func (d *opusRealtimeDecoder) Reset() {}

func newPCM16WaveFormat(sampleRate, channels int) []byte {
	raw := make([]byte, waveFormatExSize)
	binary.LittleEndian.PutUint16(raw[0:2], _waveFormatPcm)
	binary.LittleEndian.PutUint16(raw[2:4], uint16(channels))
	binary.LittleEndian.PutUint32(raw[4:8], uint32(sampleRate))
	binary.LittleEndian.PutUint32(raw[8:12], uint32(sampleRate*channels*2))
	binary.LittleEndian.PutUint16(raw[12:14], uint16(channels*2))
	binary.LittleEndian.PutUint16(raw[14:16], 16)
	binary.LittleEndian.PutUint16(raw[16:18], 0)
	return raw
}

func pcmBytesToInt16(raw []byte) []int16 {
	samples := make([]int16, len(raw)/2)
	for index := range samples {
		samples[index] = int16(binary.LittleEndian.Uint16(raw[index*2 : index*2+2]))
	}
	return samples
}

func int16ToPCMBytes(samples []int16) []byte {
	raw := make([]byte, len(samples)*2)
	for index, sample := range samples {
		binary.LittleEndian.PutUint16(raw[index*2:index*2+2], uint16(sample))
	}
	return raw
}
