package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"time"

	"github.com/jackkayser2005/ariadne/internal/browser"
	"github.com/jackkayser2005/ariadne/internal/ui"
)

const guidedUsage = `usage:
  ariadne investigate [--no-open] [--addr 127.0.0.1:0] [--output directory] [url]
  ariadne investigate --duration 30s [--output new-directory] [--location deny|approximate] [--block-origin https://destination.example] url
  ariadne inspect [--no-open] [--json] <bundle-directory-or-export.json>

Investigate opens a local interface and discovers Chrome or Edge automatically.
Browse in its fresh recording window, then stop, review, and export the evidence.
Use --no-open to print the interface URL without opening it automatically.
Use --duration for a timed recording controlled from the terminal (up to 20m).
Saved inspections verify the evidence before opening a read-only view.
`

type guidedServer func(ui.InvestigationOptions, bool, io.Writer) error

func runGuided(command string, args []string, stdout, stderr io.Writer, serve guidedServer) int {
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	noOpen := flags.Bool("no-open", false, "")
	address := flags.String("addr", "127.0.0.1:0", "")
	output := flags.String("output", "", "")
	jsonOutput := flags.Bool("json", false, "")
	duration := flags.Duration("duration", 0, "")
	location := flags.String("location", "unchanged", "")
	var blocked []string
	flags.Func("block-origin", "", func(value string) error { blocked = append(blocked, value); return nil })
	if err := flags.Parse(args); err != nil {
		_, _ = io.WriteString(stderr, guidedUsage)
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if flags.NArg() > 1 || (command == "inspect" && flags.NArg() != 1) || *duration < 0 || *duration > 20*time.Minute || (*jsonOutput && command != "inspect") {
		_, _ = io.WriteString(stderr, guidedUsage)
		return 2
	}
	host, port, err := net.SplitHostPort(*address)
	portNumber, portErr := strconv.Atoi(port)
	if err != nil || portErr != nil || portNumber < 0 || portNumber > 65535 || net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() {
		fmt.Fprintln(stderr, "ariadne: interface address must use a loopback IP")
		return 2
	}
	options := ui.InvestigationOptions{Host: *address}
	if command == "inspect" {
		if *duration != 0 || *output != "" || len(blocked) > 0 || *location != "unchanged" {
			_, _ = io.WriteString(stderr, guidedUsage)
			return 2
		}
		options.SavedPath = flags.Arg(0)
		bundle, _, err := browser.ReadInvestigation(options.SavedPath)
		if err != nil {
			fmt.Fprintln(stderr, "ariadne: inspect:", err)
			return 1
		}
		if *jsonOutput {
			if json.NewEncoder(stdout).Encode(bundle) != nil {
				return 1
			}
			return 0
		}
	} else {
		options.InitialURL = flags.Arg(0)
		if options.InitialURL != "" {
			if _, err := browser.InvestigationURL(options.InitialURL); err != nil {
				fmt.Fprintln(stderr, "ariadne:", err)
				return 2
			}
		}
		if *duration > 0 {
			if options.InitialURL == "" {
				_, _ = io.WriteString(stderr, guidedUsage)
				return 2
			}
			path := *output
			if path == "" {
				path = filepath.Join(".ariadne", "investigations", "investigation-"+time.Now().UTC().Format("20060102-150405"))
			}
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
			defer stop()
			if err := recordTimed(ctx, browser.CaptureOptions{URL: options.InitialURL, Location: *location, BlockOrigins: blocked}, *duration, path, stdout); err != nil {
				fmt.Fprintln(stderr, "ariadne: investigate:", err)
				return 1
			}
			return 0
		}
		if len(blocked) > 0 || *location != "unchanged" {
			fmt.Fprintln(stderr, "ariadne: use the interface controls, or supply --duration with terminal controls")
			return 2
		}
		options.OutputRoot = *output
		if options.OutputRoot == "" {
			options.OutputRoot = filepath.Join(".ariadne", "investigations")
		}
	}
	if err := serve(options, !*noOpen, stdout); err != nil {
		fmt.Fprintln(stderr, "ariadne:", command+":", err)
		return 1
	}
	return 0
}

func recordTimed(ctx context.Context, options browser.CaptureOptions, duration time.Duration, path string, output io.Writer) error {
	options.Markers = browser.GenerateMarkers()
	capture, err := browser.StartCapture(ctx, options)
	if err != nil {
		return err
	}
	defer capture.Stop(true)
	if _, err := fmt.Fprintln(output, "Recording in a fresh browser. Use these synthetic values to trace an input:"); err != nil {
		return err
	}
	for _, marker := range options.Markers {
		if _, err := fmt.Fprintf(output, "%s: %s\n", marker.Category, marker.Value); err != nil {
			return err
		}
	}
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-timer.C:
	case <-ctx.Done():
		return errors.New("recording cancelled; no investigation saved")
	}
	result, err := capture.Stop(false)
	if err != nil {
		return err
	}
	bundle, err := browser.SaveInvestigation(path, result)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(output, "Saved investigation: %s\nObserved steps: %d\nVisibility: %s\nOpen with: ariadne inspect %q\n", path, len(bundle.Journey.Observations), bundle.Journey.Completeness, path)
	return err
}

func serveGuided(options ui.InvestigationOptions, open bool, output io.Writer) error {
	parent := options.Context
	if parent == nil {
		parent = context.Background()
	}
	ctx, stop := signal.NotifyContext(parent, os.Interrupt)
	defer stop()
	listener, err := net.Listen("tcp", options.Host)
	if err != nil {
		return errors.New("local interface address is unavailable")
	}
	defer listener.Close()
	options.Host = listener.Addr().String()
	options.Context = ctx
	handler, err := ui.NewInvestigationHandler(options)
	if err != nil {
		return err
	}
	defer handler.Close()
	if _, err := fmt.Fprintln(output, "Ariadne local interface:", handler.URL()); err != nil {
		return err
	}
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 20 * time.Second, WriteTimeout: 45 * time.Second, IdleTimeout: time.Minute, MaxHeaderBytes: 16 << 10, ErrorLog: log.New(io.Discard, "", 0)}
	if open {
		if err := browser.OpenInterface(handler.URL()); err != nil {
			fmt.Fprintln(output, "Automatic opening unavailable. Open the session URL above in a browser.")
		}
	}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdown)
	}()
	if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return errors.New("local interface stopped unexpectedly")
	}
	return nil
}
