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

package storage_test

import (
	"database/sql"
	"testing"

	"ondc/shared/storage"

	_ "modernc.org/sqlite"
)

func TestInitSchema_Success(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open in-memory sqlite: %v", err)
	}
	defer db.Close()

	if err := storage.InitSchema(db); err != nil {
		t.Fatalf("InitSchema failed: %v", err)
	}

	// Verify tables exist
	tables := []string{"transactions", "registrations"}
	for _, tbl := range tables {
		var name string
		query := `SELECT name FROM sqlite_master WHERE type='table' AND name=?;`
		err := db.QueryRow(query, tbl).Scan(&name)
		if err != nil {
			t.Fatalf("table %q not created: %v", tbl, err)
		}
		if name != tbl {
			t.Errorf("expected table name %q, got %q", tbl, name)
		}
	}
}

func TestInitSchema_NilDB(t *testing.T) {
	if err := storage.InitSchema(nil); err == nil {
		t.Fatal("expected error on nil db, got nil")
	}

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open in-memory sqlite: %v", err)
	}
	_ = db.Close()
	if err := storage.InitSchema(db); err == nil {
		t.Fatal("expected error on closed db, got nil")
	}
}

func TestInitSchema_JSONConstraint(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open in-memory sqlite: %v", err)
	}
	defer db.Close()

	if err := storage.InitSchema(db); err != nil {
		t.Fatalf("InitSchema failed: %v", err)
	}

	// Inserting invalid JSON in transactions payload must fail
	badInsert := `INSERT INTO transactions (
		transaction_id, transaction_type, transaction_api, message_id, request_id, payload, provider_id
	) VALUES ('tx-1', 'REQ', 'search', 'msg-1', 'req-1', 'invalid-json-string', 'prv-1');`

	_, err = db.Exec(badInsert)
	if err == nil {
		t.Fatal("expected error on invalid JSON insert into payload, got nil")
	}
}
