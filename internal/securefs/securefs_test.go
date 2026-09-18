package securefs

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestValidateDirectoryAndRequireAbsent(t *testing.T) {
	root := t.TempDir()
	if err := ValidateDirectory(root); err != nil {
		t.Fatalf("ValidateDirectory() error = %v", err)
	}
	target := filepath.Join(root, "new")
	if err := RequireAbsent(target); err != nil {
		t.Fatalf("RequireAbsent() error = %v", err)
	}
	if err := os.WriteFile(target, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := RequireAbsent(target); !errors.Is(err, os.ErrExist) {
		t.Fatalf("RequireAbsent() error = %v, want os.ErrExist", err)
	}
	if err := ValidateDirectory(target); err == nil {
		t.Fatal("ValidateDirectory() accepted a regular file")
	}
}

func TestPublishDirectoryExclusivePublishesWithoutReplacement(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "staging")
	destination := filepath.Join(root, "published")
	if err := MkdirExclusive(source, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "artifact.json"), []byte("safe"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := PublishDirectoryExclusive(source, destination); err != nil {
		if errors.Is(err, errAtomicRenameUnsupported) {
			t.Skipf("atomic no-replace publication is unsupported: %v", err)
		}
		t.Fatalf("PublishDirectoryExclusive() error = %v", err)
	}
	if _, err := os.Stat(source); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("source still exists, err = %v", err)
	}
	data, err := os.ReadFile(filepath.Join(destination, "artifact.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "safe" {
		t.Fatalf("published data = %q, want safe", data)
	}
}

func TestPublishDirectoryExclusiveRejectsExistingDestination(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "staging")
	destination := filepath.Join(root, "published")
	if err := MkdirExclusive(source, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "artifact.json"), []byte("safe"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(destination, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := PublishDirectoryExclusive(source, destination); !errors.Is(err, os.ErrExist) {
		t.Fatalf("PublishDirectoryExclusive() error = %v, want os.ErrExist", err)
	}
	if _, err := os.Stat(filepath.Join(source, "artifact.json")); err != nil {
		t.Fatalf("source changed after collision: %v", err)
	}
	entries, err := os.ReadDir(destination)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("existing destination changed: %#v", entries)
	}
}

func TestWriteExclusiveCreatesNestedOutput(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "nested", "artifact.json")
	if err := WriteExclusive(path, []byte("safe"), 0o600); err != nil {
		t.Fatalf("WriteExclusive() error = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(data) != "safe" {
		t.Fatalf("ReadFile() = %q, want safe", data)
	}
}

func TestWriteExclusiveExistingParentRequiresExistingDirectory(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "nested", "artifact.json")
	if err := WriteExclusiveExistingParent(path, []byte("safe"), 0o600); err == nil {
		t.Fatal("WriteExclusiveExistingParent() created a missing parent")
	}
	if err := os.Mkdir(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := WriteExclusiveExistingParent(path, []byte("safe"), 0o600); err != nil {
		t.Fatalf("WriteExclusiveExistingParent() error = %v", err)
	}
}

func TestWriteExclusiveRejectsExistingLeaf(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "artifact.json")
	if err := os.WriteFile(path, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := WriteExclusive(path, []byte("replacement"), 0o600); !errors.Is(err, os.ErrExist) {
		t.Fatalf("WriteExclusive() error = %v, want os.ErrExist", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "original" {
		t.Fatalf("existing output changed to %q", data)
	}
}

func TestWriteExclusiveRejectsSymlinkedParent(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	link := filepath.Join(root, "linked")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	path := filepath.Join(link, "artifact.json")
	if err := WriteExclusive(path, []byte("secret"), 0o600); err == nil {
		t.Fatal("WriteExclusive() accepted a symlinked parent")
	}
	if _, err := os.Lstat(filepath.Join(outside, "artifact.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("redirected output exists: %v", err)
	}
}

func TestWriteExclusiveRejectsSymlinkedLeaf(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target")
	link := filepath.Join(root, "artifact.json")
	if err := os.WriteFile(target, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := WriteExclusive(link, []byte("replacement"), 0o600); err == nil {
		t.Fatal("WriteExclusive() accepted a symlinked leaf")
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "original" {
		t.Fatalf("symlink target changed to %q", data)
	}
}

func TestMkdirAllRejectsFileComponent(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "file")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := MkdirAll(filepath.Join(file, "child"), 0o700); err == nil {
		t.Fatal("MkdirAll() accepted a regular file component")
	}
}

func TestPublicationRejectsInvalidPathsWithoutMovingSource(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	if err := MkdirExclusive(source, 0700); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(root, "file")
	if err := os.WriteFile(file, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, pair := range [][2]string{{"", source}, {source, ""}, {source, source}, {file, filepath.Join(root, "new")}, {filepath.Join(root, "missing"), filepath.Join(root, "new")}, {source, filepath.Join(file, "new")}, {source, filepath.Join(root, "missing", "new")}} {
		if err := PublishDirectoryExclusive(pair[0], pair[1]); err == nil {
			t.Fatalf("accepted invalid paths %#v", pair)
		}
		if err := ValidateDirectory(source); err != nil {
			t.Fatalf("source lost: %v", err)
		}
	}
	if err := MkdirAll("", 0700); err == nil {
		t.Fatal("empty mkdir")
	}
	if err := MkdirExclusive("", 0700); err == nil {
		t.Fatal("empty exclusive mkdir")
	}
	if err := MkdirExclusive(source, 0700); !errors.Is(err, os.ErrExist) {
		t.Fatal("existing directory replaced", err)
	}
	if err := MkdirExclusive(filepath.Join(file, "new"), 0700); err == nil {
		t.Fatal("file parent")
	}
	if err := ValidateDirectory(""); err == nil {
		t.Fatal("empty validation")
	}
	if err := RequireAbsent(""); err == nil {
		t.Fatal("empty absent")
	}
	if err := WriteExclusive("", nil, 0600); err == nil {
		t.Fatal("empty write")
	}
}

func TestNoReplacePrimitiveRejectsLateCollision(t *testing.T) {
	root := t.TempDir()
	source, target := filepath.Join(root, "source"), filepath.Join(root, "target")
	if err := os.Mkdir(source, 0700); err != nil {
		t.Fatal(err)
	}
	if err := RequireAbsent(target); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(target, 0700); err != nil {
		t.Fatal(err)
	}
	if err := renameNoReplace(source, target); err == nil {
		t.Fatal("replaced late destination")
	}
	if _, err := os.Stat(source); err != nil {
		t.Fatal("removed source", err)
	}
	if err := renameNoReplace("bad\x00path", target); err == nil {
		t.Fatal("invalid source path accepted")
	}
	if err := renameNoReplace(source, "bad\x00path"); err == nil {
		t.Fatal("invalid destination path accepted")
	}
	if err := renameNoReplace(filepath.Join(root, "missing"), filepath.Join(root, "new")); err == nil {
		t.Fatal("missing source accepted")
	}
}

func TestFailedWriteCleanupPreservesReplacedFile(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "artifact")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	old, err := file.Stat()
	if err != nil {
		t.Fatal(err)
	}
	file.Close()
	parent, err := directorySnapshot(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := verifyOpenedPath(path, root, parent, old); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(path, filepath.Join(root, "original")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("replacement"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := verifyOpenedPath(path, root, parent, old); !IsPathSafetyError(err) {
		t.Fatal("replacement accepted", err)
	}
	removeIfSame(path, nil)
	removeIfSame(path, old)
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "replacement" {
		t.Fatal("removed replacement", err)
	}
	current, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	removeIfSame(path, current)
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("same file not removed", err)
	}
	removeIfSame(path, old)
	if err := verifyOpenedPath(path, root, parent, old); err == nil {
		t.Fatal("missing path accepted")
	}
	if err := verifyOpenedPath(path, filepath.Join(root, "missing"), parent, old); err == nil {
		t.Fatal("missing parent accepted")
	}
	if err := writeAll(file, []byte("no")); err == nil {
		t.Fatal("closed file write accepted")
	}
}
