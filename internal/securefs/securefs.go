// Package securefs provides bounded, no-symlink filesystem publication helpers.
package securefs

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

var errUnsafePath = errors.New("unsafe filesystem path")

// IsPathSafetyError reports whether err identifies a symlink, reparse point, or
// path replacement rejected by this package.
func IsPathSafetyError(err error) bool {
	return errors.Is(err, errUnsafePath)
}

// MkdirAll creates path one component at a time after rejecting symlink,
// reparse-point, irregular, and non-directory components.
func MkdirAll(path string, perm os.FileMode) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("directory path is required")
	}
	return ensureDirectories(path, perm)
}

// MkdirExclusive creates one new directory after validating every parent
// component. It fails when the requested directory already exists.
func MkdirExclusive(path string, perm os.FileMode) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("directory path is required")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("resolve directory path: %w", err)
	}
	parent := filepath.Dir(absolute)
	if err := MkdirAll(parent, 0o700); err != nil {
		return fmt.Errorf("validate directory parent: %w", err)
	}
	if err := os.Mkdir(absolute, perm); err != nil {
		return err
	}
	if _, err := lstatNoSymlinkPath(absolute); err != nil {
		return fmt.Errorf("verify directory: %w", err)
	}
	return nil
}

// PublishDirectoryExclusive atomically moves sourceDir to destinationDir
// without replacing an existing destination. Both paths must share a validated
// parent filesystem; unsupported platforms fail closed.
func PublishDirectoryExclusive(sourceDir, destinationDir string) error {
	if strings.TrimSpace(sourceDir) == "" || strings.TrimSpace(destinationDir) == "" {
		return errors.New("source and destination directories are required")
	}
	source, err := filepath.Abs(sourceDir)
	if err != nil {
		return fmt.Errorf("resolve source directory: %w", err)
	}
	destination, err := filepath.Abs(destinationDir)
	if err != nil {
		return fmt.Errorf("resolve destination directory: %w", err)
	}
	if source == destination {
		return errors.New("source and destination directories must differ")
	}
	sourceInfo, err := directorySnapshot(source)
	if err != nil {
		return fmt.Errorf("validate source directory: %w", err)
	}
	parent := filepath.Dir(destination)
	parentInfo, err := directorySnapshot(parent)
	if err != nil {
		return fmt.Errorf("validate destination parent: %w", err)
	}
	if err := RequireAbsent(destination); err != nil {
		return fmt.Errorf("validate destination: %w", err)
	}
	if err := renameNoReplace(source, destination); err != nil {
		return fmt.Errorf("publish directory without replacement: %w", err)
	}
	currentParent, err := requireDirectory(parent)
	if err != nil {
		return fmt.Errorf("verify destination parent: %w", err)
	}
	if !os.SameFile(parentInfo, currentParent) {
		return fmt.Errorf("destination parent changed during publication: %w", errUnsafePath)
	}
	destinationInfo, err := requireDirectory(destination)
	if err != nil {
		return fmt.Errorf("verify published directory: %w", err)
	}
	if !os.SameFile(sourceInfo, destinationInfo) {
		return fmt.Errorf("published directory identity changed: %w", errUnsafePath)
	}
	return nil
}

// ValidateDirectory verifies that path is an existing directory with no
// symlink, reparse-point, or irregular component.
func ValidateDirectory(path string) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("directory path is required")
	}
	_, err := requireDirectory(path)
	return err
}

// RequireAbsent verifies that path does not exist and that its parent is a
// safe existing directory.
func RequireAbsent(path string) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("path is required")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("resolve path: %w", err)
	}
	if _, err := requireDirectory(filepath.Dir(absolute)); err != nil {
		return fmt.Errorf("validate parent: %w", err)
	}
	info, err := os.Lstat(absolute)
	if err == nil {
		if safetyErr := pathSafetyError(info); safetyErr != nil {
			return safetyErr
		}
		return fmt.Errorf("path already exists: %w", os.ErrExist)
	}
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// WriteExclusive creates path without overwriting an existing leaf. It also
// creates missing parent directories through the same no-symlink walk.
func WriteExclusive(path string, data []byte, perm os.FileMode) error {
	return writeExclusive(path, data, perm, true)
}

// WriteExclusiveExistingParent creates path without creating its parent. It
// validates every existing parent component before and after the write.
func WriteExclusiveExistingParent(path string, data []byte, perm os.FileMode) error {
	return writeExclusive(path, data, perm, false)
}

func writeExclusive(path string, data []byte, perm os.FileMode, createParent bool) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("output path is required")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("resolve output path: %w", err)
	}
	parent := filepath.Dir(absolute)
	var parentInfo os.FileInfo
	if createParent {
		if err := MkdirAll(parent, 0o700); err != nil {
			return fmt.Errorf("create output directory: %w", err)
		}
	}
	parentInfo, err = requireDirectory(parent)
	if err != nil {
		return fmt.Errorf("validate output directory: %w", err)
	}
	if info, statErr := os.Lstat(absolute); statErr == nil {
		if safetyErr := pathSafetyError(info); safetyErr != nil {
			return safetyErr
		}
		return fmt.Errorf("file exists: %w", os.ErrExist)
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return fmt.Errorf("inspect output: %w", statErr)
	}

	file, err := os.OpenFile(absolute, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm)
	if err != nil {
		return fmt.Errorf("create output: %w", err)
	}
	var openedInfo os.FileInfo
	remove := true
	defer func() {
		_ = file.Close()
		if remove {
			removeIfSame(absolute, openedInfo)
		}
	}()

	openedInfo, err = file.Stat()
	if err != nil {
		return fmt.Errorf("stat output: %w", err)
	}
	if !openedInfo.Mode().IsRegular() {
		return fmt.Errorf("output must be a regular file: %w", errUnsafePath)
	}
	if err := verifyOpenedPath(absolute, parent, parentInfo, openedInfo); err != nil {
		return err
	}
	if err := writeAll(file, data); err != nil {
		return fmt.Errorf("write output: %w", err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync output: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close output: %w", err)
	}
	if err := verifyOpenedPath(absolute, parent, parentInfo, openedInfo); err != nil {
		return err
	}
	remove = false
	return nil
}

