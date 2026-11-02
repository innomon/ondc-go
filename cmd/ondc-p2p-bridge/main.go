package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"ondc/shared/p2p"
)

func main() {
	log.Println("[INFO] Starting ondc-p2p-bridge daemon...")

	// 1. Resolve configuration (CLI flags > ENV > Defaults)
	cfg, err := p2p.LoadBridgeConfig(os.Args[1:], os.LookupEnv)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			os.Exit(0)
		}
		log.Fatalf("[FATAL] Configuration error: %v", err)
	}

	log.Printf("[INFO] Config loaded: NodeID=%s, DBPath=%s, CRMPath=%s, Interval=%v, HTTPAddr=%s",
		cfg.NodeID, cfg.DBPath, cfg.CRMPath, cfg.SyncInterval, cfg.HTTPAddr)

	// 2. Initialize Bridge Daemon
	daemon, err := p2p.NewBridgeDaemon(cfg)
	if err != nil {
		log.Fatalf("[FATAL] Failed to initialize bridge daemon: %v", err)
	}
	defer daemon.Close()

	// 3. Setup context with OS signal handling for graceful shutdown
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// 4. Start HTTP Server for /healthz and /status
	httpHandler := p2p.NewDaemonHTTPHandler(daemon)
	httpServer := &http.Server{
		Addr:    cfg.HTTPAddr,
		Handler: httpHandler,
	}

	go func() {
		log.Printf("[INFO] HTTP health/status server listening on %s", cfg.HTTPAddr)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("[ERROR] HTTP server error: %v", err)
		}
	}()

	// 5. Start replication engine in a goroutine
	daemonErrCh := make(chan error, 1)
	go func() {
		daemonErrCh <- daemon.Start(ctx)
	}()

	// 6. Wait for shutdown signal or daemon error
	select {
	case <-ctx.Done():
		log.Println("[INFO] Shutdown signal received, terminating gracefully...")
	case err := <-daemonErrCh:
		if err != nil && !errors.Is(err, context.Canceled) {
			log.Printf("[ERROR] Bridge daemon terminated with error: %v", err)
		}
	}

	// 7. Gracefully shut down HTTP server
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		log.Printf("[WARN] HTTP server shutdown error: %v", err)
	}

	// 8. Stop daemon
	daemon.Stop()
	log.Println("[INFO] ondc-p2p-bridge shutdown complete.")
}
