//go:build darwin

package securefs

import (
	"syscall"
	"unsafe"
)

const darwinRenameatxNPSyscall = uintptr(488)
const darwinAtFDCWD = uintptr(^uint(1))
const darwinRenameExclusive = uintptr(4)

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
		darwinRenameatxNPSyscall,
		darwinAtFDCWD,
		uintptr(unsafe.Pointer(oldPtr)),
		darwinAtFDCWD,
		uintptr(unsafe.Pointer(newPtr)),
		darwinRenameExclusive,
		0,
	)
	if errno != 0 {
		return errno
	}
	return nil
}
