//go:build windows

package input

import (
	"syscall"
	"unsafe"

	"multisnekkvm/internal/clipboard"
)

func GetClipboardText() string {
	text, _ := getClipboardText(0, 0)
	return text
}

func GetClipboardTextForSync() (string, bool) {
	return getClipboardText(clipboard.MaxClipboardUTF16Bytes, clipboard.MaxClipboardTextBytes)
}

func openClipboardWithRetry() bool {
	for attempt := 0; attempt < clipboardOpenMaxAttempts; attempt++ {
		if openClipboardFn() {
			return true
		}
		if attempt < clipboardOpenMaxAttempts-1 {
			clipboardRetrySleep(clipboardOpenRetryDelay)
		}
	}
	return false
}

func getClipboardText(maxUTF16Bytes uintptr, maxUTF8Bytes int) (string, bool) {
	if !openClipboardWithRetry() {
		return "", false
	}
	defer closeClipboardFn()

	h, _, _ := pGetClipboardData.Call(cfUnicodeText)
	if h == 0 {
		return "", false
	}

	sizeBytes, _, _ := pGlobalSize.Call(h)
	if sizeBytes < 2 {
		return "", false
	}
	if maxUTF16Bytes > 0 && sizeBytes > maxUTF16Bytes {
		return "", true
	}

	ptr, _, _ := pGlobalLock.Call(h)
	if ptr == 0 {
		return "", false
	}
	defer pGlobalUnlock.Call(h)

	utf16Len := int(sizeBytes / 2)
	text := syscall.UTF16ToString(unsafe.Slice((*uint16)(unsafe.Pointer(ptr)), utf16Len))
	if maxUTF8Bytes > 0 && len(text) > maxUTF8Bytes {
		return "", true
	}
	return text, false
}

func SetClipboardText(text string) {
	if !openClipboardWithRetry() {
		return
	}
	defer closeClipboardFn()

	utf16, _ := syscall.UTF16FromString(text)
	size := len(utf16) * 2

	h, _, _ := pGlobalAlloc.Call(gmemMoveable, uintptr(size))
	if h == 0 {
		return
	}

	ptr, _, _ := pGlobalLock.Call(h)
	if ptr == 0 {
		pGlobalFree.Call(h)
		return
	}

	dst := unsafe.Slice((*uint16)(unsafe.Pointer(ptr)), len(utf16))
	copy(dst, utf16)
	pGlobalUnlock.Call(h)

	pEmptyClipboard.Call()
	if ret, _, _ := pSetClipboardData.Call(cfUnicodeText, h); ret == 0 {
		pGlobalFree.Call(h)
	}
}

func GetClipboardFiles() []string { return nil }
