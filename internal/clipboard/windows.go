//go:build windows && !headless

package clipboard

import (
	"errors"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	cfUnicodeText = 13
	gmemMoveable  = 0x0002
)

var (
	user32               = windows.NewLazySystemDLL("user32.dll")
	kernel32             = windows.NewLazySystemDLL("kernel32.dll")
	procOpenClipboard    = user32.NewProc("OpenClipboard")
	procCloseClipboard   = user32.NewProc("CloseClipboard")
	procEmptyClipboard   = user32.NewProc("EmptyClipboard")
	procGetClipboardData = user32.NewProc("GetClipboardData")
	procSetClipboardData = user32.NewProc("SetClipboardData")
	procGlobalAlloc      = kernel32.NewProc("GlobalAlloc")
	procGlobalFree       = kernel32.NewProc("GlobalFree")
	procGlobalLock       = kernel32.NewProc("GlobalLock")
	procGlobalUnlock     = kernel32.NewProc("GlobalUnlock")
)

func Init() error {
	return nil
}

func ReadText() ([]byte, error) {
	if err := openClipboard(); err != nil {
		return nil, err
	}
	defer closeClipboard()

	handle, _, err := procGetClipboardData.Call(cfUnicodeText)
	if handle == 0 {
		if err != windows.ERROR_SUCCESS {
			return nil, err
		}
		return nil, nil
	}
	ptr, _, err := procGlobalLock.Call(handle)
	if ptr == 0 {
		return nil, err
	}
	defer procGlobalUnlock.Call(handle)

	utf16Data := unsafe.Slice((*uint16)(unsafe.Pointer(ptr)), utf16StringLen((*uint16)(unsafe.Pointer(ptr)))+1)
	return []byte(string(utf16.Decode(utf16Data[:len(utf16Data)-1]))), nil
}

func WriteText(data []byte) error {
	if err := openClipboard(); err != nil {
		return err
	}
	defer closeClipboard()

	if r1, _, err := procEmptyClipboard.Call(); r1 == 0 {
		return err
	}

	encoded := utf16.Encode([]rune(string(data) + "\x00"))
	size := uintptr(len(encoded) * 2)
	handle, _, err := procGlobalAlloc.Call(gmemMoveable, size)
	if handle == 0 {
		return err
	}

	ptr, _, err := procGlobalLock.Call(handle)
	if ptr == 0 {
		procGlobalFree.Call(handle)
		return err
	}
	copy(unsafe.Slice((*uint16)(unsafe.Pointer(ptr)), len(encoded)), encoded)
	procGlobalUnlock.Call(handle)

	if r1, _, err := procSetClipboardData.Call(cfUnicodeText, handle); r1 == 0 {
		procGlobalFree.Call(handle)
		return err
	}
	return nil
}

func openClipboard() error {
	if r1, _, err := procOpenClipboard.Call(0); r1 == 0 {
		if err == windows.ERROR_SUCCESS {
			return errors.New("OpenClipboard failed")
		}
		return err
	}
	return nil
}

func closeClipboard() {
	procCloseClipboard.Call()
}

func utf16StringLen(ptr *uint16) int {
	n := 0
	for *(*uint16)(unsafe.Pointer(uintptr(unsafe.Pointer(ptr)) + uintptr(n)*2)) != 0 {
		n++
	}
	return n
}
