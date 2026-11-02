package test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"ondc/shared/p2p"
	"ondc/shared/storage"
	_ "modernc.org/sqlite"
)

func TestP2PBridgeDaemonE2E_FullBidirectionalConvergence(t *testing.T) {
	tmpDir := t.TempDir()
	ondcPath := filepath.Join(tmpDir, "ondc.db")
	crmPath := filepath.Join(tmpDir, "sqlite_p2p_crm.db")

	// Pick a free high port or arbitrary local port for HTTP test
	httpPort := 18095
	httpAddr := fmt.Sprintf("127.0.0.1:%d", httpPort)

	cfg := &p2p.BridgeConfig{
		DBPath:       ondcPath,
		CRMPath:      crmPath,
		NodeID:       "daemon-e2e-node",
		SyncInterval: 50 * time.Millisecond,
		MaxBatchSize: 100,
		HTTPAddr:     httpAddr,
	}

	daemon, err := p2p.NewBridgeDaemon(cfg)
	if err != nil {
		t.Fatalf("failed to create bridge daemon: %v", err)
	}
	defer daemon.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 1. Launch HTTP Server for /healthz and /status
	httpHandler := p2p.NewDaemonHTTPHandler(daemon)
	httpServer := &http.Server{
		Addr:    cfg.HTTPAddr,
		Handler: httpHandler,
	}
	go func() {
		_ = httpServer.ListenAndServe()
	}()
	defer func() {
		shCtx, shCancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer shCancel()
		_ = httpServer.Shutdown(shCtx)
	}()

	// 2. Launch BridgeDaemon in background
	daemonErrCh := make(chan error, 1)
	go func() {
		daemonErrCh <- daemon.Start(ctx)
	}()

	// Wait briefly for HTTP server to become responsive
	var healthOK bool
	for i := 0; i < 20; i++ {
		time.Sleep(25 * time.Millisecond)
		resp, err := http.Get(fmt.Sprintf("http://%s/healthz", httpAddr))
		if err == nil && resp.StatusCode == http.StatusOK {
			healthOK = true
			resp.Body.Close()
			break
		}
		if resp != nil {
			resp.Body.Close()
		}
	}
	if !healthOK {
		t.Fatalf("HTTP /healthz did not respond OK within timeout")
	}

	// 3. Test Outbound Auto-Replication: ONDC DB -> sqlite-p2p crm_store
	repo := daemon.Repository()
	now := time.Now().UTC()
	outboundTxn := storage.TransactionRecord{
		TransactionID:   "e2e-txn-outbound",
		TransactionType: "REQUEST-ACTION",
		TransactionAPI:  "search",
		MessageID:       "e2e-msg-1",
		RequestID:       "e2e-req-1",
		Payload:         `{"item":"sovereign-node"}`,
		ProviderID:      "provider-e2e",
		MessageStatus:   "ACK",
		ReqReceivedTime: now,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := repo.SaveTransaction(ctx, outboundTxn); err != nil {
		t.Fatalf("failed to save outbound transaction: %v", err)
	}

	// Verify automatic propagation to crm_store
	expectedCRMKey := p2p.FormatSQLiteP2PKey("transactions", "e2e-txn-outbound:REQUEST-ACTION:e2e-msg-1:e2e-req-1")
	var outboundSynced bool
	for i := 0; i < 40; i++ {
		time.Sleep(25 * time.Millisecond)
		rec, _ := p2p.ReadRecordFromCRMStore(ctx, daemon.CRMDB(), expectedCRMKey)
		if rec != nil {
			outboundSynced = true
			break
		}
	}
	if !outboundSynced {
		t.Fatalf("outbound transaction did not auto-replicate to crm_store within timeout")
	}

	// 4. Test Inbound Auto-Replication: sqlite-p2p crm_store -> ONDC DB
	inboundTxn := storage.TransactionRecord{
		TransactionID:   "e2e-txn-inbound",
		TransactionType: "REQUEST-ACTION",
		TransactionAPI:  "on_search",
		MessageID:       "e2e-msg-2",
		RequestID:       "e2e-req-2",
		Payload:         `{"item":"replicated-from-sqlite-p2p"}`,
		ProviderID:      "provider-remote",
		MessageStatus:   "ACK",
		ReqReceivedTime: now,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	inboundData, _ := json.Marshal(inboundTxn)
	inboundMeta, _ := json.Marshal(p2p.SQLiteP2PMetadata{
		Table:       "transactions",
		EntityID:    "e2e-txn-inbound:REQUEST-ACTION:e2e-msg-2:e2e-req-2",
		NodeID:      "remote-peer-auto",
		LamportTime: 125,
		ChangeID:    "chg-inbound-125",
	})
	inboundCS := p2p.SQLiteP2PChangeset{
		Sequence:  125,
		Timestamp: time.Now().UnixNano(),
		Operation: p2p.SQLiteP2POpInsert,
		Key:       p2p.FormatSQLiteP2PKey("transactions", "e2e-txn-inbound:REQUEST-ACTION:e2e-msg-2:e2e-req-2"),
		Metadata:  inboundMeta,
		Data:      inboundData,
	}

	// Apply directly into crm_store as if delivered by P2P feed
	if err := p2p.ApplyChangesetToCRMStore(ctx, daemon.CRMDB(), &inboundCS); err != nil {
		t.Fatalf("failed to apply inbound changeset into crm_store: %v", err)
	}

	// Wait for background ticker to ingest into ONDC DB
	var inboundSynced bool
	for i := 0; i < 40; i++ {
		time.Sleep(25 * time.Millisecond)
		fetched, err := repo.GetTransaction(ctx, "e2e-txn-inbound", "e2e-msg-2")
		if err == nil && fetched != nil && fetched.Payload == `{"item":"replicated-from-sqlite-p2p"}` {
			inboundSynced = true
			break
		}
	}
	if !inboundSynced {
		t.Fatalf("inbound changeset from crm_store did not auto-replicate to ONDC DB within timeout")
	}

	// 5. Verify /status Telemetry reflects successful bidirectional syncs
	resp, err := http.Get(fmt.Sprintf("http://%s/status", httpAddr))
	if err != nil {
		t.Fatalf("failed to query /status: %v", err)
	}
	defer resp.Body.Close()

	var stats p2p.DaemonStats
	if err := json.NewDecoder(resp.Body).Decode(&stats); err != nil {
		t.Fatalf("failed to decode /status JSON: %v", err)
	}

	if stats.ExportCount == 0 {
		t.Errorf("expected ExportCount > 0, got %d", stats.ExportCount)
	}
	if stats.ImportCount == 0 {
		t.Errorf("expected ImportCount > 0, got %d", stats.ImportCount)
	}
	if stats.SyncCount == 0 {
		t.Errorf("expected SyncCount > 0, got %d", stats.SyncCount)
	}

	// 6. Graceful Shutdown Verification
	cancel()
	select {
	case err := <-daemonErrCh:
		if err != nil && err != context.Canceled {
			t.Errorf("daemon did not terminate cleanly: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("daemon failed to shut down within 2 seconds")
	}

	// Verify WAL files persisted on disk
	verifyDB, err := sql.Open("sqlite", ondcPath)
	if err != nil {
		t.Fatalf("failed to reopen ondcDB on disk: %v", err)
	}
	defer verifyDB.Close()

	var count int
	if err := verifyDB.QueryRow("SELECT COUNT(*) FROM transactions").Scan(&count); err != nil {
		t.Fatalf("failed to query transactions on disk: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 persisted transactions on disk, got %d", count)
	}
}
