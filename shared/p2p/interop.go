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

package p2p

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// SQLiteP2POperation represents the changeset mutation type in the sqlite-p2p specification.
type SQLiteP2POperation uint8

const (
	// SQLiteP2POpInsert represents a new record insertion in sqlite-p2p.
	SQLiteP2POpInsert SQLiteP2POperation = 1
	// SQLiteP2POpUpdate represents a record modification in sqlite-p2p.
	SQLiteP2POpUpdate SQLiteP2POperation = 2
	// SQLiteP2POpDelete represents a record deletion in sqlite-p2p.
	SQLiteP2POpDelete SQLiteP2POperation = 3
)

// SQLiteP2PChangeset mirrors the binary/JSON changeset wire-format of sqlite-p2p (Hypercore/Autobase stream).
type SQLiteP2PChangeset struct {
	Timestamp int64              `json:"timestamp"`
	Sequence  uint64             `json:"seq"`
	Operation SQLiteP2POperation `json:"op"`
	Key       string             `json:"key"`
	Metadata  json.RawMessage    `json:"metadata,omitempty"`
	Data      []byte             `json:"data,omitempty"`
	PrevHash  []byte             `json:"prev_hash,omitempty"`
}

// SQLiteP2PMetadata encapsulates the metadata envelope stored in sqlite-p2p crm_store entries.
type SQLiteP2PMetadata struct {
	Table       string `json:"table"`
	EntityID    string `json:"entity_id"`
	NodeID      string `json:"node_id"`
	LamportTime uint64 `json:"lamport_time"`
	ChangeID    string `json:"change_id"`
}

// SQLiteP2PSchemaDDL defines the unified key-value table schema from the sqlite-p2p specification.
const SQLiteP2PSchemaDDL = `
CREATE TABLE IF NOT EXISTS crm_store (
    key TEXT PRIMARY KEY,
    metadata TEXT CHECK(json_valid(metadata)),
    data BLOB
) WITHOUT ROWID;
`

// InitSQLiteP2PCRMStore initializes the sqlite-p2p crm_store table and required PRAGMAs on a SQLite database.
func InitSQLiteP2PCRMStore(db *sql.DB) error {
	if db == nil {
		return errors.New("db cannot be nil")
	}

	pragmas := []string{
		"PRAGMA foreign_keys = ON;",
		"PRAGMA journal_mode = WAL;",
		"PRAGMA synchronous = NORMAL;",
	}
	for _, p := range pragmas {
		if _, err := db.Exec(p); err != nil {
			return fmt.Errorf("execute pragma %q: %w", p, err)
		}
	}

	if _, err := db.Exec(SQLiteP2PSchemaDDL); err != nil {
		return fmt.Errorf("create crm_store table: %w", err)
	}

	return nil
}

// FormatSQLiteP2PKey constructs a canonical sqlite-p2p primary key for an ONDC entity.
func FormatSQLiteP2PKey(table, entityID string) string {
	return fmt.Sprintf("ondc:%s:%s", table, entityID)
}

// ParseSQLiteP2PKey decomposes a canonical sqlite-p2p key into table name and entity ID.
func ParseSQLiteP2PKey(key string) (table, entityID string, err error) {
	parts := strings.Split(key, ":")
	if len(parts) < 3 || parts[0] != "ondc" {
		return "", "", fmt.Errorf("invalid ondc sqlite-p2p key format %q", key)
	}
	return parts[1], strings.Join(parts[2:], ":"), nil
}

