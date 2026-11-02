// Copyright 2026 ONDC Go Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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

func setupONDCNode(t *testing.T, nodeID string, dbPath string) (*sql.DB, *storage.SQLiteRepository, *p2p.ChangeLogStore, *p2p.SyncService) {
	t.Helper()

	if dbPath == "" {
		dbPath = ":memory:"
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("failed to open ondc db: %v", err)
	}

	if err := storage.InitSchema(db); err != nil {
		t.Fatalf("failed to init ondc schema: %v", err)
	}

	if dbPath != ":memory:" {
		if _, err := db.Exec("PRAGMA journal_mode = WAL;"); err != nil {
			t.Fatalf("failed to enable wal: %v", err)
		}
	}

	clock := p2p.NewLamportClock(0)
	changeStore, err := p2p.NewChangeLogStore(db, nodeID, clock)
	if err != nil {
		t.Fatalf("failed to create changelog store: %v", err)
	}

	repo, err := storage.NewSQLiteRepository(storage.SQLiteOptions{
		DB: db,
	})
	if err != nil {
		t.Fatalf("failed to create sqlite repo: %v", err)
	}
	repo.SetChangeTracker(changeStore)

	syncService := p2p.NewSyncService(p2p.SyncServiceConfig{
		NodeID:   nodeID,
		DB:       db,
		Store:    changeStore,
		Clock:    clock,
		Registry: p2p.NewPeerRegistry(time.Minute),
	})

	return db, repo, changeStore, syncService
}

func setupSQLiteP2PNode(t *testing.T, dbPath string) *sql.DB {
	t.Helper()

	if dbPath == "" {
		dbPath = ":memory:"
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("failed to open sqlite-p2p db: %v", err)
	}

	if err := p2p.InitSQLiteP2PCRMStore(db); err != nil {
		t.Fatalf("failed to init crm_store schema: %v", err)
	}

	return db
}

func TestFormatAndParseSQLiteP2PKey(t *testing.T) {
	key := p2p.FormatSQLiteP2PKey("transactions", "txn-12345")
	if key != "ondc:transactions:txn-12345" {
		t.Fatalf("unexpected key format: %s", key)
	}

	table, entityID, err := p2p.ParseSQLiteP2PKey(key)
	if err != nil {
		t.Fatalf("failed to parse key: %v", err)
	}
	if table != "transactions" || entityID != "txn-12345" {
		t.Fatalf("expected table 'transactions' and entity 'txn-12345', got %s, %s", table, entityID)
	}

	// Invalid key
	if _, _, err := p2p.ParseSQLiteP2PKey("invalid:key"); err == nil {
		t.Fatalf("expected error parsing invalid key, got nil")
	}
}

func TestChangeRecordAndChangesetConversion(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Millisecond)
	chg := p2p.ChangeRecord{
		ChangeID:    "chg-001",
		TableName:   "transactions",
		EntityID:    "txn-999",
		Operation:   p2p.OpInsert,
		LamportTime: 42,
		NodeID:      "node-test",
		Payload:     `{"transaction_id":"txn-999","message_status":"ACK"}`,
		CreatedAt:   now,
	}

	cs, err := p2p.ChangeRecordToSQLiteP2PChangeset(chg)
	if err != nil {
		t.Fatalf("failed to convert ChangeRecord to Changeset: %v", err)
	}

	if cs.Key != "ondc:transactions:txn-999" {
		t.Errorf("expected key ondc:transactions:txn-999, got %s", cs.Key)
	}
	if cs.Operation != p2p.SQLiteP2POpInsert {
		t.Errorf("expected op %d, got %d", p2p.SQLiteP2POpInsert, cs.Operation)
	}
	if cs.Sequence != 42 {
		t.Errorf("expected sequence 42, got %d", cs.Sequence)
	}
	if string(cs.Data) != chg.Payload {
		t.Errorf("expected data %s, got %s", chg.Payload, string(cs.Data))
	}

	// Roundtrip back to ChangeRecord
	reconstructed, err := p2p.SQLiteP2PChangesetToChangeRecord(cs)
	if err != nil {
		t.Fatalf("failed to convert Changeset to ChangeRecord: %v", err)
	}

	if reconstructed.TableName != chg.TableName {
		t.Errorf("expected table %s, got %s", chg.TableName, reconstructed.TableName)
	}
	if reconstructed.EntityID != chg.EntityID {
		t.Errorf("expected entity %s, got %s", chg.EntityID, reconstructed.EntityID)
	}
	if reconstructed.Operation != chg.Operation {
		t.Errorf("expected op %s, got %s", chg.Operation, reconstructed.Operation)
	}
	if reconstructed.LamportTime != chg.LamportTime {
		t.Errorf("expected lamport time %d, got %d", chg.LamportTime, reconstructed.LamportTime)
	}
	if reconstructed.Payload != chg.Payload {
		t.Errorf("expected payload %s, got %s", chg.Payload, reconstructed.Payload)
	}
}

