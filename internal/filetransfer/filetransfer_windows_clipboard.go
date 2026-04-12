//go:build windows

package filetransfer

import (
	"log"
	"syscall"
	"unsafe"
)

func ReadOleDragFiles() []string {
	var dataObj uintptr
	hr, _, _ := pOleGetClipboard.Call(uintptr(unsafe.Pointer(&dataObj)))
	if hr != 0 || dataObj == 0 {
		return nil
	}
	defer comRelease(dataObj)

	fe := formatEtc{
		CfFormat: cfHDrop,
		DwAspect: dvaspectContent,
		Lindex:   -1,
		Tymed:    tymedHGlobal,
	}
	var medium stgMedium
	hr = comCall(dataObj, 3,
		uintptr(unsafe.Pointer(&fe)),
		uintptr(unsafe.Pointer(&medium)),
	)
	if hr != 0 {
		return nil
	}
	defer pReleaseStgMedium.Call(uintptr(unsafe.Pointer(&medium)))

	if medium.Tymed != tymedHGlobal || medium.HGlobal == 0 {
		return nil
	}

	ptr, _, _ := pGlobalLock.Call(medium.HGlobal)
	if ptr == 0 {
		return nil
	}
	defer pGlobalUnlock.Call(medium.HGlobal)

	df := (*dropFiles)(unsafe.Pointer(ptr))
	fileListPtr := ptr + uintptr(df.PFiles)
	totalSize, _, _ := pGlobalSize.Call(medium.HGlobal)
	if totalSize <= uintptr(df.PFiles) {
		return nil
	}
	remaining := totalSize - uintptr(df.PFiles)

	var paths []string
	if df.FWide != 0 {
		maxWChars := int(remaining / 2)
		wchars := unsafe.Slice((*uint16)(unsafe.Pointer(fileListPtr)), maxWChars)
		for i := 0; i < len(wchars); {
			end := i
			for end < len(wchars) && wchars[end] != 0 {
				end++
			}
			if end == i {
				break
			}
			paths = append(paths, syscall.UTF16ToString(wchars[i:end+1]))
			i = end + 1
		}
	} else {
		ansiBytes := unsafe.Slice((*byte)(unsafe.Pointer(fileListPtr)), remaining)
		for i := 0; i < len(ansiBytes); {
			end := i
			for end < len(ansiBytes) && ansiBytes[end] != 0 {
				end++
			}
			if end == i {
				break
			}
			paths = append(paths, string(ansiBytes[i:end]))
			i = end + 1
		}
	}
	return paths
}

func IsLeftButtonDown() bool {
	r, _, _ := pGetAsyncKeyState.Call(vkLButton)
	return r&0x8000 != 0
}

func setClipboardFiles(files []string) {
	SetClipboardFiles(files)
}

// SetClipboardFiles puts the given file paths on the Windows clipboard as a HDROP
// (shell file list) so the user can Ctrl+V them into Explorer.
func SetClipboardFiles(files []string) {
	if len(files) == 0 {
		return
	}

	const dropFilesSize = 20
	size := uintptr(dropFilesSize)
	var encoded [][]uint16
	for _, file := range files {
		utf16, _ := syscall.UTF16FromString(file)
		encoded = append(encoded, utf16)
		size += uintptr(len(utf16) * 2)
	}
	size += 2

	hg, _, _ := pGlobalAlloc.Call(gmemMoveable, size)
	if hg == 0 {
		log.Println("setClipboardFiles: GlobalAlloc failed")
		return
	}
	ptr, _, _ := pGlobalLock.Call(hg)
	if ptr == 0 {
		pGlobalFree.Call(hg)
		return
	}

	df := (*dropFiles)(unsafe.Pointer(ptr))
	df.PFiles = dropFilesSize
	df.FWide = 1

	dst := ptr + dropFilesSize
	for _, utf16 := range encoded {
		for _, ch := range utf16 {
			*(*uint16)(unsafe.Pointer(dst)) = ch
			dst += 2
		}
	}
	*(*uint16)(unsafe.Pointer(dst)) = 0

	pGlobalUnlock.Call(hg)

	r, _, _ := pOpenClipboard.Call(0)
	if r == 0 {
		pGlobalFree.Call(hg)
		log.Println("setClipboardFiles: OpenClipboard failed")
		return
	}
	pEmptyClipboard.Call()
	if ret, _, _ := pSetClipboardData.Call(uintptr(cfHDrop), hg); ret == 0 {
		pGlobalFree.Call(hg)
		log.Println("setClipboardFiles: SetClipboardData failed")
	}
	pCloseClipboard.Call()
}

func OpenReceivedFiles(paths []string) {
	for _, path := range paths {
		pathUTF16, _ := syscall.UTF16PtrFromString(path)
		pShellExecuteW.Call(0, 0, uintptr(unsafe.Pointer(pathUTF16)), 0, 0, 1)
	}
}
