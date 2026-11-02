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

	"ondc/shared/node"
)

func main() {
	log.Println("[INFO] Starting Unified ONDC Node (ondc-node)...")

	// 1. Resolve configuration (CLI flags > ENV > Defaults)
	cfg, err := node.LoadNodeConfig(os.Args[1:], os.LookupEnv)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			os.Exit(0)
		}
		log.Fatalf("[FATAL] Configuration error: %v", err)
	}

	log.Printf("[INFO] Node configuration: Mode=%s, Domain=%s, HTTPPort=%d, HTTPSPort=%d, DBPath=%s, CRMPath=%s, P2P=%v, Buyer=%v, Seller=%v",
		cfg.Mode, cfg.Domain, cfg.HTTPPort, cfg.HTTPSPort, cfg.DBPath, cfg.CRMPath, cfg.EnableP2P, cfg.EnableBuyer, cfg.EnableSeller)

	// 2. Setup context with OS signal handling for graceful shutdown
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// 3. Initialize Unified Server
	services := node.NodeServices{
		OnboardingSiteVerificationToken: os.Getenv("ONDC_SITE_VERIFICATION_TOKEN"),
	}

	server, err := node.NewUnifiedServer(cfg, services, nil)
	if err != nil {
		log.Fatalf("[FATAL] Failed to initialize unified server: %v", err)
	}

	// 4. Start Server
	serverErrCh := make(chan error, 1)
	go func() {
		if cfg.Mode == node.ModeVM && !node.IsLocalhost(cfg.Domain) {
			log.Printf("[INFO] Running in VM Direct Internet mode with Let's Encrypt AutoCert for domain %s (HTTPS :%d, HTTP :%d)",
				cfg.Domain, cfg.HTTPSPort, cfg.HTTPPort)
		} else {
			log.Printf("[INFO] Running in %s mode on HTTP :%d", cfg.Mode, cfg.HTTPPort)
		}

		if err := server.Start(ctx); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErrCh <- err
		}
	}()

	// 5. Wait for OS signal or unexpected server failure
	select {
	case <-ctx.Done():
		log.Println("[INFO] Shutdown signal received. Draining connections...")
	case err := <-serverErrCh:
		log.Fatalf("[FATAL] Server encountered fatal error: %v", err)
	}

	// 6. Graceful shutdown
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.DrainTimeout)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("[WARN] Error during graceful shutdown: %v", err)
	} else {
		log.Println("[INFO] Unified ONDC Node stopped cleanly.")
	}
}