func TestBidirectionalInterop_ONDC_To_SQLiteP2P(t *testing.T) {
	ctx := context.Background()

	ondcDB, repo, changeStore, syncService := setupONDCNode(t, "node-ondc-1", "")
	defer ondcDB.Close()

	crmDB := setupSQLiteP2PNode(t, "")
	defer crmDB.Close()

	bridge := p2p.NewSQLiteP2PBridge(ondcDB, changeStore, syncService, crmDB)

	// Step 1: Write an ONDC transaction in ONDC Go repository
	tx := storage.TransactionRecord{
		TransactionID:   "txn-retail-101",
		TransactionType: "REQUEST-ACTION",
		TransactionAPI:  "search",
		MessageID:       "msg-search-101",
		RequestID:       "req-search-101",
		Payload:         `{"search":{"item":"organic tea"}}`,
		ProviderID:      "prv-retail-1",
		MessageStatus:   "ACK",
		ReqReceivedTime: time.Now().UTC(),
	}

	if err := repo.SaveTransaction(ctx, tx); err != nil {
		t.Fatalf("failed to save transaction in ONDC repo: %v", err)
	}

	// Step 2: Export from ONDC Go to sqlite-p2p crm_store
	exported, err := bridge.ExportToSQLiteP2P(ctx, 0)
	if err != nil {
		t.Fatalf("failed to export to sqlite-p2p: %v", err)
	}
	if len(exported) != 1 {
		t.Fatalf("expected 1 exported changeset, got %d", len(exported))
	}

	expectedKey := p2p.FormatSQLiteP2PKey("transactions", tx.TransactionID+":"+tx.TransactionType+":"+tx.MessageID+":"+tx.RequestID)
	if exported[0].Key != expectedKey {
		t.Fatalf("expected key %s, got %s", expectedKey, exported[0].Key)
	}

	// Step 3: Directly query the sqlite-p2p crm_store table to verify persistence
	var storedKey, storedMeta string
	var storedData []byte
	err = crmDB.QueryRowContext(ctx, "SELECT key, metadata, data FROM crm_store WHERE key = ?", expectedKey).Scan(
		&storedKey, &storedMeta, &storedData,
	)
	if err != nil {
		t.Fatalf("failed to query crm_store in sqlite-p2p: %v", err)
	}

	if storedKey != expectedKey {
		t.Errorf("expected stored key %s, got %s", expectedKey, storedKey)
	}

	var storedTx storage.TransactionRecord
	if err := json.Unmarshal(storedData, &storedTx); err != nil {
		t.Fatalf("failed to unmarshal stored transaction from crm_store data: %v", err)
	}
	if storedTx.TransactionID != tx.TransactionID || storedTx.MessageStatus != "ACK" {
		t.Errorf("data mismatch in sqlite-p2p crm_store: %+v", storedTx)
	}

	// Step 4: Update the transaction in ONDC Go and verify update reflects in sqlite-p2p
	tx.MessageStatus = "PROCESSED"
	if err := repo.SaveTransaction(ctx, tx); err != nil {
		t.Fatalf("failed to update transaction: %v", err)
	}

	exportedUpdates, err := bridge.ExportToSQLiteP2P(ctx, exported[0].Sequence)
	if err != nil {
		t.Fatalf("failed to export update to sqlite-p2p: %v", err)
	}
	if len(exportedUpdates) != 1 {
		t.Fatalf("expected 1 update exported, got %d", len(exportedUpdates))
	}

	// Query crm_store again and verify updated status
	err = crmDB.QueryRowContext(ctx, "SELECT data FROM crm_store WHERE key = ?", expectedKey).Scan(&storedData)
	if err != nil {
		t.Fatalf("failed to query updated crm_store: %v", err)
	}
	if err := json.Unmarshal(storedData, &storedTx); err != nil {
		t.Fatalf("failed to unmarshal updated transaction: %v", err)
	}
	if storedTx.MessageStatus != "PROCESSED" {
		t.Errorf("expected updated message_status PROCESSED, got %s", storedTx.MessageStatus)
	}
}

