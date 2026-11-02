package node_test

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"ondc/shared/node"
	"ondc/shared/storage"

	_ "modernc.org/sqlite"
)

func TestUnifiedServer_TunnelMode_Lifecycle(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := &node.NodeConfig{
		Mode:         node.ModeTunnel,
		HTTPPort:     0, // dynamic port for test
		Domain:       "localhost",
		DBPath:       filepath.Join(tmpDir, "ondc.db"),
		CRMPath:      filepath.Join(tmpDir, "crm.db"),
		EnableBuyer:  true,
		EnableSeller: true,
		EnableP2P:    false, // P2P tested separately
		DrainTimeout: 2 * time.Second,
	}

	services := node.NodeServices{
		OnboardingSiteVerificationToken: "test-token-12345",
	}

	server, err := node.NewUnifiedServer(cfg, services, nil)
	if err != nil {
		t.Fatalf("failed to create UnifiedServer: %v", err)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}

	addr := listener.Addr().String()

	errCh := make(chan error, 1)
	go func() {
		errCh <- server.Serve(listener, nil)
	}()

	// Verify HTTP responses
	resp, err := http.Get(fmt.Sprintf("http://%s/healthz", addr))
	if err != nil {
		t.Fatalf("failed to GET /healthz: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 OK from /healthz, got %d", resp.StatusCode)
	}

	respHTML, err := http.Get(fmt.Sprintf("http://%s/ondc-site-verification.html", addr))
	if err != nil {
		t.Fatalf("failed to GET site verification: %v", err)
	}
	defer respHTML.Body.Close()
	body, _ := io.ReadAll(respHTML.Body)
	if !contains(string(body), "test-token-12345") {
		t.Errorf("expected verification token in html body, got %s", string(body))
	}

	// Graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		t.Fatalf("failed to shutdown: %v", err)
	}

	select {
	case err := <-errCh:
		if err != nil && err != http.ErrServerClosed {
			t.Fatalf("Serve returned unexpected error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for server shutdown")
	}
}

