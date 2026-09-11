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
