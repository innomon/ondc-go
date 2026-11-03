package main

import (
	"context"
	"errors"
	"flag"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"ondc/shared/logging"
	"ondc/shared/node"
)

func main() {
	// 1. Resolve configuration (CLI flags > ENV > Defaults)
	cfg, err := node.LoadNodeConfig(os.Args[1:], os.LookupEnv)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			os.Exit(0)
		}
		logging.Exitf("Configuration error: %v", err)
	}

	// 2. Initialize Hybrid Structured Logging
	_, err = logging.Init(logging.Config{
		Level:          cfg.LogLevel,
		ConsoleEnabled: cfg.LogConsole,
		FileEnabled:    cfg.LogFile,
		Dir:            cfg.LogDir,
		FileName:       "ondc-node.jsonl",
		MaxSizeMB:      cfg.LogMaxSizeMB,
		MaxBackups:     cfg.LogMaxBackups,
	})
	if err != nil {
		logging.Exitf("Failed to initialize logging: %v", err)
	}

	logging.Infof("Starting Unified ONDC Node (ondc-node)...")
	logging.Infof("Node configuration: Mode=%s, Domain=%s, HTTPPort=%d, HTTPSPort=%d, DBPath=%s, CRMPath=%s, P2P=%v, Buyer=%v, Seller=%v",
		cfg.Mode, cfg.Domain, cfg.HTTPPort, cfg.HTTPSPort, cfg.DBPath, cfg.CRMPath, cfg.EnableP2P, cfg.EnableBuyer, cfg.EnableSeller)

	// 3. Setup context with OS signal handling for graceful shutdown
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// 4. Initialize Unified Server
	services := node.NodeServices{
		OnboardingSiteVerificationToken: os.Getenv("ONDC_SITE_VERIFICATION_TOKEN"),
	}

	server, err := node.NewUnifiedServer(cfg, services, nil)
	if err != nil {
		logging.Exitf("Failed to initialize unified server: %v", err)
	}

	// 5. Start Server
	serverErrCh := make(chan error, 1)
	go func() {
		if cfg.Mode == node.ModeVM && !node.IsLocalhost(cfg.Domain) {
			logging.Infof("Running in VM Direct Internet mode with Let's Encrypt AutoCert for domain %s (HTTPS :%d, HTTP :%d)",
				cfg.Domain, cfg.HTTPSPort, cfg.HTTPPort)
		} else {
			logging.Infof("Running in %s mode on HTTP :%d", cfg.Mode, cfg.HTTPPort)
		}

		if err := server.Start(ctx); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErrCh <- err
		}
	}()

	// 6. Wait for OS signal or unexpected server failure
	select {
	case <-ctx.Done():
		logging.Infof("Shutdown signal received. Draining connections...")
	case err := <-serverErrCh:
		logging.Exitf("Server encountered fatal error: %v", err)
	}

	// 7. Graceful shutdown
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.DrainTimeout)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		logging.Warningf("Error during graceful shutdown: %v", err)
	} else {
		logging.Infof("Unified ONDC Node stopped cleanly.")
	}
}
