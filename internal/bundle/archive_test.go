package bundle

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

type archiveTestFileInfo struct {
	mode os.FileMode
}

func (archiveTestFileInfo) Name() string           { return "test" }
func (archiveTestFileInfo) Size() int64            { return 0 }
func (info archiveTestFileInfo) Mode() os.FileMode { return info.mode }
func (archiveTestFileInfo) ModTime() time.Time     { return time.Time{} }
func (info archiveTestFileInfo) IsDir() bool       { return info.mode.IsDir() }
func (archiveTestFileInfo) Sys() any               { return nil }

func TestPathSafetyError(t *testing.T) {
	for _, test := range []struct {
		name string
		mode os.FileMode
		want string
	}{
		{name: "symlink", mode: os.ModeSymlink, want: "symbolic links"},
		{name: "irregular", mode: os.ModeIrregular, want: "irregular path components"},
		{name: "regular"},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := pathSafetyError(archiveTestFileInfo{mode: test.mode})
			if test.want == "" {
				if err != nil {
					t.Fatalf("pathSafetyError() = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("pathSafetyError() = %v, want %q", err, test.want)
			}
		})
	}
}

func TestIndexReturnsSortedVerifiedEntries(t *testing.T) {
	root := t.TempDir()
	archiveRun(t, root, "z-run", runOptions{})
	archiveRun(t, root, "a-run", runOptions{})

	entries, err := Index(root)
	if err != nil {
		t.Fatal(err)
	}
	want := []ArchiveEntry{
		{Directory: "a-run", ManifestName: "experiment-001-email", Differences: 1},
		{Directory: "z-run", ManifestName: "experiment-001-email", Differences: 1},
	}
	if len(entries) != len(want) {
		t.Fatalf("Index() length = %d, want %d", len(entries), len(want))
	}
	for index := range want {
		if entries[index] != want[index] {
			t.Fatalf("Index()[%d] = %#v, want %#v", index, entries[index], want[index])
		}
	}
}

