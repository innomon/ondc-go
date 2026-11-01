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

package storage

import (
	"context"
	"database/sql"
	"fmt"
)

// SchemaDDL defines the SQLite table definitions and indexes.
const SchemaDDL = `
CREATE TABLE IF NOT EXISTS transactions (
    transaction_id TEXT NOT NULL,
    transaction_type TEXT NOT NULL,
    transaction_api TEXT NOT NULL,
    message_id TEXT NOT NULL,
    request_id TEXT NOT NULL,
    payload TEXT NOT NULL CHECK(json_valid(payload)),
    provider_id TEXT NOT NULL,
    message_status TEXT,
    error_type TEXT,
    error_code TEXT,
    error_path TEXT,
    error_message TEXT,
    req_received_time DATETIME,
    additional_data TEXT CHECK(additional_data IS NULL OR json_valid(additional_data)),
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (transaction_id, transaction_type, message_id, request_id)
);

CREATE INDEX IF NOT EXISTS idx_transactions_txn_msg ON transactions (transaction_id, message_id);
CREATE INDEX IF NOT EXISTS idx_transactions_provider ON transactions (provider_id);

CREATE TABLE IF NOT EXISTS registrations (
    registration_id TEXT PRIMARY KEY,
    ondc_environment TEXT NOT NULL,
    ondc_registry_url TEXT NOT NULL,
    entity_name TEXT NOT NULL,
    business_address TEXT,
    gst_details TEXT,
    pan_no TEXT,
    address_signatory TEXT,
    email_id TEXT,
    mobile_number TEXT,
    domains_enabled TEXT,
    app_type TEXT,
    subscriber_id TEXT NOT NULL UNIQUE,
    subscriber_url TEXT NOT NULL,
    signing_public_key TEXT,
    encr_public_key TEXT,
    creation_time DATETIME DEFAULT CURRENT_TIMESTAMP,
    last_modified_time DATETIME DEFAULT CURRENT_TIMESTAMP,
    creation_by TEXT,
    last_modified_by TEXT,
    additional_data TEXT CHECK(additional_data IS NULL OR json_valid(additional_data))
);

CREATE INDEX IF NOT EXISTS idx_registrations_subscriber ON registrations (subscriber_id);
`

// InitSchema applies the SQLite table and index schemas.
func InitSchema(db *sql.DB) error {
	return InitSchemaContext(context.Background(), db)
}

// InitSchemaContext applies the SQLite table and index schemas with context.
func InitSchemaContext(ctx context.Context, db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("init schema: db connection cannot be nil")
	}

	if _, err := db.ExecContext(ctx, "PRAGMA foreign_keys = ON;"); err != nil {
		return fmt.Errorf("enable foreign keys: %w", err)
	}

	if _, err := db.ExecContext(ctx, SchemaDDL); err != nil {
		return fmt.Errorf("execute schema ddl: %w", err)
	}

	return nil
}
