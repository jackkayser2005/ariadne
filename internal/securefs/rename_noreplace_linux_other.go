//go:build linux && !amd64 && !arm64

package securefs

func renameNoReplace(_, _ string) error {
	return errAtomicRenameUnsupported
}