func TestIndexReturnsUnknownSummary(t *testing.T) {
	root := t.TempDir()
	runDir := makeStorageFailureRun(t, "")
	archiveDir := filepath.Join(root, "storage-gap")
	if err := os.Rename(runDir, archiveDir); err != nil {
		t.Fatal(err)
	}
	if _, err := Write(archiveDir); err != nil {
		t.Fatal(err)
	}

	entries, err := Index(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Unknowns != 2 || entries[0].Differences != 0 {
		t.Fatalf("Index() = %#v", entries)
	}
}

func TestIndexRejectsInvalidEntries(t *testing.T) {
	t.Run("empty root", func(t *testing.T) {
		if _, err := Index(" "); err == nil || !strings.Contains(err.Error(), "archive root is required") {
			t.Fatalf("Index() error = %v", err)
		}
	})

	t.Run("missing root", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "missing")
		if _, err := Index(root); err == nil || !strings.Contains(err.Error(), "archive root") {
			t.Fatalf("Index() error = %v", err)
		}
	})

	t.Run("file root", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "file")
		if err := os.WriteFile(root, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := Index(root); err == nil || !strings.Contains(err.Error(), "not a directory") {
			t.Fatalf("Index() error = %v", err)
		}
	})

	t.Run("malformed child", func(t *testing.T) {
		root := t.TempDir()
		if err := os.Mkdir(filepath.Join(root, "malformed"), 0o700); err != nil {
			t.Fatal(err)
		}
		if _, err := Index(root); err == nil || !strings.Contains(err.Error(), "archive entry") {
			t.Fatalf("Index() error = %v", err)
		}
	})

	t.Run("non-directory child", func(t *testing.T) {
		root := t.TempDir()
		if err := os.WriteFile(filepath.Join(root, "not-a-run"), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := Index(root); err == nil || !strings.Contains(err.Error(), "not a directory") {
			t.Fatalf("Index() error = %v", err)
		}
	})

	t.Run("oversized output", func(t *testing.T) {
		root := t.TempDir()
		child := filepath.Join(root, "oversized")
		if err := os.Mkdir(child, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(
			filepath.Join(child, "evidence.json"),
			bytes.Repeat([]byte("x"), maxOutputBytes+1),
			0o600,
		); err != nil {
			t.Fatal(err)
		}
		if _, err := Index(root); err == nil || !strings.Contains(err.Error(), "exceeds") {
			t.Fatalf("Index() error = %v", err)
		}
	})
}

func TestIndexRejectsSymbolicLinks(t *testing.T) {
	t.Run("root", func(t *testing.T) {
		target := t.TempDir()
		link := filepath.Join(t.TempDir(), "archive-link")
		if err := os.Symlink(target, link); err != nil {
			t.Skipf("symbolic links unavailable: %v", err)
		}
		if _, err := Index(link); err == nil || !strings.Contains(err.Error(), "symbolic link") {
			t.Fatalf("Index() error = %v", err)
		}
	})

	t.Run("child", func(t *testing.T) {
		root := t.TempDir()
		target := t.TempDir()
		if err := os.Symlink(target, filepath.Join(root, "linked-run")); err != nil {
			t.Skipf("symbolic links unavailable: %v", err)
		}
		if _, err := Index(root); err == nil || !strings.Contains(err.Error(), "symbolic link") {
			t.Fatalf("Index() error = %v", err)
		}
	})

	t.Run("artifact", func(t *testing.T) {
		root := t.TempDir()
		runDir := archiveRun(t, root, "linked-artifact", runOptions{})
		outside := filepath.Join(t.TempDir(), "network.json")
		if err := os.WriteFile(outside, []byte(`{"request":"outside"}`), 0o600); err != nil {
			t.Fatal(err)
		}
		artifact := filepath.Join(runDir, "baseline", "observations", "network.json")
		if err := os.Remove(artifact); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outside, artifact); err != nil {
			t.Skipf("symbolic links unavailable: %v", err)
		}
		if _, err := Index(root); err == nil || !strings.Contains(err.Error(), "symbolic links are not allowed") {
			t.Fatalf("Index() error = %v", err)
		}
	})

	t.Run("junction", func(t *testing.T) {
		if runtime.GOOS != "windows" {
			t.Skip("junctions are Windows-specific")
		}
		root := t.TempDir()
		runDir := archiveRun(t, root, "linked-junction", runOptions{})
		outside := filepath.Join(t.TempDir(), "observations")
		if err := os.Rename(
			filepath.Join(runDir, "baseline", "observations"),
			outside,
		); err != nil {
			t.Fatal(err)
		}
		link := filepath.Join(runDir, "baseline", "observations")
		if err := exec.Command("cmd.exe", "/c", "mklink", "/J", link, outside).Run(); err != nil {
			t.Skipf("junctions unavailable: %v", err)
		}
		if _, err := Index(root); err == nil || !strings.Contains(err.Error(), "irregular path components") {
			t.Fatalf("Index() error = %v", err)
		}
	})

	t.Run("intermediate", func(t *testing.T) {
		root := t.TempDir()
		runDir := archiveRun(t, root, "linked-intermediate", runOptions{})
		outside := filepath.Join(t.TempDir(), "observations")
		if err := os.Rename(
			filepath.Join(runDir, "baseline", "observations"),
			outside,
		); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outside, filepath.Join(runDir, "baseline", "observations")); err != nil {
			t.Skipf("symbolic links unavailable: %v", err)
		}
		if _, err := Index(root); err == nil || !strings.Contains(err.Error(), "symbolic links are not allowed") {
			t.Fatalf("Index() error = %v", err)
		}
	})
}

func TestValidArchiveEntryName(t *testing.T) {
	for _, name := range []string{"", ".", "..", "nested/run", `nested\run`, "bad\nname", strings.Repeat("a", 256)} {
		if validArchiveEntryName(name) {
			t.Fatalf("validArchiveEntryName(%q) = true", name)
		}
	}
	for _, name := range []string{"run-001", "archive copy"} {
		if !validArchiveEntryName(name) {
			t.Fatalf("validArchiveEntryName(%q) = false", name)
		}
	}
}

func archiveRun(t *testing.T, root, name string, options runOptions) string {
	t.Helper()
	runDir := makeRun(t, options)
	archiveDir := filepath.Join(root, name)
	if err := os.Rename(runDir, archiveDir); err != nil {
		t.Fatal(err)
	}
	if _, err := Write(archiveDir); err != nil {
		t.Fatal(err)
	}
	return archiveDir
}
