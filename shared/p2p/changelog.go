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

// Package p2p provides peer discovery, change-log replication, and synchronization protocols for ONDC sqlite nodes.
package p2p

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"

	"ondc/shared/storage"
)

// Mutation operation types.
const (
	OpInsert = "INSERT"
	OpUpdate = "UPDATE"
	OpDelete = "DELETE"
)

// LamportClock provides a thread-safe monotonic logical clock for causal ordering.
type LamportClock struct {
	mu      sync.RWMutex
	counter uint64
}

// NewLamportClock constructs a new LamportClock with an initial counter value.
func NewLamportClock(initial uint64) *LamportClock {
	return &LamportClock{
		counter: initial,
	}
}

// Current returns the current logical clock value without incrementing.
func (c *LamportClock) Current() uint64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.counter
}

// Increment advances the clock by 1 and returns the new value.
func (c *LamportClock) Increment() uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.counter++
	return c.counter
}

// Witness incorporates an incoming logical timestamp and updates the local clock to max(local, remote) + 1.
func (c *LamportClock) Witness(remote uint64) uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	if remote > c.counter {
		c.counter = remote
	}
	c.counter++
	return c.counter
}

// ChangeRecord encapsulates an individual mutation event replicated across nodes.
type ChangeRecord struct {
	ID          int64     `json:"id,omitempty"`
	ChangeID    string    `json:"change_id"`
	TableName   string    `json:"table_name"`
	EntityID    string    `json:"entity_id"`
	Operation   string    `json:"operation"`
	LamportTime uint64    `json:"lamport_time"`
	NodeID      string    `json:"node_id"`
	Payload     string    `json:"payload"`
	CreatedAt   time.Time `json:"created_at"`
}

// ChangeLogStore manages the persistence and retrieval of change log entries in SQLite.
type ChangeLogStore struct {
	db     *sql.DB
	nodeID string
	clock  *LamportClock
}

// Ensure ChangeLogStore satisfies storage.ChangeTracker.
var _ storage.ChangeTracker = (*ChangeLogStore)(nil)

// NewChangeLogStore initializes the p2p_changelog table and creates a ChangeLogStore.
func NewChangeLogStore(db *sql.DB, nodeID string, clock *LamportClock) (*ChangeLogStore, error) {
	if db == nil {
		return nil, fmt.Errorf("db cannot be nil")
	}
	if clock == nil {
		clock = NewLamportClock(0)
	}

	schema := `
	CREATE TABLE IF NOT EXISTS p2p_changelog (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		change_id TEXT UNIQUE NOT NULL,
		table_name TEXT NOT NULL,
		entity_id TEXT NOT NULL,
		operation TEXT NOT NULL,
		lamport_time INTEGER NOT NULL,
		node_id TEXT NOT NULL,
		payload TEXT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_p2p_changelog_lamport ON p2p_changelog (lamport_time);
	CREATE INDEX IF NOT EXISTS idx_p2p_changelog_entity ON p2p_changelog (table_name, entity_id);
	`

	if _, err := db.Exec(schema); err != nil {
		return nil, fmt.Errorf("init changelog table: %w", err)
	}

	store := &ChangeLogStore{
		db:     db,
		nodeID: nodeID,
		clock:  clock,
	}

	// Synchronize clock with highest recorded lamport time if initial clock was 0
	if clock.Current() == 0 {
		latest, err := store.GetLatestLamportTime(context.Background())
		if err == nil && latest > 0 {
			clock.mu.Lock()
			if latest > clock.counter {
				clock.counter = latest
			}
			clock.mu.Unlock()
		}
	}

	return store, nil
}

// RecordChange records a new change entry with a locally assigned Lamport timestamp.
func (s *ChangeLogStore) RecordChange(ctx context.Context, chg ChangeRecord) error {
	if chg.LamportTime == 0 {
		chg.LamportTime = s.clock.Increment()
	}
	if chg.NodeID == "" {
		chg.NodeID = s.nodeID
	}
	if chg.ChangeID == "" {
		chg.ChangeID = fmt.Sprintf("%s-%d", chg.NodeID, chg.LamportTime)
	}
	if chg.CreatedAt.IsZero() {
		chg.CreatedAt = time.Now().UTC()
	}

	query := `
	INSERT INTO p2p_changelog (
		change_id, table_name, entity_id, operation, lamport_time, node_id, payload, created_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?);
	`

	_, err := s.db.ExecContext(ctx, query,
		chg.ChangeID, chg.TableName, chg.EntityID, chg.Operation,
		chg.LamportTime, chg.NodeID, chg.Payload, chg.CreatedAt.Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("record change: %w", err)
	}

	return nil
}

// RecordMutation adapts storage.ChangeTracker calls into ChangeRecord entries.
func (s *ChangeLogStore) RecordMutation(ctx context.Context, table, entityID, operation, payload string) error {
	chg := ChangeRecord{
		TableName: table,
		EntityID:  entityID,
		Operation: operation,
		Payload:   payload,
	}
	return s.RecordChange(ctx, chg)
}

// GetChangesSince retrieves changes with a Lamport timestamp strictly greater than sinceLamportTime.
func (s *ChangeLogStore) GetChangesSince(ctx context.Context, sinceLamportTime uint64, limit int) ([]ChangeRecord, error) {
	if limit <= 0 {
		limit = 100
	}

	query := `
	SELECT id, change_id, table_name, entity_id, operation, lamport_time, node_id, payload, created_at
	FROM p2p_changelog
	WHERE lamport_time > ?
	ORDER BY lamport_time ASC, id ASC
	LIMIT ?;
	`

	rows, err := s.db.QueryContext(ctx, query, sinceLamportTime, limit)
	if err != nil {
		return nil, fmt.Errorf("query changes since: %w", err)
	}
	defer rows.Close()

	var records []ChangeRecord
	for rows.Next() {
		var r ChangeRecord
		var createdAt string
		if err := rows.Scan(
			&r.ID, &r.ChangeID, &r.TableName, &r.EntityID, &r.Operation,
			&r.LamportTime, &r.NodeID, &r.Payload, &createdAt,
		); err != nil {
			return nil, fmt.Errorf("scan changelog row: %w", err)
		}
		r.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		records = append(records, r)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows err: %w", err)
	}

	return records, nil
}

// GetLatestLamportTime returns the highest Lamport timestamp persisted in the changelog.
func (s *ChangeLogStore) GetLatestLamportTime(ctx context.Context) (uint64, error) {
	query := `SELECT COALESCE(MAX(lamport_time), 0) FROM p2p_changelog;`
	var latest uint64
	err := s.db.QueryRowContext(ctx, query).Scan(&latest)
	if err != nil {
		return 0, fmt.Errorf("get latest lamport time: %w", err)
	}
	return latest, nil
}

// ApplyRemoteChange ingests a change from a remote peer, updating the local Lamport clock.
func (s *ChangeLogStore) ApplyRemoteChange(ctx context.Context, chg ChangeRecord) error {
	s.clock.Witness(chg.LamportTime)

	query := `
	INSERT OR IGNORE INTO p2p_changelog (
		change_id, table_name, entity_id, operation, lamport_time, node_id, payload, created_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?);
	`

	_, err := s.db.ExecContext(ctx, query,
		chg.ChangeID, chg.TableName, chg.EntityID, chg.Operation,
		chg.LamportTime, chg.NodeID, chg.Payload, chg.CreatedAt.Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("apply remote change: %w", err)
	}

	return nil
}
