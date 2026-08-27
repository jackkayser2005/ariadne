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
