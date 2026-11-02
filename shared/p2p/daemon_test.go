package p2p_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"ondc/shared/p2p"
	"ondc/shared/storage"
	_ "modernc.org/sqlite"
)

func setupTestDaemonEnviron(t *testing.T) (*p2p.BridgeDaemon, *p2p.BridgeConfig, *sql.DB, *sql.DB) {
	t.Helper()
	tmpDir := t.TempDir()
	ondcPath := filepath.Join(tmpDir, "ondc.db")
	crmPath := filepath.Join(tmpDir, "crm.db")

	cfg := &p2p.BridgeConfig{
		DBPath:       ondcPath,
		CRMPath:      crmPath,
		NodeID:       "daemon-test-node-01",
		SyncInterval: 50 * time.Millisecond,
		MaxBatchSize: 100,
		HTTPAddr:     ":0",
	}

	daemon, err := p2p.NewBridgeDaemon(cfg)
	if err != nil {
		t.Fatalf("failed to create bridge daemon: %v", err)
	}

	return daemon, cfg, daemon.ONDCDB(), daemon.CRMDB()
}

func TestBridgeDaemon_LifecycleAndSyncOnce(t *testing.T) {
	daemon, _, _, crmDB := setupTestDaemonEnviron(t)
	defer daemon.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Initial stats check
	stats := daemon.Stats()
	if stats.NodeID != "daemon-test-node-01" {
		t.Errorf("expected node ID 'daemon-test-node-01', got '%s'", stats.NodeID)
	}
	if stats.SyncCount != 0 {
		t.Errorf("expected 0 syncs initially, got %d", stats.SyncCount)
	}

	// Insert test transaction into ondcDB
	repo := daemon.Repository()
	now := time.Now().UTC()
	txn := storage.TransactionRecord{
		TransactionID:   "daemon-txn-1",
		TransactionType: "REQUEST-ACTION",
		TransactionAPI:  "search",
		MessageID:       "msg-1",
		RequestID:       "req-1",
		Payload:         `{"status":"created"}`,
		ProviderID:      "provider-1",
		MessageStatus:   "ACK",
		ReqReceivedTime: now,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := repo.SaveTransaction(ctx, txn); err != nil {
		t.Fatalf("failed to save txn: %v", err)
	}

	// Execute manual SyncOnce
	if err := daemon.SyncOnce(ctx); err != nil {
		t.Fatalf("SyncOnce failed: %v", err)
	}

	stats = daemon.Stats()
	if stats.SyncCount != 1 {
		t.Errorf("expected SyncCount 1, got %d", stats.SyncCount)
	}
	if stats.ExportCount != 1 {
		t.Errorf("expected ExportCount 1, got %d", stats.ExportCount)
	}

	// Verify crm_store got the record
	key := p2p.FormatSQLiteP2PKey("transactions", "daemon-txn-1:REQUEST-ACTION:msg-1:req-1")
	record, err := p2p.ReadRecordFromCRMStore(ctx, crmDB, key)
	if err != nil {
		t.Fatalf("failed to read from crm_store: %v", err)
	}
	if record == nil {
		t.Fatalf("expected record in crm_store, got nil")
	}

	// Now insert a remote changeset into crm_store and verify SyncOnce imports it
	remoteTxn := storage.TransactionRecord{
		TransactionID:   "daemon-txn-2",
		TransactionType: "REQUEST-ACTION",
		TransactionAPI:  "on_search",
		MessageID:       "msg-2",
		RequestID:       "req-2",
		Payload:         `{"status":"remote_sync_ok"}`,
		ProviderID:      "provider-2",
		MessageStatus:   "ACK",
		ReqReceivedTime: now,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	remoteData, _ := json.Marshal(remoteTxn)
	remoteMeta, _ := json.Marshal(p2p.SQLiteP2PMetadata{
		Table:       "transactions",
		EntityID:    "daemon-txn-2:REQUEST-ACTION:msg-2:req-2",
		NodeID:      "remote-peer-99",
		LamportTime: 99,
		ChangeID:    "change-99",
	})

	remoteCS := p2p.SQLiteP2PChangeset{
		Sequence:  100,
		Timestamp: time.Now().UnixNano(),
		Operation: p2p.SQLiteP2POpInsert,
		Key:       p2p.FormatSQLiteP2PKey("transactions", "daemon-txn-2:REQUEST-ACTION:msg-2:req-2"),
		Metadata:  remoteMeta,
		Data:      remoteData,
	}
	if err := p2p.ApplyChangesetToCRMStore(ctx, crmDB, &remoteCS); err != nil {
		t.Fatalf("failed to apply remote cs: %v", err)
	}

	if err := daemon.SyncOnce(ctx); err != nil {
		t.Fatalf("second SyncOnce failed: %v", err)
	}

	stats = daemon.Stats()
	if stats.SyncCount != 2 {
		t.Errorf("expected SyncCount 2, got %d", stats.SyncCount)
	}
	if stats.ImportCount != 1 {
		t.Errorf("expected ImportCount 1, got %d", stats.ImportCount)
	}

	// Verify ondcDB has remote transaction
	fetched, err := repo.GetTransaction(ctx, "daemon-txn-2", "msg-2")
	if err != nil {
		t.Fatalf("failed to fetch imported txn: %v", err)
	}
	if fetched == nil || fetched.Payload != `{"status":"remote_sync_ok"}` {
		t.Fatalf("imported txn content mismatch: %+v", fetched)
	}
}

func TestBridgeDaemon_BackgroundStartAndStop(t *testing.T) {
	daemon, _, _, crmDB := setupTestDaemonEnviron(t)
	defer daemon.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start daemon in background
	errCh := make(chan error, 1)
	go func() {
		errCh <- daemon.Start(ctx)
	}()

	// Insert transaction into ondcDB
	repo := daemon.Repository()
	now := time.Now().UTC()
	txn := storage.TransactionRecord{
		TransactionID:   "bg-txn-1",
		TransactionType: "REQUEST-ACTION",
		TransactionAPI:  "search",
		MessageID:       "bg-msg-1",
		RequestID:       "bg-req-1",
		Payload:         `{"bg":"active"}`,
		ProviderID:      "bg-p1",
		MessageStatus:   "ACK",
		ReqReceivedTime: now,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	_ = repo.SaveTransaction(context.Background(), txn)

	// Wait for background ticker to replicate
	key := p2p.FormatSQLiteP2PKey("transactions", "bg-txn-1:REQUEST-ACTION:bg-msg-1:bg-req-1")
	var synced bool
	for i := 0; i < 40; i++ {
		time.Sleep(25 * time.Millisecond)
		rec, _ := p2p.ReadRecordFromCRMStore(context.Background(), crmDB, key)
		if rec != nil {
			synced = true
			break
		}
	}
	if !synced {
		t.Errorf("background sync did not replicate record within timeout")
	}

	// Stop daemon gracefully
	cancel()
	select {
	case err := <-errCh:
		if err != nil && err != context.Canceled {
			t.Errorf("expected clean exit or context.Canceled, got: %v", err)
		}
	case <-time.After(1 * time.Second):
		t.Errorf("daemon did not stop gracefully within 1s")
	}
}