func TestUnifiedServer_VMMode_LocalhostFallback(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := &node.NodeConfig{
		Mode:         node.ModeVM,
		HTTPPort:     0,
		HTTPSPort:    0,
		Domain:       "localhost", // Localhost should fallback to plain HTTP
		CertDir:      filepath.Join(tmpDir, "certs"),
		DBPath:       filepath.Join(tmpDir, "ondc.db"),
		CRMPath:      filepath.Join(tmpDir, "crm.db"),
		EnableBuyer:  true,
		EnableSeller: true,
		EnableP2P:    false,
		DrainTimeout: 2 * time.Second,
	}

	server, err := node.NewUnifiedServer(cfg, node.NodeServices{}, nil)
	if err != nil {
		t.Fatalf("failed to create UnifiedServer: %v", err)
	}

	if server.AutocertManager != nil {
		t.Errorf("expected AutocertManager to be nil for localhost domain fallback")
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	addr := listener.Addr().String()

	errCh := make(chan error, 1)
	go func() {
		errCh <- server.Serve(listener, nil)
	}()

	resp, err := http.Get(fmt.Sprintf("http://%s/healthz", addr))
	if err != nil {
		t.Fatalf("failed to GET /healthz: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 OK, got %d", resp.StatusCode)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		t.Fatalf("failed to shutdown: %v", err)
	}

	select {
	case err := <-errCh:
		if err != nil && err != http.ErrServerClosed {
			t.Fatalf("Serve returned unexpected error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for server shutdown")
	}
}

func TestUnifiedServer_VMMode_AutocertConfig(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := &node.NodeConfig{
		Mode:         node.ModeVM,
		HTTPPort:     80,
		HTTPSPort:    443,
		Domain:       "sovereign.ondc-partner.org",
		CertDir:      filepath.Join(tmpDir, "certs"),
		DBPath:       filepath.Join(tmpDir, "ondc.db"),
		CRMPath:      filepath.Join(tmpDir, "crm.db"),
		EnableBuyer:  true,
		EnableSeller: true,
		EnableP2P:    false,
		DrainTimeout: 2 * time.Second,
	}

	server, err := node.NewUnifiedServer(cfg, node.NodeServices{}, nil)
	if err != nil {
		t.Fatalf("failed to create UnifiedServer: %v", err)
	}

	if server.AutocertManager == nil {
		t.Fatal("expected AutocertManager to be configured for public domain in VM mode")
	}

	// Verify HostPolicy Whitelist accepts target domain
	ctx := context.Background()
	if err := server.AutocertManager.HostPolicy(ctx, "sovereign.ondc-partner.org"); err != nil {
		t.Errorf("expected HostPolicy to accept domain, got error: %v", err)
	}
	if err := server.AutocertManager.HostPolicy(ctx, "unauthorized.attacker.com"); err == nil {
		t.Errorf("expected HostPolicy to reject unauthorized domain")
	}

	if server.HTTPServer == nil || server.HTTPServer.TLSConfig == nil {
		t.Fatal("expected HTTPServer with TLSConfig to be initialized")
	}
}

func TestUnifiedServer_EmbeddedBridgeDaemon(t *testing.T) {
	tmpDir := t.TempDir()
	ondcDBPath := filepath.Join(tmpDir, "ondc.db")
	crmDBPath := filepath.Join(tmpDir, "crm.db")

	cfg := &node.NodeConfig{
		Mode:         node.ModeTunnel,
		HTTPPort:     0,
		Domain:       "localhost",
		DBPath:       ondcDBPath,
		CRMPath:      crmDBPath,
		NodeID:       "node-embed-test",
		SyncInterval: 50 * time.Millisecond,
		MaxBatchSize: 100,
		EnableBuyer:  true,
		EnableSeller: true,
		EnableP2P:    true,
		DrainTimeout: 2 * time.Second,
	}

	server, err := node.NewUnifiedServer(cfg, node.NodeServices{}, nil)
	if err != nil {
		t.Fatalf("failed to create UnifiedServer: %v", err)
	}

	if server.BridgeDaemon == nil {
		t.Fatal("expected BridgeDaemon to be initialized when EnableP2P is true")
	}

	repo := server.Repository()
	if repo == nil {
		t.Fatal("expected server.Repository() to return non-nil repository")
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	addr := listener.Addr().String()

	errCh := make(chan error, 1)
	go func() {
		errCh <- server.Serve(listener, nil)
	}()

	// Write a transaction through repository
	testTxn := storage.TransactionRecord{
		TransactionID:   "txn-embed-001",
		TransactionType: "search",
		TransactionAPI:  "search",
		MessageID:       "msg-001",
		RequestID:       "req-001",
		Payload:         `{"search":"item"}`,
		MessageStatus:   "INITIATED",
		CreatedAt:       time.Now(),
	}

	if err := repo.SaveTransaction(context.Background(), testTxn); err != nil {
		t.Fatalf("failed to save transaction: %v", err)
	}

	// Allow sync loop to run
	time.Sleep(150 * time.Millisecond)

	// Verify status endpoint reports p2p status
	resp, err := http.Get(fmt.Sprintf("http://%s/status", addr))
	if err != nil {
		t.Fatalf("failed to GET /status: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if !contains(string(body), "outbound_replicated") && !contains(string(body), "node-embed-test") {
		t.Errorf("expected status to report bridge telemetry, got: %s", string(body))
	}

	// Verify record propagated to crm_store
	crmDB, err := sql.Open("sqlite", crmDBPath)
	if err != nil {
		t.Fatalf("failed to open crm db: %v", err)
	}
	defer crmDB.Close()

	var crmCount int
	err = crmDB.QueryRow("SELECT COUNT(*) FROM crm_store WHERE key LIKE 'ondc:transactions:%'").Scan(&crmCount)
	if err != nil {
		t.Fatalf("failed to query crm_store: %v", err)
	}
	if crmCount == 0 {
		t.Errorf("expected crm_store to contain replicated records, got 0")
	}

	// Clean shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		t.Fatalf("failed to shutdown server: %v", err)
	}

	select {
	case err := <-errCh:
		if err != nil && err != http.ErrServerClosed {
			t.Fatalf("Serve returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for shutdown")
	}
}

func TestUnifiedServer_EdgeCasesAndErrors(t *testing.T) {
	// 1. Nil config
	_, err := node.NewUnifiedServer(nil, node.NodeServices{}, nil)
	if err == nil {
		t.Fatal("expected error for nil config, got nil")
	}

	// 2. Repository without P2P
	cfgNoP2P := &node.NodeConfig{
		Mode:      node.ModeTunnel,
		HTTPPort:  0,
		Domain:    "localhost",
		EnableP2P: false,
	}
	srvNoP2P, err := node.NewUnifiedServer(cfgNoP2P, node.NodeServices{}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo := srvNoP2P.Repository(); repo != nil {
		t.Errorf("expected nil repository when P2P is disabled, got %v", repo)
	}

	// 3. Serve without listeners
	err = srvNoP2P.Serve(nil, nil)
	if err == nil {
		t.Fatal("expected error when no listeners provided, got nil")
	}

	// 4. Double serve
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer l.Close()

	srv2, err := node.NewUnifiedServer(cfgNoP2P, node.NodeServices{}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	go func() {
		_ = srv2.Serve(l, nil)
	}()

	// Wait briefly for serve to register started
	time.Sleep(20 * time.Millisecond)

	err = srv2.Serve(l, nil)
	if err == nil || err.Error() != "server already started" {
		t.Errorf("expected 'server already started' error, got %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_ = srv2.Shutdown(ctx)
}

