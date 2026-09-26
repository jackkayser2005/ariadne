package browser

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestJourneyCleanupWaitsForDelayedWindowsProfileHandles(t *testing.T) {
	profile := t.TempDir()
	file := filepath.Join(profile, "closing-browser-file")
	if err := os.WriteFile(file, []byte("synthetic"), 0600); err != nil {
		t.Fatal(err)
	}
	name, err := syscall.UTF16PtrFromString(file)
	if err != nil {
		t.Fatal(err)
	}
	handle, err := syscall.CreateFile(name, syscall.GENERIC_READ, syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE, nil, syscall.OPEN_EXISTING, syscall.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		t.Fatal(err)
	}
	released := make(chan struct{})
	go func() { time.Sleep(1200 * time.Millisecond); syscall.CloseHandle(handle); close(released) }()
	defer func() { <-released }()
	done := make(chan struct{})
	close(done)
	process := &browserProcess{profile: profile, done: done}
	if err := process.close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(profile); !os.IsNotExist(err) {
		t.Fatal("temporary profile remains after its last handle closed")
	}
}