func TestBidirectionalInterop_SQLiteP2P_To_ONDC(t *testing.T) {
	ctx := context.Background()

	ondcDB, repo, changeStore, syncService := setupONDCNode(t, "node-ondc-2", "")
	defer ondcDB.Close()

	crmDB := setupSQLiteP2PNode(t, "")
	defer crmDB.Close()

	bridge := p2p.NewSQLiteP2PBridge(ondcDB, changeStore, syncService, crmDB)

	// Step 1: Simulate sqlite-p2p peer creating a transaction mutation in crm_store
	peerTx := storage.TransactionRecord{
		TransactionID:   "txn-peer-202",
		TransactionType: "REQUEST-ACTION",
		TransactionAPI:  "select",
		MessageID:       "msg-select-202",
		RequestID:       "req-select-202",
		Payload:         `{"select":{"item_id":"item-42","qty":2}}`,
		ProviderID:      "prv-peer-bpp",
		MessageStatus:   "INITIATED",
		ReqReceivedTime: time.Now().UTC(),
	}
	peerTxData, _ := json.Marshal(peerTx)

	peerMeta := p2p.SQLiteP2PMetadata{
		Table:       "transactions",
		EntityID:    "txn-peer-202",
		NodeID:      "node-sqlite-p2p-alpha",
		LamportTime: 10,
		ChangeID:    "peer-chg-1",
	}
	peerMetaJSON, _ := json.Marshal(peerMeta)

	cs := &p2p.SQLiteP2PChangeset{
		Timestamp: time.Now().UTC().UnixNano(),
		Sequence:  10,
		Operation: p2p.SQLiteP2POpInsert,
		Key:       p2p.FormatSQLiteP2PKey("transactions", "txn-peer-202"),
		Metadata:  peerMetaJSON,
		Data:      peerTxData,
	}

	// Persist to sqlite-p2p crm_store
	if err := p2p.ApplyChangesetToCRMStore(ctx, crmDB, cs); err != nil {
		t.Fatalf("failed to apply changeset to crm_store: %v", err)
	}

	// Step 2: Import changeset from sqlite-p2p into ONDC Go
	applied, err := bridge.ImportFromSQLiteP2P(ctx, []*p2p.SQLiteP2PChangeset{cs})
	if err != nil {
		t.Fatalf("failed to import changeset into ONDC: %v", err)
	}
	if applied != 1 {
		t.Fatalf("expected 1 applied change, got %d", applied)
	}

	// Step 3: Query ONDC Go repository using standard storage APIs
	importedTx, err := repo.GetTransaction(ctx, "txn-peer-202", "msg-select-202")
	if err != nil {
		t.Fatalf("failed to get imported transaction from ONDC repo: %v", err)
	}

	if importedTx.TransactionID != "txn-peer-202" {
		t.Errorf("expected transaction ID txn-peer-202, got %s", importedTx.TransactionID)
	}
	if importedTx.MessageStatus != "INITIATED" {
		t.Errorf("expected message status INITIATED, got %s", importedTx.MessageStatus)
	}
	if importedTx.ProviderID != "prv-peer-bpp" {
		t.Errorf("expected provider ID prv-peer-bpp, got %s", importedTx.ProviderID)
	}

	// Step 4: Simulate sqlite-p2p peer updating the transaction with a higher sequence
	peerTx.MessageStatus = "CONFIRMED"
	updatedData, _ := json.Marshal(peerTx)
	peerMeta.LamportTime = 20
	updatedMetaJSON, _ := json.Marshal(peerMeta)

	csUpdate := &p2p.SQLiteP2PChangeset{
		Timestamp: time.Now().UTC().UnixNano(),
		Sequence:  20,
		Operation: p2p.SQLiteP2POpUpdate,
		Key:       p2p.FormatSQLiteP2PKey("transactions", "txn-peer-202"),
		Metadata:  updatedMetaJSON,
		Data:      updatedData,
	}

	appliedUpdate, err := bridge.ImportFromSQLiteP2P(ctx, []*p2p.SQLiteP2PChangeset{csUpdate})
	if err != nil {
		t.Fatalf("failed to import update into ONDC: %v", err)
	}
	if appliedUpdate != 1 {
		t.Fatalf("expected 1 applied update, got %d", appliedUpdate)
	}

	updatedTx, err := repo.GetTransaction(ctx, "txn-peer-202", "msg-select-202")
	if err != nil {
		t.Fatalf("failed to get updated transaction: %v", err)
	}
	if updatedTx.MessageStatus != "CONFIRMED" {
		t.Errorf("expected updated message status CONFIRMED, got %s", updatedTx.MessageStatus)
	}
}

