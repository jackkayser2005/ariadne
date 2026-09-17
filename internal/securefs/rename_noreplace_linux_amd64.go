//go:build linux && amd64

package securefs

import (
	"syscall"
	"unsafe"
)

const linuxRenameat2Syscall = uintptr(316)
const linuxAtFDCWD = uintptr(^uint(99))
const linuxRenameNoReplace = uintptr(1)

func renameNoReplace(oldPath, newPath string) error {
	oldPtr, err := syscall.BytePtrFromString(oldPath)
	if err != nil {
		return err
	}
	newPtr, err := syscall.BytePtrFromString(newPath)
	if err != nil {
		return err
	}
	_, _, errno := syscall.RawSyscall6(
		linuxRenameat2Syscall,
		linuxAtFDCWD,
		uintptr(unsafe.Pointer(oldPtr)),
		linuxAtFDCWD,
		uintptr(unsafe.Pointer(newPtr)),
		linuxRenameNoReplace,
		0,
	)
	if errno != 0 {
		return errno
	}
	return nil
}
