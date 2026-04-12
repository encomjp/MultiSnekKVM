//go:build windows

package audio

import (
	"bytes"
	"encoding/binary"
	"math"
)

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

func clampAudioSample(sample float64) float64 {
	if sample < -1 {
		return -1
	}
	if sample > 1 {
		return 1
	}
	return sample
}

func readNormalizedSample(raw []byte, frame, channel int, format waveFormatEx, codec audioSampleCodec) float64 {
	bytesPerSample := int(format.BitsPerSample) / 8
	offset := frame*int(format.BlockAlign) + channel*bytesPerSample
	if offset < 0 || offset+bytesPerSample > len(raw) {
		return 0
	}

	switch codec {
	case audioSampleCodecPCM16:
		value := int16(binary.LittleEndian.Uint16(raw[offset : offset+2]))
		return float64(value) / 32768.0
	case audioSampleCodecPCM24:
		value := int32(raw[offset]) | int32(raw[offset+1])<<8 | int32(raw[offset+2])<<16
		if value&0x00800000 != 0 {
			value |= ^0x00ffffff
		}
		return float64(value) / 8388608.0
	case audioSampleCodecPCM32:
		value := int32(binary.LittleEndian.Uint32(raw[offset : offset+4]))
		return float64(value) / 2147483648.0
	case audioSampleCodecFloat32:
		value := float64(math.Float32frombits(binary.LittleEndian.Uint32(raw[offset : offset+4])))
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return 0
		}
		return clampAudioSample(value)
	default:
		return 0
	}
}

func writeNormalizedSample(dst []byte, frame, channel int, format waveFormatEx, codec audioSampleCodec, sample float64) {
	sample = clampAudioSample(sample)
	bytesPerSample := int(format.BitsPerSample) / 8
	offset := frame*int(format.BlockAlign) + channel*bytesPerSample
	if offset < 0 || offset+bytesPerSample > len(dst) {
		return
	}

	switch codec {
	case audioSampleCodecPCM16:
		value := int32(math.Round(sample * 32767.0))
		if sample <= -1 {
			value = -32768
		}
		binary.LittleEndian.PutUint16(dst[offset:offset+2], uint16(int16(value)))
	case audioSampleCodecPCM24:
		value := int32(math.Round(sample * 8388607.0))
		if sample <= -1 {
			value = -8388608
		}
		dst[offset] = byte(value)
		dst[offset+1] = byte(value >> 8)
		dst[offset+2] = byte(value >> 16)
	case audioSampleCodecPCM32:
		value := int64(math.Round(sample * 2147483647.0))
		if sample <= -1 {
			value = -2147483648
		}
		binary.LittleEndian.PutUint32(dst[offset:offset+4], uint32(int32(value)))
	case audioSampleCodecFloat32:
		binary.LittleEndian.PutUint32(dst[offset:offset+4], math.Float32bits(float32(sample)))
	}
}

func mixedSampleForOutput(src []byte, frame, dstChannel, dstChannels int, srcFormat waveFormatEx, srcCodec audioSampleCodec) float64 {
	srcChannels := int(srcFormat.Channels)
	if srcChannels == 0 {
		return 0
	}

	if srcChannels == 1 {
		return readNormalizedSample(src, frame, 0, srcFormat, srcCodec)
	}

	if dstChannels == 1 {
		total := 0.0
		for channel := 0; channel < srcChannels; channel++ {
			total += readNormalizedSample(src, frame, channel, srcFormat, srcCodec)
		}
		return total / float64(srcChannels)
	}

	if dstChannel < srcChannels {
		return readNormalizedSample(src, frame, dstChannel, srcFormat, srcCodec)
	}

	return readNormalizedSample(src, frame, dstChannel%srcChannels, srcFormat, srcCodec)
}

