package browser

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// FindBrowser discovers a locally installed Chrome or Edge without a user path.
func FindBrowser() (string, error) {
	candidates := []string{}
	if runtime.GOOS == "windows" {
		for _, root := range []string{os.Getenv("ProgramFiles"), os.Getenv("ProgramFiles(x86)"), os.Getenv("LOCALAPPDATA")} {
			if root != "" {
				for _, relative := range []string{"Google/Chrome/Application/chrome.exe", "Microsoft/Edge/Application/msedge.exe"} {
					candidates = append(candidates, filepath.Join(root, filepath.FromSlash(relative)))
				}
			}
		}
	} else if runtime.GOOS == "darwin" {
		candidates = append(candidates, "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome", "/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge")
	}
	for _, name := range []string{"google-chrome", "chromium", "chromium-browser", "microsoft-edge", "chrome", "msedge"} {
		if path, err := exec.LookPath(name); err == nil {
			candidates = append(candidates, path)
		}
	}
	for _, path := range candidates {
		if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() {
			return path, nil
		}
	}
	return "", errors.New("install Chrome or Edge to start a browser investigation")
}

// InvestigationURL accepts only explicit HTTP(S) sites without embedded credentials.
func InvestigationURL(raw string) (*url.URL, error) {
	if len(raw) > 4096 || strings.ContainsAny(raw, "\r\n\x00") {
		return nil, errors.New("website address is invalid")
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil || u.Opaque != "" || u.Fragment != "" {
		return nil, errors.New("enter a complete http:// or https:// website address without a password or fragment")
	}
	if u.Port() != "" {
		port, err := strconv.Atoi(u.Port())
		if err != nil || port < 1 || port > 65535 {
			return nil, errors.New("website port is invalid")
		}
	}
	u.Host = strings.ToLower(u.Host)
	if (u.Scheme == "https" && u.Port() == "443") || (u.Scheme == "http" && u.Port() == "80") {
		u.Host = u.Hostname()
		if strings.Contains(u.Host, ":") {
			u.Host = "[" + u.Host + "]"
		}
	}
	return u, nil
}

type browserProcess struct {
	command *exec.Cmd
	profile string
	done    chan struct{}
}

func launchBrowser(ctx context.Context, headless bool) (*browserProcess, string, error) {
	path, err := FindBrowser()
	if err != nil {
		return nil, "", err
	}
	profile, err := os.MkdirTemp("", "ariadne-browser-")
	if err != nil {
		return nil, "", errors.New("could not create a fresh browser profile")
	}
	arguments := []string{"--user-data-dir=" + profile, "--remote-debugging-port=0", "--remote-debugging-address=127.0.0.1", "--no-first-run", "--no-default-browser-check", "--disable-background-networking", "--disable-sync", "--disable-extensions", "--password-store=basic", "about:blank"}
	if headless {
		arguments = append([]string{"--headless=new"}, arguments...)
	}
	command := exec.Command(path, arguments...)
	process := &browserProcess{command: command, profile: profile, done: make(chan struct{})}
	if err := command.Start(); err != nil {
		_ = os.RemoveAll(profile)
		return nil, "", errors.New("could not start the browser")
	}
	go func() { _ = command.Wait(); close(process.done) }()
	timer := time.NewTimer(15 * time.Second)
	defer timer.Stop()
	tick := time.NewTicker(50 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			process.close()
			return nil, "", ctx.Err()
		case <-process.done:
			process.close()
			return nil, "", errors.New("browser stopped before recording could start")
		case <-timer.C:
			process.close()
			return nil, "", errors.New("browser did not become ready")
		case <-tick.C:
			data, err := readInvestigationFile(filepath.Join(profile, "DevToolsActivePort"), 1024)
			if err != nil || len(data) > 1024 {
				continue
			}
			lines := strings.Split(strings.TrimSpace(string(data)), "\n")
			if len(lines) != 2 {
				continue
			}
			port, err := strconv.Atoi(strings.TrimSpace(lines[0]))
			if err != nil || port < 1 || port > 65535 || !strings.HasPrefix(lines[1], "/devtools/browser/") || strings.ContainsAny(lines[1], "?#\r\t ") {
				continue
			}
			return process, "ws://" + net.JoinHostPort("127.0.0.1", strconv.Itoa(port)) + lines[1], nil
		}
	}
}

func (p *browserProcess) close() error {
	select {
	case <-p.done:
	default:
		if runtime.GOOS == "windows" {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			_ = exec.CommandContext(ctx, "taskkill.exe", "/PID", strconv.Itoa(p.command.Process.Pid), "/T", "/F").Run()
			cancel()
		}
		_ = p.command.Process.Kill()
		select {
		case <-p.done:
		case <-time.After(5 * time.Second):
			return errors.New("browser process cleanup did not finish")
		}
	}
	// Only our randomly created, owned profile is removed. Chrome can release its
	// last file handles briefly after exiting on Windows.
	for attempt := 0; attempt < 20; attempt++ {
		if err := os.RemoveAll(p.profile); err == nil {
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return errors.New("browser closed but its temporary profile could not be removed")
}

// OpenInterface opens the trusted loopback interface in the supported browser.
func OpenInterface(address string) error {
	u, err := url.Parse(address)
	if err != nil || u.Scheme != "http" || net.ParseIP(u.Hostname()) == nil || !net.ParseIP(u.Hostname()).IsLoopback() {
		return errors.New("interface address must be loopback")
	}
	path, err := FindBrowser()
	if err != nil {
		return err
	}
	command := exec.Command(path, "--new-window", address)
	if err := command.Start(); err != nil {
		return fmt.Errorf("open local interface: %w", err)
	}
	go func() { _ = command.Wait() }()
	return nil
}