func ensureDirectories(path string, perm os.FileMode) error {
	_, root, components, err := splitAbsolute(path)
	if err != nil {
		return err
	}
	if err := validateDirectoryRoot(root); err != nil {
		return err
	}
	current := root
	for _, component := range components {
		if component == "" || component == "." {
			continue
		}
		current = filepath.Join(current, component)
		info, err := os.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			if err := os.Mkdir(current, perm); err != nil && !errors.Is(err, os.ErrExist) {
				return fmt.Errorf("create %s: %w", current, err)
			}
			info, err = os.Lstat(current)
		}
		if err != nil {
			return fmt.Errorf("inspect %s: %w", current, err)
		}
		if err := pathSafetyError(info); err != nil {
			return fmt.Errorf("inspect %s: %w", current, err)
		}
		if !info.IsDir() {
			return fmt.Errorf("%s is not a directory", current)
		}
	}
	return nil
}

func requireDirectory(path string) (os.FileInfo, error) {
	info, err := lstatNoSymlinkPath(path)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, errors.New("directory required")
	}
	return info, nil
}

func validateDirectoryRoot(path string) error {
	_, err := requireDirectory(path)
	return err
}

func splitAbsolute(path string) (string, string, []string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", "", nil, err
	}
	volume := filepath.VolumeName(absolute)
	remainder := strings.TrimPrefix(absolute, volume)
	root := volume
	separator := string(filepath.Separator)
	if strings.HasPrefix(remainder, separator) {
		root += separator
		remainder = strings.TrimPrefix(remainder, separator)
	}
	if root == "" {
		root = separator
	}
	components := strings.Split(remainder, separator)
	return absolute, root, components, nil
}

func lstatNoSymlinkPath(path string) (os.FileInfo, error) {
	_, root, components, err := splitAbsolute(path)
	if err != nil {
		return nil, err
	}
	info, err := os.Lstat(root)
	if err != nil {
		return nil, err
	}
	if err := pathSafetyError(info); err != nil {
		return nil, err
	}
	current := root
	for _, component := range components {
		if component == "" || component == "." {
			continue
		}
		current = filepath.Join(current, component)
		info, err = os.Lstat(current)
		if err != nil {
			return nil, err
		}
		if err := pathSafetyError(info); err != nil {
			return nil, err
		}
	}
	return info, nil
}

func verifyOpenedPath(path, parent string, parentInfo, openedInfo os.FileInfo) error {
	currentParent, err := requireDirectory(parent)
	if err != nil {
		return fmt.Errorf("verify output directory: %w", err)
	}
	if !os.SameFile(parentInfo, currentParent) {
		return fmt.Errorf("output directory changed during write: %w", errUnsafePath)
	}
	current, err := lstatNoSymlinkPath(path)
	if err != nil {
		return fmt.Errorf("verify output path: %w", err)
	}
	if !current.Mode().IsRegular() || !os.SameFile(current, openedInfo) {
		return fmt.Errorf("output path changed during write: %w", errUnsafePath)
	}
	return nil
}

func removeIfSame(path string, expected os.FileInfo) {
	if expected == nil {
		return
	}
	current, err := lstatNoSymlinkPath(path)
	if err != nil || !os.SameFile(current, expected) {
		return
	}
	_ = os.Remove(path)
}

func pathSafetyError(info os.FileInfo) error {
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("symbolic links are not allowed: %w", errUnsafePath)
	}
	if info.Mode()&os.ModeIrregular != 0 {
		return fmt.Errorf("reparse points and other irregular path components are not allowed: %w", errUnsafePath)
	}
	return nil
}

func writeAll(file *os.File, data []byte) error {
	for len(data) > 0 {
		written, err := file.Write(data)
		if written > 0 {
			data = data[written:]
		}
		if err != nil {
			return err
		}
		if written == 0 {
			return io.ErrShortWrite
		}
	}
	return nil
}

// directorySnapshot captures file identity through an open handle. On Windows,
// Lstat defers reading the file ID until SameFile; after a rename its old path
// no longer resolves. Stat on the handle records the identity before publication.
func directorySnapshot(path string) (os.FileInfo, error) {
	before, err := requireDirectory(path)
	if err != nil {
		return nil, err
	}
	directory, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer directory.Close()
	snapshot, err := directory.Stat()
	if err != nil {
		return nil, err
	}
	if !snapshot.IsDir() || !os.SameFile(before, snapshot) {
		return nil, fmt.Errorf("directory changed before publication: %w", errUnsafePath)
	}
	return snapshot, nil
}