func TestBidirectionalInterop_Registrations(t *testing.T) {
	ctx := context.Background()

	ondcDB, repo, changeStore, syncService := setupONDCNode(t, "node-ondc-reg", "")
	defer ondcDB.Close()

	crmDB := setupSQLiteP2PNode(t, "")
	defer crmDB.Close()

	bridge := p2p.NewSQLiteP2PBridge(ondcDB, changeStore, syncService, crmDB)

	// Step 1: ONDC Go saves participant registration
	reg := storage.RegistrationRecord{
		RegistrationID:   "reg-sub-555",
		ONDCEnvironment:  "staging",
		ONDCRegistryURL:  "https://staging.registry.ondc.org",
		EntityName:       "Acme Retail BPP",
		SubscriberID:     "bpp.acme.com",
		SubscriberURL:    "https://bpp.acme.com/ondc",
		SigningPublicKey: "ed25519-public-key-base64-test",
		EncrPublicKey:    "x25519-public-key-base64-test",
		CreationBy:       "admin",
		LastModifiedBy:   "admin",
	}

	if err := repo.SaveRegistration(ctx, reg); err != nil {
		t.Fatalf("failed to save registration: %v", err)
	}

	// Export to sqlite-p2p crm_store
	exported, err := bridge.ExportToSQLiteP2P(ctx, 0)
	if err != nil {
		t.Fatalf("failed to export registration to sqlite-p2p: %v", err)
	}
	if len(exported) != 1 {
		t.Fatalf("expected 1 exported registration, got %d", len(exported))
	}

	// Verify existence in crm_store
	expectedKey := "ondc:registrations:reg-sub-555"
	readCS, err := p2p.ReadRecordFromCRMStore(ctx, crmDB, expectedKey)
	if err != nil {
		t.Fatalf("failed to read registration from crm_store: %v", err)
	}

	var storedReg storage.RegistrationRecord
	if err := json.Unmarshal(readCS.Data, &storedReg); err != nil {
		t.Fatalf("failed to unmarshal registration: %v", err)
	}
	if storedReg.SubscriberID != "bpp.acme.com" || storedReg.SigningPublicKey != reg.SigningPublicKey {
		t.Errorf("registration data mismatch in crm_store: %+v", storedReg)
	}

	// Step 2: sqlite-p2p node generates a new registration and imports to ONDC Go
	peerReg := storage.RegistrationRecord{
		RegistrationID:   "reg-sub-777",
		ONDCEnvironment:  "pre-production",
		ONDCRegistryURL:  "https://preprod.registry.ondc.org",
		EntityName:       "Peer Network Buyer",
		SubscriberID:     "bap.peerbuyer.in",
		SubscriberURL:    "https://bap.peerbuyer.in/bap",
		SigningPublicKey: "peer-signing-key-123",
		EncrPublicKey:    "peer-encr-key-456",
		CreationBy:       "peer-admin",
		LastModifiedBy:   "peer-admin",
	}
	peerRegData, _ := json.Marshal(peerReg)
	peerRegMeta, _ := json.Marshal(p2p.SQLiteP2PMetadata{
		Table:       "registrations",
		EntityID:    "reg-sub-777",
		NodeID:      "peer-reg-node",
		LamportTime: 15,
	})

	csReg := &p2p.SQLiteP2PChangeset{
		Timestamp: time.Now().UTC().UnixNano(),
		Sequence:  15,
		Operation: p2p.SQLiteP2POpInsert,
		Key:       p2p.FormatSQLiteP2PKey("registrations", "reg-sub-777"),
		Metadata:  peerRegMeta,
		Data:      peerRegData,
	}

	applied, err := bridge.ImportFromSQLiteP2P(ctx, []*p2p.SQLiteP2PChangeset{csReg})
	if err != nil {
		t.Fatalf("failed to import registration into ONDC: %v", err)
	}
	if applied != 1 {
		t.Fatalf("expected 1 applied registration, got %d", applied)
	}

	gotReg, err := repo.GetRegistration(ctx, "reg-sub-777")
	if err != nil {
		t.Fatalf("failed to get imported registration from repo: %v", err)
	}
	if gotReg.SubscriberID != "bap.peerbuyer.in" || gotReg.SigningPublicKey != "peer-signing-key-123" {
		t.Errorf("imported registration mismatch: %+v", gotReg)
	}
}

