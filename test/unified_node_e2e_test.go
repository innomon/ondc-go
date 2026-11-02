package test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ondc/shared/node"
	"ondc/shared/p2p"
	"ondc/shared/storage"

	_ "modernc.org/sqlite"
)

func TestUnifiedNodeE2E_FullLifecycleAndReplication(t *testing.T) {
	tmpDir := t.TempDir()
	ondcDBPath := filepath.Join(tmpDir, "ondc.db")
	crmDBPath := filepath.Join(tmpDir, "sqlite_p2p_crm.db")

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to bind test listener: %v", err)
	}
	serverAddr := listener.Addr().String()

	cfg := &node.NodeConfig{
		Mode:         node.ModeTunnel,
		HTTPPort:     listener.Addr().(*net.TCPAddr).Port,
		Domain:       "localhost",
		CertDir:      filepath.Join(tmpDir, "certs"),
		DBPath:       ondcDBPath,
		CRMPath:      crmDBPath,
		NodeID:       "unified-e2e-node",
		SyncInterval: 50 * time.Millisecond,
		MaxBatchSize: 100,
		EnableBuyer:  true,
		EnableSeller: true,
		EnableP2P:    true,
		DrainTimeout: 3 * time.Second,
	}

	services := node.NodeServices{
		OnboardingSiteVerificationToken: "ondc-e2e-token-abc",
	}

	server, err := node.NewUnifiedServer(cfg, services, nil)
	if err != nil {
		t.Fatalf("failed to create UnifiedServer: %v", err)
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- server.Serve(listener, nil)
	}()

	client := &http.Client{Timeout: 3 * time.Second}

	// 1. Root info check
	resp, err := client.Get(fmt.Sprintf("http://%s/", serverAddr))
	if err != nil {
		t.Fatalf("failed GET /: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 OK from /, got %d", resp.StatusCode)
	}
	var rootData map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&rootData); err != nil {
		t.Fatalf("failed to decode root json: %v", err)
	}
	if rootData["service"] != "ondc-node" || rootData["mode"] != "tunnel" {
		t.Errorf("unexpected root payload: %+v", rootData)
	}

	// 2. Onboarding verification HTML
	respHTML, err := client.Get(fmt.Sprintf("http://%s/ondc-site-verification.html", serverAddr))
	if err != nil {
		t.Fatalf("failed GET /ondc-site-verification.html: %v", err)
	}
	defer respHTML.Body.Close()
	bodyHTML, _ := io.ReadAll(respHTML.Body)
	if !strings.Contains(string(bodyHTML), "ondc-e2e-token-abc") {
		t.Errorf("expected token in site verification, got %s", string(bodyHTML))
	}

	// 3. Onboarding /on_subscribe
	respSub, err := client.Post(fmt.Sprintf("http://%s/on_subscribe", serverAddr), "application/json", bytes.NewBufferString(`{}`))
	if err != nil {
		t.Fatalf("failed POST /on_subscribe: %v", err)
	}
	defer respSub.Body.Close()
	if respSub.StatusCode != http.StatusOK {
		t.Errorf("expected 200 from /on_subscribe, got %d", respSub.StatusCode)
	}

	// 4. Buyer BAP and Client endpoints
	respBAP, err := client.Post(fmt.Sprintf("http://%s/bap/on_search", serverAddr), "application/json", bytes.NewBufferString(`{}`))
	if err != nil {
		t.Fatalf("failed POST /bap/on_search: %v", err)
	}
	respBAP.Body.Close()
	if respBAP.StatusCode != http.StatusOK {
		t.Errorf("expected 200 from /bap/on_search, got %d", respBAP.StatusCode)
	}

	respClient, err := client.Post(fmt.Sprintf("http://%s/client/search", serverAddr), "application/json", bytes.NewBufferString(`{}`))
	if err != nil {
		t.Fatalf("failed POST /client/search: %v", err)
	}
	respClient.Body.Close()
	if respClient.StatusCode != http.StatusOK {
		t.Errorf("expected 200 from /client/search, got %d", respClient.StatusCode)
	}

	// 5. Seller BPP endpoint
	respBPP, err := client.Post(fmt.Sprintf("http://%s/bpp/search", serverAddr), "application/json", bytes.NewBufferString(`{}`))
	if err != nil {
		t.Fatalf("failed POST /bpp/search: %v", err)
	}
	respBPP.Body.Close()
	if respBPP.StatusCode != http.StatusOK {
		t.Errorf("expected 200 from /bpp/search, got %d", respBPP.StatusCode)
	}

	// 6. Observability: /healthz and /status
	respHealth, err := client.Get(fmt.Sprintf("http://%s/healthz", serverAddr))
	if err != nil {
		t.Fatalf("failed GET /healthz: %v", err)
	}
	respHealth.Body.Close()
	if respHealth.StatusCode != http.StatusOK {
		t.Errorf("expected 200 from /healthz, got %d", respHealth.StatusCode)
	}

	respStatus, err := client.Get(fmt.Sprintf("http://%s/status", serverAddr))
	if err != nil {
		t.Fatalf("failed GET /status: %v", err)
	}
	defer respStatus.Body.Close()
	bodyStatus, _ := io.ReadAll(respStatus.Body)
	if !strings.Contains(string(bodyStatus), "unified-e2e-node") {
		t.Errorf("expected node ID in /status output, got %s", string(bodyStatus))
	}

	// 7. Verify In-Process Outbound P2P Replication: ONDC -> CRM
	repo := server.Repository()
	if repo == nil {
		t.Fatal("server repository is nil")
	}

	txnOut := storage.TransactionRecord{
		TransactionID:   "txn-e2e-out-001",
		TransactionType: "select",
		TransactionAPI:  "select",
		MessageID:       "msg-e2e-out",
		RequestID:       "req-e2e-out",
		Payload:         `{"item_id":"item-123"}`,
		MessageStatus:   "INITIATED",
		CreatedAt:       time.Now(),
	}

	if err := repo.SaveTransaction(context.Background(), txnOut); err != nil {
		t.Fatalf("failed to save transaction: %v", err)
	}

	// Wait for background sync loop
	time.Sleep(150 * time.Millisecond)

	crmDB, err := sql.Open("sqlite", crmDBPath)
	if err != nil {
		t.Fatalf("failed to open crm db: %v", err)
	}
	defer crmDB.Close()

	var crmCount int
	expectedKey := "ondc:transactions:txn-e2e-out-001:select:msg-e2e-out:req-e2e-out"
	err = crmDB.QueryRow("SELECT COUNT(*) FROM crm_store WHERE key = ?", expectedKey).Scan(&crmCount)
	if err != nil {
		t.Fatalf("failed to query crm_store: %v", err)
	}
	if crmCount != 1 {
		t.Errorf("expected 1 record in crm_store for key %s, got %d", expectedKey, crmCount)
	}

	// 8. Verify In-Process Inbound P2P Replication: CRM -> ONDC
	inboundKey := p2p.FormatSQLiteP2PKey("transactions", "txn-e2e-in-002:REQUEST-ACTION:msg-e2e-in:req-e2e-in")
	inboundTxn := storage.TransactionRecord{
		TransactionID:   "txn-e2e-in-002",
		TransactionType: "REQUEST-ACTION",
		TransactionAPI:  "init",
		MessageID:       "msg-e2e-in",
		RequestID:       "req-e2e-in",
		Payload:         `{"item_id":"item-456"}`,
		MessageStatus:   "ACKNOWLEDGED",
		CreatedAt:       time.Now(),
	}
	inboundData, _ := json.Marshal(inboundTxn)
	inboundMeta, _ := json.Marshal(map[string]any{
		"logical_clock": 105,
		"node_id":       "peer-external-001",
	})
	inboundCS := p2p.SQLiteP2PChangeset{
		Sequence:  105,
		Timestamp: time.Now().UnixNano(),
		Operation: p2p.SQLiteP2POpInsert,
		Key:       inboundKey,
		Metadata:  inboundMeta,
		Data:      inboundData,
	}

	if err := p2p.ApplyChangesetToCRMStore(context.Background(), crmDB, &inboundCS); err != nil {
		t.Fatalf("failed to apply inbound changeset into crm_store: %v", err)
	}

	// Wait for background sync loop to pull and converge
	var convergedStatus string
	for i := 0; i < 40; i++ {
		time.Sleep(25 * time.Millisecond)
		fetched, err := repo.GetTransaction(context.Background(), "txn-e2e-in-002", "msg-e2e-in")
		if err == nil && fetched != nil {
			convergedStatus = fetched.MessageStatus
			break
		}
	}
	if convergedStatus != "ACKNOWLEDGED" {
		t.Errorf("expected converged message_status 'ACKNOWLEDGED', got %q", convergedStatus)
	}

	// 9. Clean Shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		t.Fatalf("failed to gracefully shutdown server: %v", err)
	}

	select {
	case err := <-errCh:
		if err != nil && err != http.ErrServerClosed {
			t.Fatalf("Serve returned error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for Serve to terminate")
	}
}