// ChangeRecordToSQLiteP2PChangeset translates an ONDC ChangeRecord into a sqlite-p2p Changeset.
func ChangeRecordToSQLiteP2PChangeset(chg ChangeRecord) (*SQLiteP2PChangeset, error) {
	var op SQLiteP2POperation
	switch chg.Operation {
	case OpInsert:
		op = SQLiteP2POpInsert
	case OpUpdate:
		op = SQLiteP2POpUpdate
	case OpDelete:
		op = SQLiteP2POpDelete
	default:
		return nil, fmt.Errorf("unsupported ondc operation: %s", chg.Operation)
	}

	meta := SQLiteP2PMetadata{
		Table:       chg.TableName,
		EntityID:    chg.EntityID,
		NodeID:      chg.NodeID,
		LamportTime: chg.LamportTime,
		ChangeID:    chg.ChangeID,
	}
	metaBytes, err := json.Marshal(meta)
	if err != nil {
		return nil, fmt.Errorf("marshal sqlite-p2p metadata: %w", err)
	}

	ts := chg.CreatedAt.UnixNano()
	if ts <= 0 {
		ts = time.Now().UTC().UnixNano()
	}

	return &SQLiteP2PChangeset{
		Timestamp: ts,
		Sequence:  chg.LamportTime,
		Operation: op,
		Key:       FormatSQLiteP2PKey(chg.TableName, chg.EntityID),
		Metadata:  json.RawMessage(metaBytes),
		Data:      []byte(chg.Payload),
	}, nil
}

// SQLiteP2PChangesetToChangeRecord translates a sqlite-p2p Changeset into an ONDC ChangeRecord.
func SQLiteP2PChangesetToChangeRecord(cs *SQLiteP2PChangeset) (*ChangeRecord, error) {
	if cs == nil {
		return nil, errors.New("changeset cannot be nil")
	}

	var op string
	switch cs.Operation {
	case SQLiteP2POpInsert:
		op = OpInsert
	case SQLiteP2POpUpdate:
		op = OpUpdate
	case SQLiteP2POpDelete:
		op = OpDelete
	default:
		return nil, fmt.Errorf("unsupported sqlite-p2p operation: %d", cs.Operation)
	}

	table, entityID, err := ParseSQLiteP2PKey(cs.Key)
	if err != nil {
		// Attempt fallback from metadata
		var meta SQLiteP2PMetadata
		if len(cs.Metadata) > 0 && json.Unmarshal(cs.Metadata, &meta) == nil && meta.Table != "" {
			table = meta.Table
			entityID = meta.EntityID
		} else {
			return nil, fmt.Errorf("failed to extract table and entity from key %q: %w", cs.Key, err)
		}
	}

	var meta SQLiteP2PMetadata
	if len(cs.Metadata) > 0 {
		_ = json.Unmarshal(cs.Metadata, &meta)
	}

	nodeID := meta.NodeID
	if nodeID == "" {
		nodeID = "sqlite-p2p-node"
	}

	changeID := meta.ChangeID
	if changeID == "" {
		changeID = fmt.Sprintf("p2p-%d-%s", cs.Sequence, entityID)
	}

	createdAt := time.Unix(0, cs.Timestamp).UTC()
	if cs.Timestamp <= 0 {
		createdAt = time.Now().UTC()
	}

	return &ChangeRecord{
		ChangeID:    changeID,
		TableName:   table,
		EntityID:    entityID,
		Operation:   op,
		LamportTime: cs.Sequence,
		NodeID:      nodeID,
		Payload:     string(cs.Data),
		CreatedAt:   createdAt,
	}, nil
}

// ApplyChangesetToCRMStore applies a sqlite-p2p changeset to the crm_store table.
func ApplyChangesetToCRMStore(ctx context.Context, db *sql.DB, cs *SQLiteP2PChangeset) error {
	if db == nil || cs == nil {
		return errors.New("db and changeset cannot be nil")
	}

	switch cs.Operation {
	case SQLiteP2POpInsert, SQLiteP2POpUpdate:
		query := `
		INSERT INTO crm_store (key, metadata, data)
		VALUES (?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET
			metadata = excluded.metadata,
			data = excluded.data;
		`
		metaStr := string(cs.Metadata)
		if len(cs.Metadata) == 0 {
			metaStr = "{}"
		}
		_, err := db.ExecContext(ctx, query, cs.Key, metaStr, cs.Data)
		if err != nil {
			return fmt.Errorf("put crm_store %s: %w", cs.Key, err)
		}
		return nil

	case SQLiteP2POpDelete:
		query := `DELETE FROM crm_store WHERE key = ?;`
		_, err := db.ExecContext(ctx, query, cs.Key)
		if err != nil {
			return fmt.Errorf("delete crm_store %s: %w", cs.Key, err)
		}
		return nil

	default:
		return fmt.Errorf("unsupported changeset op: %d", cs.Operation)
	}
}

