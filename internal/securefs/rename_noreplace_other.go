//go:build !linux && !darwin && !windows

package securefs

func renameNoReplace(_, _ string) error {
	return errAtomicRenameUnsupported
}
