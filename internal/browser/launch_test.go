package browser

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestInvestigationURLRequiresExplicitSafeOrigin(t *testing.T) {
	for _, raw := range []string{"", "example.com", "file:///private", "https://user:password@example.com", "https://example.com/#fragment", "http://example.com:0", "http://example.com:65536", "http://example.com:no", "http://example.com/\nprivate", "http://example.com/\x00", strings.Repeat("a", 4097)} {
		if _, err := InvestigationURL(raw); err == nil {
			t.Errorf("accepted %q", raw)
		}
	}
	for raw, want := range map[string]string{"https://EXAMPLE.com:443/path?q=test": "https://example.com/path?q=test", "http://EXAMPLE.com:80": "http://example.com", "http://[::1]:80/": "http://[::1]/", "https://example.com:8443/": "https://example.com:8443/"} {
		u, err := InvestigationURL(raw)
		if err != nil || u.String() != want {
			t.Errorf("%s: %v, %v", raw, u, err)
		}
	}
	for _, address := range []string{"https://127.0.0.1", "http://example.com", "http://192.0.2.1", "%", "file:///private"} {
		if err := OpenInterface(address); err == nil {
			t.Errorf("opened untrusted interface %q", address)
		}
	}
}

func TestBrowserDiscoveryAndStartupFailureAreActionable(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("macOS also searches fixed application roots")
	}
	for _, name := range []string{"ProgramFiles", "ProgramFiles(x86)", "LOCALAPPDATA", "PATH"} {
		t.Setenv(name, "")
	}
	if _, err := FindBrowser(); err == nil || !strings.Contains(err.Error(), "install Chrome or Edge") {
		t.Fatalf("missing browser: %v", err)
	}
	if _, _, err := launchBrowser(context.Background(), true); err == nil {
		t.Fatal("launched absent browser")
	}
	if err := OpenInterface("http://127.0.0.1:8787"); err == nil {
		t.Fatal("opened absent browser")
	}
	root := t.TempDir()
	path := filepath.Join(root, "google-chrome")
	if runtime.GOOS == "windows" {
		path = filepath.Join(root, "Google", "Chrome", "Application", "chrome.exe")
		t.Setenv("ProgramFiles", root)
	} else {
		t.Setenv("PATH", root)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("invalid executable"), 0700); err != nil {
		t.Fatal(err)
	}
	if found, err := FindBrowser(); err != nil || found != path {
		t.Fatalf("discovery: %s %v", found, err)
	}
	if _, _, err := launchBrowser(context.Background(), true); err == nil {
		t.Fatal("accepted invalid browser executable")
	}
	if err := OpenInterface("http://127.0.0.1:8787"); err == nil {
		t.Fatal("opened invalid browser executable")
	}
}
