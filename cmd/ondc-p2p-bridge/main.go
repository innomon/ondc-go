package main

import (
	"context"
	"errors"
	"flag"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"ondc/shared/logging"
	"ondc/shared/p2p"
)

func main() {
	// 1. Resolve configuration (CLI flags > ENV > Defaults)
	cfg, err := p2p.LoadBridgeConfig(os.Args[1:], os.LookupEnv)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			os.Exit(0)
		}
		logging.Exitf("Configuration error: %v", err)
	}

	// 2. Initialize Hybrid Structured Logging
	_, err = logging.Init(logging.Config{
		Level:          "INFO",
		ConsoleEnabled: true,
		FileEnabled:    true,
		Dir:            "./data/logs",
		FileName:       "ondc-p2p-bridge.jsonl",
		MaxSizeMB:      50,
		MaxBackups:     3,
	})
	if err != nil {
		logging.Exitf("Failed to initialize logging: %v", err)
	}

	logging.Infof("Starting ondc-p2p-bridge daemon...")
	logging.Infof("Config loaded: NodeID=%s, DBPath=%s, CRMPath=%s, Interval=%v, HTTPAddr=%s",
		cfg.NodeID, cfg.DBPath, cfg.CRMPath, cfg.SyncInterval, cfg.HTTPAddr)

	// 3. Initialize Bridge Daemon
	daemon, err := p2p.NewBridgeDaemon(cfg)
	if err != nil {
		logging.Exitf("Failed to initialize bridge daemon: %v", err)
	}
	defer daemon.Close()

	// 4. Setup context with OS signal handling for graceful shutdown
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// 5. Start HTTP Server for /healthz and /status
	httpHandler := p2p.NewDaemonHTTPHandler(daemon)
	httpServer := &http.Server{
		Addr:    cfg.HTTPAddr,
		Handler: httpHandler,
	}

	go func() {
		logging.Infof("HTTP health/status server listening on %s", cfg.HTTPAddr)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logging.Errorf("HTTP server error: %v", err)
		}
	}()

	// 6. Start replication engine in a goroutine
	daemonErrCh := make(chan error, 1)
	go func() {
		daemonErrCh <- daemon.Start(ctx)
	}()

	// 7. Wait for shutdown signal or daemon error
	select {
	case <-ctx.Done():
		logging.Infof("Shutdown signal received, terminating gracefully...")
	case err := <-daemonErrCh:
		if err != nil && !errors.Is(err, context.Canceled) {
			logging.Errorf("Bridge daemon terminated with error: %v", err)
		}
	}

	// 8. Gracefully shut down HTTP server
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		logging.Warningf("HTTP server shutdown error: %v", err)
	}

	// 9. Stop daemon
	daemon.Stop()
	logging.Infof("ondc-p2p-bridge shutdown complete.")
}