func convertAudioPCMNearest(src []byte, srcFmt, dstFmt waveFormatEx) []byte {
	srcBpf := int(srcFmt.BlockAlign)
	dstBpf := int(dstFmt.BlockAlign)
	srcBps := int(srcFmt.BitsPerSample) / 8
	dstBps := int(dstFmt.BitsPerSample) / 8
	if srcBpf == 0 || dstBpf == 0 || srcBps == 0 || dstBps == 0 {
		return src
	}
	if srcBps != dstBps {
		return src
	}
	bps := srcBps
	srcFrames := len(src) / srcBpf
	if srcFrames == 0 {
		return nil
	}

	srcCh := int(srcFmt.Channels)
	dstCh := int(dstFmt.Channels)
	srcRate := int64(srcFmt.SamplesPerSec)
	dstRate := int64(dstFmt.SamplesPerSec)

	dstFrames := int(int64(srcFrames) * dstRate / srcRate)
	if dstFrames == 0 {
		return nil
	}

	dst := make([]byte, dstFrames*dstBpf)
	chToCopy := srcCh
	if dstCh < chToCopy {
		chToCopy = dstCh
	}

	for i := 0; i < dstFrames; i++ {
		srcIdx := int(int64(i) * srcRate / dstRate)
		if srcIdx >= srcFrames {
			srcIdx = srcFrames - 1
		}
		srcBase := srcIdx * srcBpf
		dstBase := i * dstBpf
		if srcCh == 1 && dstCh > 1 {
			sample := src[srcBase : srcBase+bps]
			for ch := 0; ch < dstCh; ch++ {
				copy(dst[dstBase+ch*bps:dstBase+(ch+1)*bps], sample)
			}
			continue
		}
		for ch := 0; ch < chToCopy; ch++ {
			copy(dst[dstBase+ch*bps:dstBase+(ch+1)*bps], src[srcBase+ch*bps:srcBase+(ch+1)*bps])
		}
	}
	return dst
}

func convertAudioPCM(src []byte, srcRaw, dstRaw []byte) []byte {
	srcFmt, srcOK := decodeWaveFormat(srcRaw)
	dstFmt, dstOK := decodeWaveFormat(dstRaw)
	if !srcOK || !dstOK {
		return src
	}

	srcBpf := int(srcFmt.BlockAlign)
	dstBpf := int(dstFmt.BlockAlign)
	if srcBpf == 0 || dstBpf == 0 {
		return src
	}

	srcFrames := len(src) / srcBpf
	if srcFrames == 0 {
		return nil
	}

	srcCodec := sampleCodecForFormat(srcRaw)
	dstCodec := sampleCodecForFormat(dstRaw)
	if srcCodec == audioSampleCodecUnknown || dstCodec == audioSampleCodecUnknown {
		return convertAudioPCMNearest(src, srcFmt, dstFmt)
	}

	dstFrames := int((int64(srcFrames)*int64(dstFmt.SamplesPerSec) + int64(srcFmt.SamplesPerSec)/2) / int64(srcFmt.SamplesPerSec))
	if dstFrames == 0 {
		return nil
	}

	dst := make([]byte, dstFrames*dstBpf)
	srcRate := float64(srcFmt.SamplesPerSec)
	dstRate := float64(dstFmt.SamplesPerSec)
	dstChannels := int(dstFmt.Channels)

	for frame := 0; frame < dstFrames; frame++ {
		sourcePos := float64(frame) * srcRate / dstRate
		leftFrame := int(sourcePos)
		if leftFrame >= srcFrames {
			leftFrame = srcFrames - 1
		}
		rightFrame := leftFrame + 1
		if rightFrame >= srcFrames {
			rightFrame = srcFrames - 1
		}
		frac := sourcePos - float64(leftFrame)

		for channel := 0; channel < dstChannels; channel++ {
			leftSample := mixedSampleForOutput(src, leftFrame, channel, dstChannels, srcFmt, srcCodec)
			sample := leftSample
			if frac > 0 && rightFrame != leftFrame {
				rightSample := mixedSampleForOutput(src, rightFrame, channel, dstChannels, srcFmt, srcCodec)
				sample = leftSample + (rightSample-leftSample)*frac
			}
			writeNormalizedSample(dst, frame, channel, dstFmt, dstCodec, sample)
		}
	}

	return dst
}
