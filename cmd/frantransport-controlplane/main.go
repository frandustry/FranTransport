package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/frandustry/FranTransport/internal/controlplane"
	"github.com/frandustry/FranTransport/internal/store"
)

func main() {
	var dbPath, listen string
	var issue time.Duration
	flag.StringVar(&dbPath, "db", "frantransport.db", "SQLite database path")
	flag.StringVar(&listen, "listen", "127.0.0.1:8080", "HTTP listen address")
	flag.DurationVar(&issue, "issue-token", 0, "issue one enrollment token with this TTL, then exit")
	flag.Parse()
	logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	db, err := store.OpenSQLite(dbPath)
	if err != nil {
		logger.Error("open database", "error", err)
		os.Exit(1)
	}
	defer db.Close()
	if issue > 0 {
		token, err := db.IssueEnrollmentToken(context.Background(), issue)
		if err != nil {
			logger.Error("issue enrollment token", "error", err)
			os.Exit(1)
		}
		fmt.Println(token)
		return
	}
	h := (&controlplane.Server{Store: db}).Handler()
	srv := &http.Server{Addr: listen, Handler: h, ReadHeaderTimeout: 5 * time.Second}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()
	logger.Info("control plane listening", "address", listen)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Error("serve", "error", err)
		os.Exit(1)
	}
}
