//go:build !windows

package audio

import "multisnekkvm/internal/protocol"

type AudioStreamer struct{}

func NewAudioStreamer() (*AudioStreamer, error)                        { return &AudioStreamer{}, nil }
func (a *AudioStreamer) SetQualityProfile(string) error               { return nil }
func (a *AudioStreamer) SetPlaybackFormat([]byte) error               { return nil }
func (a *AudioStreamer) StartCapture(func(protocol.Frame)) error      { return nil }
func (a *AudioStreamer) StopCapture()                                 {}
func (a *AudioStreamer) StartPlayback() error                         { return nil }
func (a *AudioStreamer) StopPlayback()                                {}
func (a *AudioStreamer) EnqueueAudio([]byte)                          {}
func (a *AudioStreamer) IsCapturing() bool                            { return false }
func (a *AudioStreamer) IsPlaying() bool                              { return false }
func (a *AudioStreamer) Close()                                       {}
func (a *AudioStreamer) SetMicPlaybackFormat([]byte) error            { return nil }
func (a *AudioStreamer) StartMicCapture(func(protocol.Frame)) error   { return nil }
func (a *AudioStreamer) StopMicCapture()                              {}
func (a *AudioStreamer) StartMicPlayback() error                      { return nil }
func (a *AudioStreamer) StopMicPlayback()                             {}
func (a *AudioStreamer) EnqueueMicAudio([]byte)                       {}
func (a *AudioStreamer) IsMicCapturing() bool                         { return false }
func (a *AudioStreamer) IsMicPlaying() bool                           { return false }
func (a *AudioStreamer) SetCaptureDeviceID(string)                    {}
func (a *AudioStreamer) SetPlaybackDeviceID(string)                   {}
func (a *AudioStreamer) SetMicDeviceID(string)                        {}
func (a *AudioStreamer) SetMicPlaybackDeviceID(string)                {}
func (a *AudioStreamer) GetMicPlaybackDeviceID() string               { return "" }
