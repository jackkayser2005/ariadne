package bundle

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadBoundedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "receipt.json")
	if err := os.WriteFile(path, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	data, err := ReadBoundedFile(path, 16)
	if err != nil {
		t.Fatalf("ReadBoundedFile() error = %v", err)
	}
	if string(data) != "{}\n" {
		t.Fatalf("ReadBoundedFile() = %q", data)
	}
}
func TestReadBoundedFileSafetyBoundary(t *testing.T) {
	root := t.TempDir()
	regular := filepath.Join(root, "regular.json")
	if err := os.WriteFile(regular, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if IsPathSafetyError(nil) {
		t.Fatal("IsPathSafetyError(nil) = true")
	}
	data, err := ReadBoundedFile(regular, 16)
	if err != nil {
		t.Fatalf("ReadBoundedFile() error = %v", err)
	}
	if string(data) != "{}" {
		t.Fatalf("ReadBoundedFile() = %q", data)
	}

	missing, err := ReadBoundedFile(filepath.Join(root, "missing.json"), 16)
	if err == nil || missing != nil || IsPathSafetyError(err) {
		t.Fatalf("missing ReadBoundedFile() = data %v, error %v", missing, err)
	}

	if _, err := ReadBoundedFile(root, 16); err == nil || !IsPathSafetyError(err) {
		t.Fatalf("directory ReadBoundedFile() = %v, want path-safety error", err)
	}

	target := filepath.Join(root, "target.json")
	link := filepath.Join(root, "link.json")
	if err := os.WriteFile(target, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := ReadBoundedFile(link, 16); err == nil || !IsPathSafetyError(err) {
		t.Fatalf("symlink ReadBoundedFile() = %v, want path-safety error", err)
	}
}
