//go:build windows

package securefs

import (
	"errors"
	"os"
	"syscall"
	"unsafe"
)

var moveFileExW = syscall.NewLazyDLL("kernel32.dll").NewProc("MoveFileExW")

const (
	moveFileWriteThrough      = uintptr(0x8)
	windowsErrorFileExists    = syscall.Errno(80)
	windowsErrorAlreadyExists = syscall.Errno(183)
)

func renameNoReplace(oldPath, newPath string) error {
	oldPtr, err := syscall.UTF16PtrFromString(oldPath)
	if err != nil {
		return err
	}
	newPtr, err := syscall.UTF16PtrFromString(newPath)
	if err != nil {
		return err
	}
	result, _, callErr := moveFileExW.Call(
		uintptr(unsafe.Pointer(oldPtr)),
		uintptr(unsafe.Pointer(newPtr)),
		moveFileWriteThrough,
	)
	if result != 0 {
		return nil
	}
	if errno, ok := callErr.(syscall.Errno); ok &&
		(errno == windowsErrorFileExists || errno == windowsErrorAlreadyExists) {
		return os.ErrExist
	}
	if callErr == nil {
		return errors.New("MoveFileExW failed without an error")
	}
	return callErr
}
