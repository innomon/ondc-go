package p2p_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"ondc/shared/p2p"
)

func TestDaemonHTTPHandler_HealthzAndStatus(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &p2p.BridgeConfig{
		DBPath:       filepath.Join(tmpDir, "ondc.db"),
		CRMPath:      filepath.Join(tmpDir, "crm.db"),
		NodeID:       "http-test-node",
		SyncInterval: time.Second,
		MaxBatchSize: 100,
		HTTPAddr:     ":8085",
	}

	daemon, err := p2p.NewBridgeDaemon(cfg)
	if err != nil {
		t.Fatalf("failed to create daemon: %v", err)
	}
	defer daemon.Close()

	handler := p2p.NewDaemonHTTPHandler(daemon)

	// 1. Test /healthz
	reqHealth := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rrHealth := httptest.NewRecorder()
	handler.ServeHTTP(rrHealth, reqHealth)

	if rrHealth.Code != http.StatusOK {
		t.Errorf("expected status 200 for /healthz, got %d", rrHealth.Code)
	}

	var healthResp map[string]string
	if err := json.NewDecoder(rrHealth.Body).Decode(&healthResp); err != nil {
		t.Fatalf("failed to decode health response: %v", err)
	}
	if healthResp["status"] != "ok" {
		t.Errorf("expected status 'ok', got '%s'", healthResp["status"])
	}

	// 2. Test /status
	reqStatus := httptest.NewRequest(http.MethodGet, "/status", nil)
	rrStatus := httptest.NewRecorder()
	handler.ServeHTTP(rrStatus, reqStatus)

	if rrStatus.Code != http.StatusOK {
		t.Errorf("expected status 200 for /status, got %d", rrStatus.Code)
	}

	var statsResp p2p.DaemonStats
	if err := json.NewDecoder(rrStatus.Body).Decode(&statsResp); err != nil {
		t.Fatalf("failed to decode stats response: %v", err)
	}
	if statsResp.NodeID != "http-test-node" {
		t.Errorf("expected node ID 'http-test-node', got '%s'", statsResp.NodeID)
	}

	// 3. Test 404
	reqNotFound := httptest.NewRequest(http.MethodGet, "/unknown", nil)
	rrNotFound := httptest.NewRecorder()
	handler.ServeHTTP(rrNotFound, reqNotFound)
	if rrNotFound.Code != http.StatusNotFound {
		t.Errorf("expected 404 for unknown route, got %d", rrNotFound.Code)
	}

	// 4. Test Method Not Allowed on /healthz
	reqBadMethod := httptest.NewRequest(http.MethodPost, "/healthz", nil)
	rrBadMethod := httptest.NewRecorder()
	handler.ServeHTTP(rrBadMethod, reqBadMethod)
	if rrBadMethod.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405 for POST /healthz, got %d", rrBadMethod.Code)
	}

	// 5. Test Method Not Allowed on /status
	reqBadStatusMethod := httptest.NewRequest(http.MethodPost, "/status", nil)
	rrBadStatusMethod := httptest.NewRecorder()
	handler.ServeHTTP(rrBadStatusMethod, reqBadStatusMethod)
	if rrBadStatusMethod.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405 for POST /status, got %d", rrBadStatusMethod.Code)
	}

	// 6. Test Config accessor on daemon
	if daemon.Config().NodeID != "http-test-node" {
		t.Errorf("daemon.Config().NodeID mismatch")
	}
}
