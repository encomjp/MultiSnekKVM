//go:build !windows

package input

import "multisnekkvm/internal/protocol"

type InputHook struct {
	onStateChange func()
}

func NewInputHook() *InputHook                                { return &InputHook{} }
func (ih *InputHook) SetOnStateChange(fn func())              { ih.onStateChange = fn }
func (ih *InputHook) SetOnEdgeDrag(fn func())                 {}
func (ih *InputHook) SetConnected(bool, func(protocol.Frame)) {}
func (ih *InputHook) IsInRemoteMode() bool                    { return false }
func (ih *InputHook) ExitRemoteMode()                         {}
func (ih *InputHook) SetEdgeSide(string)                      {}
func (ih *InputHook) GetEdgeSide() string                     { return "right" }
func (ih *InputHook) SetSensitivity(float64)                  {}
func (ih *InputHook) GetSensitivity() float64                 { return 1.0 }

func InjectMouseMove(dx, dy int32)                                      {}
func InjectMouseClick(button byte, pressed bool)                        {}
func InjectMouseScroll(delta int32)                                     {}
func InjectKey(vkCode uint16, scanCode uint16, flags uint32, down bool) {}
func ReleaseAllModifiers()                                              {}
func GetClipboardText() string                                          { return "" }
func GetClipboardTextForSync() (string, bool)                           { return "", false }
func SetClipboardText(text string)                                      {}
func GetClipboardFiles() []string                                       { return nil }
func GetScreenBounds() (int32, int32, int32, int32)                     { return 0, 1920, 0, 1080 }
func GetCursorPosition() (int32, int32)                                 { return 0, 0 }
func IsSecureDesktopActive() bool                                       { return false }