func TestBidirectionalInterop_FullMeshRoundtripAndConflictResolution(t *testing.T) {
	ctx := context.Background()

	// Setup Node A (ONDC Go instance)
	ondcDB, repoA, changeStoreA, syncServiceA := setupONDCNode(t, "node-ondc-a", "")
	defer ondcDB.Close()

	// Setup Node B (sqlite-p2p instance)
	crmDB := setupSQLiteP2PNode(t, "")
	defer crmDB.Close()

	bridge := p2p.NewSQLiteP2PBridge(ondcDB, changeStoreA, syncServiceA, crmDB)

	// Step 1: Initial record created on Node A
	txA := storage.TransactionRecord{
		TransactionID:   "txn-mesh-301",
		TransactionType: "REQUEST-ACTION",
		TransactionAPI:  "init",
		MessageID:       "msg-init-301",
		RequestID:       "req-init-301",
		Payload:         `{"items":["item-a"]}`,
		ProviderID:      "prv-1",
		MessageStatus:   "INITIAL_STATE",
		ReqReceivedTime: time.Now().UTC(),
	}
	if err := repoA.SaveTransaction(ctx, txA); err != nil {
		t.Fatalf("Node A SaveTransaction: %v", err)
	}

	// Sync Node A -> Node B
	if _, err := bridge.ExportToSQLiteP2P(ctx, 0); err != nil {
		t.Fatalf("Export to Node B: %v", err)
	}

	// Step 2: Concurrent mutations with deterministic LWW resolution
	// Node A mutates at Lamport Time 5
	txA.MessageStatus = "NODE-A-LOWER-TIMESTAMP"
	txA.Payload = `{"items":["item-a-modified-by-A"]}`
	if err := repoA.SaveTransaction(ctx, txA); err != nil {
		t.Fatalf("Node A update: %v", err)
	}

	// Node B (sqlite-p2p) mutates the same transaction at Lamport Time 25 (higher clock)
	txB := txA
	txB.MessageStatus = "NODE-B-WINNER-TIMESTAMP"
	txB.Payload = `{"items":["item-a-winner-B"]}`
	txBData, _ := json.Marshal(txB)
	metaB, _ := json.Marshal(p2p.SQLiteP2PMetadata{
		Table:       "transactions",
		EntityID:    "txn-mesh-301",
		NodeID:      "node-b-p2p",
		LamportTime: 25,
		ChangeID:    "chg-b-winner",
	})

	csB := &p2p.SQLiteP2PChangeset{
		Timestamp: time.Now().UTC().UnixNano(),
		Sequence:  25,
		Operation: p2p.SQLiteP2POpUpdate,
		Key:       p2p.FormatSQLiteP2PKey("transactions", "txn-mesh-301"),
		Metadata:  metaB,
		Data:      txBData,
	}
	if err := p2p.ApplyChangesetToCRMStore(ctx, crmDB, csB); err != nil {
		t.Fatalf("Apply to crm_store: %v", err)
	}

	// Bidirectional synchronization:
	// Node B changes import into Node A
	applied, err := bridge.ImportFromSQLiteP2P(ctx, []*p2p.SQLiteP2PChangeset{csB})
	if err != nil {
		t.Fatalf("Import to Node A: %v", err)
	}
	if applied != 1 {
		t.Fatalf("expected 1 applied change on Node A, got %d", applied)
	}

	// Verify Node A converged to Node B's version because of higher Lamport Time (25 > 5)
	convergedTx, err := repoA.GetTransaction(ctx, "txn-mesh-301", "msg-init-301")
	if err != nil {
		t.Fatalf("Node A GetTransaction: %v", err)
	}

	if convergedTx.MessageStatus != "NODE-B-WINNER-TIMESTAMP" {
		t.Errorf("expected LWW winner NODE-B-WINNER-TIMESTAMP, got %s", convergedTx.MessageStatus)
	}
	if convergedTx.Payload != `{"items":["item-a-winner-B"]}` {
		t.Errorf("expected converged payload from Node B, got %s", convergedTx.Payload)
	}
}

