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
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func TestStatusHandler_ServeHTTP(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	clock := NewLamportClock(5)
	store, err := NewChangeLogStore(db, "node-status-1", clock)
	if err != nil {
		t.Fatalf("create store: %v", err)
	}

	// Record a mutation
	_ = store.RecordChange(context.Background(), ChangeRecord{
		TableName: "transactions",
		EntityID:  "txn-1",
		Operation: OpInsert,
		Payload:   "{}",
	})

	registry := NewPeerRegistry(time.Minute)
	registry.Register(PeerInfo{
		ID:       "peer-alpha",
		Role:     "BPP",
		Address:  "http://127.0.0.1:9090",
		State:    PeerStateActive,
		LastSeen: time.Now(),
	})

	statusHandler := NewStatusHandler(StatusConfig{
		NodeID:       "node-status-1",
		SubscriberID: "bap.test.ondc.org",
		Role:         "BAP",
		Clock:        clock,
		Store:        store,
		Registry:     registry,
		StartTime:    time.Now().Add(-10 * time.Second),
	})

	mux := http.NewServeMux()
	mux.Handle("/p2p/status", statusHandler)

	req := httptest.NewRequest(http.MethodGet, "/p2p/status", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}

	var status NodeStatus
	if err := json.NewDecoder(rec.Body).Decode(&status); err != nil {
		t.Fatalf("failed to decode json status: %v", err)
	}

	if status.NodeID != "node-status-1" || status.SubscriberID != "bap.test.ondc.org" {
		t.Errorf("unexpected node identity: %+v", status)
	}
	if status.Role != "BAP" {
		t.Errorf("unexpected role: %s", status.Role)
	}
	if status.CurrentLamportTime < 6 {
		t.Errorf("expected lamport time >= 6, got %d", status.CurrentLamportTime)
	}
	if status.TotalChanges != 1 {
		t.Errorf("expected 1 total change, got %d", status.TotalChanges)
	}
	if len(status.ActivePeers) != 1 || status.ActivePeers[0].ID != "peer-alpha" {
		t.Errorf("unexpected active peers: %+v", status.ActivePeers)
	}
	if status.UptimeSeconds < 10 {
		t.Errorf("expected uptime >= 10s, got %d", status.UptimeSeconds)
	}

	// Method not allowed for POST
	postReq := httptest.NewRequest(http.MethodPost, "/p2p/status", nil)
	postRec := httptest.NewRecorder()
	mux.ServeHTTP(postRec, postReq)
	if postRec.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405 on POST /p2p/status, got %d", postRec.Code)
	}
}