// ReadRecordFromCRMStore retrieves a record from crm_store by key and parses it as a Changeset.
func ReadRecordFromCRMStore(ctx context.Context, db *sql.DB, key string) (*SQLiteP2PChangeset, error) {
	if db == nil {
		return nil, errors.New("db cannot be nil")
	}

	query := `SELECT key, metadata, data FROM crm_store WHERE key = ?;`
	row := db.QueryRowContext(ctx, query, key)

	var cs SQLiteP2PChangeset
	var metaStr string
	err := row.Scan(&cs.Key, &metaStr, &cs.Data)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errors.New("record not found in crm_store")
	}
	if err != nil {
		return nil, fmt.Errorf("query crm_store %s: %w", key, err)
	}

	cs.Metadata = json.RawMessage(metaStr)
	cs.Operation = SQLiteP2POpUpdate

	var meta SQLiteP2PMetadata
	if err := json.Unmarshal(cs.Metadata, &meta); err == nil {
		cs.Sequence = meta.LamportTime
	}

	return &cs, nil
}

// SQLiteP2PBridge orchestrates bidirectional synchronization between an ONDC SQLite instance
// and a sqlite-p2p crm_store instance.
type SQLiteP2PBridge struct {
	ondcDB      *sql.DB
	p2pStore    *ChangeLogStore
	syncService *SyncService
	crmDB       *sql.DB
}

// NewSQLiteP2PBridge constructs a bidirectional bridge instance.
func NewSQLiteP2PBridge(ondcDB *sql.DB, p2pStore *ChangeLogStore, syncService *SyncService, crmDB *sql.DB) *SQLiteP2PBridge {
	return &SQLiteP2PBridge{
		ondcDB:      ondcDB,
		p2pStore:    p2pStore,
		syncService: syncService,
		crmDB:       crmDB,
	}
}

// ExportToSQLiteP2P reads ONDC changelog mutations since sinceLamportTime, translates them into
// sqlite-p2p Changesets, and applies them to the crm_store table.
func (b *SQLiteP2PBridge) ExportToSQLiteP2P(ctx context.Context, sinceLamportTime uint64) ([]*SQLiteP2PChangeset, error) {
	changes, err := b.p2pStore.GetChangesSince(ctx, sinceLamportTime, 500)
	if err != nil {
		return nil, fmt.Errorf("fetch ondc changes: %w", err)
	}

	var exported []*SQLiteP2PChangeset
	for _, chg := range changes {
		cs, err := ChangeRecordToSQLiteP2PChangeset(chg)
		if err != nil {
			return nil, fmt.Errorf("translate change %s: %w", chg.ChangeID, err)
		}

		if err := ApplyChangesetToCRMStore(ctx, b.crmDB, cs); err != nil {
			return nil, fmt.Errorf("apply changeset %s to crm_store: %w", cs.Key, err)
		}

		exported = append(exported, cs)
	}

	return exported, nil
}

// ImportFromSQLiteP2P translates an array of sqlite-p2p Changesets into ONDC ChangeRecords
// and ingests them into the ONDC SyncService with deterministic Last-Write-Wins (LWW) conflict resolution.
func (b *SQLiteP2PBridge) ImportFromSQLiteP2P(ctx context.Context, changesets []*SQLiteP2PChangeset) (int, error) {
	if len(changesets) == 0 {
		return 0, nil
	}

	var records []ChangeRecord
	for _, cs := range changesets {
		rec, err := SQLiteP2PChangesetToChangeRecord(cs)
		if err != nil {
			return 0, fmt.Errorf("translate changeset %s: %w", cs.Key, err)
		}
		records = append(records, *rec)
	}

	applied, err := b.syncService.ApplyChangeBatch(ctx, records)
	if err != nil {
		return applied, fmt.Errorf("apply batch to ondc: %w", err)
	}

	return applied, nil
}
