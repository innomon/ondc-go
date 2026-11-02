package node

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"sync"

	"ondc/shared/p2p"
	"ondc/shared/storage"

	"golang.org/x/crypto/acme/autocert"
)

// UnifiedServer represents the unified single-process node server orchestrating
// onboarding, buyer/seller endpoints, embedded SQLite P2P replication, and dual-mode TLS listeners.
type UnifiedServer struct {
	Config          *NodeConfig
	Mux             http.Handler
	HTTPServer      *http.Server
	ACMEServer      *http.Server
	AutocertManager *autocert.Manager
	BridgeDaemon    *p2p.BridgeDaemon

	ownDaemon bool
	mu        sync.Mutex
	stopCh    chan struct{}
	started   bool
}

// NewUnifiedServer builds and wires a UnifiedServer instance with dynamic config,
// services multiplexer, and embedded P2P replication engine.
// If daemon is nil and cfg.EnableP2P is true, an embedded BridgeDaemon is instantiated automatically.
func NewUnifiedServer(
	cfg *NodeConfig,
	services NodeServices,
	daemon *p2p.BridgeDaemon,
) (*UnifiedServer, error) {
	if cfg == nil {
		return nil, errors.New("node config is nil")
	}

	server := &UnifiedServer{
		Config: cfg,
		stopCh: make(chan struct{}),
	}

	// 1. Embedded Bridge Daemon
	if daemon != nil {
		server.BridgeDaemon = daemon
	} else if cfg.EnableP2P {
		bridgeCfg := &p2p.BridgeConfig{
			DBPath:       cfg.DBPath,
			CRMPath:      cfg.CRMPath,
			NodeID:       cfg.NodeID,
			SyncInterval: cfg.SyncInterval,
			MaxBatchSize: cfg.MaxBatchSize,
			HTTPAddr:     fmt.Sprintf(":%d", cfg.HTTPPort),
		}

		d, err := p2p.NewBridgeDaemon(bridgeCfg)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize embedded bridge daemon: %w", err)
		}
		server.BridgeDaemon = d
		server.ownDaemon = true
	}

	// Mount daemon status/health handlers onto services if daemon is active
	if server.BridgeDaemon != nil {
		daemonHandler := p2p.NewDaemonHTTPHandler(server.BridgeDaemon)
		if services.HealthHandler == nil {
			services.HealthHandler = daemonHandler
		}
		if services.P2PHandler == nil {
			services.P2PHandler = daemonHandler
		}
	}

	// 2. Build Unified HTTP Router
	mux := NewNodeMux(cfg, services)
	server.Mux = mux

	// 3. Configure HTTP / HTTPS Listeners based on Deployment Mode
	isVMWithRealDomain := cfg.Mode == ModeVM && !IsLocalhost(cfg.Domain)

	if isVMWithRealDomain {
		// Ensure certificate cache directory exists
		if err := os.MkdirAll(cfg.CertDir, 0700); err != nil {
			return nil, fmt.Errorf("failed to create cert-dir %q: %w", cfg.CertDir, err)
		}

		m := &autocert.Manager{
			Prompt:     autocert.AcceptTOS,
			HostPolicy: autocert.HostWhitelist(cfg.Domain),
			Cache:      autocert.DirCache(cfg.CertDir),
		}
		server.AutocertManager = m

		server.HTTPServer = &http.Server{
			Addr:    fmt.Sprintf(":%d", cfg.HTTPSPort),
			Handler: mux,
			TLSConfig: &tls.Config{
				GetCertificate: m.GetCertificate,
			},
		}

		server.ACMEServer = &http.Server{
			Addr:    fmt.Sprintf(":%d", cfg.HTTPPort),
			Handler: m.HTTPHandler(nil),
		}
	} else {
		// Tunnel Mode, Local Mode, or VM Localhost Fallback: Plain HTTP
		server.HTTPServer = &http.Server{
			Addr:    fmt.Sprintf(":%d", cfg.HTTPPort),
			Handler: mux,
		}
	}

	return server, nil
}

// Repository returns the change-tracked SQLite storage repository managed by the embedded daemon, if active.
func (s *UnifiedServer) Repository() storage.Repository {
	if s.BridgeDaemon != nil {
		return s.BridgeDaemon.Repository()
	}
	return nil
}

// Serve starts the server listening on the provided network listeners.
// Useful for hermetic tests or customized network bindings.
func (s *UnifiedServer) Serve(httpListener net.Listener, tlsListener net.Listener) error {
	s.mu.Lock()
	if s.started {
		s.mu.Unlock()
		return errors.New("server already started")
	}
	s.started = true
	s.mu.Unlock()

	// Start embedded bridge daemon in background if present
	if s.BridgeDaemon != nil {
		go func() {
			_ = s.BridgeDaemon.Start(context.Background())
		}()
	}

	// If ACME server is configured and an HTTP listener is provided for it
	if s.ACMEServer != nil && httpListener != nil {
		go func() {
			_ = s.ACMEServer.Serve(httpListener)
		}()
	}

	// Primary server
	if tlsListener != nil {
		return s.HTTPServer.Serve(tlsListener)
	}

	if httpListener != nil {
		if s.HTTPServer.TLSConfig != nil {
			tlsL := tls.NewListener(httpListener, s.HTTPServer.TLSConfig)
			return s.HTTPServer.Serve(tlsL)
		}
		return s.HTTPServer.Serve(httpListener)
	}

	return errors.New("no listeners provided to Serve")
}

// Start boots the server using configured TCP addresses.
func (s *UnifiedServer) Start(ctx context.Context) error {
	s.mu.Lock()
	if s.started {
		s.mu.Unlock()
		return errors.New("server already started")
	}
	s.started = true
	s.mu.Unlock()

	// Start embedded bridge daemon in background if present
	if s.BridgeDaemon != nil {
		go func() {
			_ = s.BridgeDaemon.Start(ctx)
		}()
	}

	// In VM mode with real domain: serve ACME redirect on port 80 and TLS on port 443
	if s.ACMEServer != nil && s.AutocertManager != nil {
		go func() {
			if err := s.ACMEServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				fmt.Fprintf(os.Stderr, "ACME HTTP listener error: %v\n", err)
			}
		}()

		return s.HTTPServer.ListenAndServeTLS("", "")
	}

	// Plain HTTP (Cloudflare Tunnel or Local)
	return s.HTTPServer.ListenAndServe()
}

// Shutdown gracefully stops network listeners and the embedded P2P bridge daemon.
func (s *UnifiedServer) Shutdown(ctx context.Context) error {
	var errs []error

	// 1. Shutdown HTTP and ACME servers
	if s.HTTPServer != nil {
		if err := s.HTTPServer.Shutdown(ctx); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errs = append(errs, fmt.Errorf("httpserver shutdown: %w", err))
		}
	}
	if s.ACMEServer != nil {
		if err := s.ACMEServer.Shutdown(ctx); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errs = append(errs, fmt.Errorf("acmeserver shutdown: %w", err))
		}
	}

	// 2. Stop embedded bridge daemon
	if s.BridgeDaemon != nil {
		s.BridgeDaemon.Stop()
		if s.ownDaemon {
			if err := s.BridgeDaemon.Close(); err != nil {
				errs = append(errs, fmt.Errorf("bridge daemon close: %w", err))
			}
		}
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}
