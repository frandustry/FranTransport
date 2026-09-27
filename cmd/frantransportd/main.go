package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/frandustry/FranTransport/internal/node"
	fttailcat "github.com/frandustry/FranTransport/pkg/transport/tailcat"
)

func main() {
	configDir, err := os.UserConfigDir()
	if err != nil {
		configDir = "."
	}
	var name, identityPath, controlURL, localListen, derpMapURL string
	hostname, _ := os.Hostname()
	flag.StringVar(&name, "name", hostname, "node display name")
	flag.StringVar(&identityPath, "identity", filepath.Join(configDir, "frantransport", "identity.json"), "local identity path")
	flag.StringVar(&controlURL, "control-plane", "http://127.0.0.1:8080", "control plane URL")
	flag.StringVar(&localListen, "local-listen", "127.0.0.1:7777", "local status/API listen address")
	flag.StringVar(&derpMapURL, "derp-map-url", "", "optional Tailcat DERP map URL")
	flag.Parse()
	logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	startCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	tr, err := fttailcat.New(startCtx, fttailcat.Config{DERPMapURL: derpMapURL})
	if err != nil {
		logger.Error("start Tailcat transport", "error", err)
		os.Exit(1)
	}
	cp := &node.ControlPlaneClient{BaseURL: controlURL}
	daemon, err := node.NewDaemon(node.DaemonConfig{Name: name, IdentityPath: identityPath, EnrollmentToken: os.Getenv("FRANTRANSPORT_ENROLLMENT_TOKEN"), ControlPlane: cp, Transport: tr, LocalListen: localListen, Logger: logger})
	if err != nil {
		_ = tr.Close()
		logger.Error("configure daemon", "error", err)
		os.Exit(1)
	}
	if err := daemon.Run(ctx); err != nil {
		logger.Error("daemon stopped", "error", err)
		os.Exit(1)
	}
}