func TestBidirectionalInterop_RealWALFiles(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()

	ondcDBPath := filepath.Join(tempDir, "ondc_node.db")
	crmDBPath := filepath.Join(tempDir, "crm_node.db")

	ondcDB, repo, changeStore, syncService := setupONDCNode(t, "node-ondc-disk", ondcDBPath)
	defer ondcDB.Close()

	crmDB := setupSQLiteP2PNode(t, crmDBPath)
	defer crmDB.Close()

	bridge := p2p.NewSQLiteP2PBridge(ondcDB, changeStore, syncService, crmDB)

	// Verify WAL mode is active on both real disk databases
	for name, db := range map[string]*sql.DB{"ondc": ondcDB, "crm": crmDB} {
		var jMode string
		if err := db.QueryRow("PRAGMA journal_mode;").Scan(&jMode); err != nil {
			t.Fatalf("failed to query journal_mode for %s: %v", name, err)
		}
		if jMode != "wal" {
			t.Errorf("expected %s journal_mode wal, got %s", name, jMode)
		}
	}

	// Write transaction in ONDC
	txDisk := storage.TransactionRecord{
		TransactionID:   "txn-disk-wal-1",
		TransactionType: "REQUEST-ACTION",
		TransactionAPI:  "confirm",
		MessageID:       "msg-confirm-wal-1",
		RequestID:       "req-confirm-wal-1",
		Payload:         `{"order":{"id":"ord-101","status":"CONFIRMED"}}`,
		ProviderID:      "prv-disk-1",
		MessageStatus:   "ACK",
		ReqReceivedTime: time.Now().UTC(),
	}
	if err := repo.SaveTransaction(ctx, txDisk); err != nil {
		t.Fatalf("failed to save disk transaction: %v", err)
	}

	// Export to sqlite-p2p crm_store file
	if _, err := bridge.ExportToSQLiteP2P(ctx, 0); err != nil {
		t.Fatalf("failed to export to crm_store file: %v", err)
	}

	// Verify record exists in crm_store file
	key := p2p.FormatSQLiteP2PKey("transactions", txDisk.TransactionID+":"+txDisk.TransactionType+":"+txDisk.MessageID+":"+txDisk.RequestID)
	readCS, err := p2p.ReadRecordFromCRMStore(ctx, crmDB, key)
	if err != nil {
		t.Fatalf("failed to read from crm_store file: %v", err)
	}

	var readTx storage.TransactionRecord
	if err := json.Unmarshal(readCS.Data, &readTx); err != nil {
		t.Fatalf("failed to unmarshal tx from disk crm: %v", err)
	}
	if readTx.TransactionID != "txn-disk-wal-1" || readTx.MessageStatus != "ACK" {
		t.Errorf("disk transaction mismatch: %+v", readTx)
	}
}
