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
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// SQLiteOptions defines configuration for the SQLite repository.
type SQLiteOptions struct {
	DBPath    string
	DB        *sql.DB
	EnableWAL bool
}

// SQLiteRepository provides an embedded SQLite implementation of Repository.
type SQLiteRepository struct {
	db            *sql.DB
	ownsDB        bool
	changeTracker ChangeTracker
}

// SetChangeTracker attaches a change tracker for recording mutation events.
func (r *SQLiteRepository) SetChangeTracker(tracker ChangeTracker) {
	r.changeTracker = tracker
}

// Ensure SQLiteRepository satisfies the Repository interface.
var _ Repository = (*SQLiteRepository)(nil)

// NewSQLiteRepository creates and initializes an embedded SQLite repository.
func NewSQLiteRepository(opts SQLiteOptions) (*SQLiteRepository, error) {
	var db *sql.DB
	var ownsDB bool

	if opts.DB != nil {
		db = opts.DB
		ownsDB = false
	} else {
		if opts.DBPath == "" {
			return nil, fmt.Errorf("new sqlite repository: either DB or DBPath must be provided")
		}

		if opts.DBPath != ":memory:" {
			dir := filepath.Dir(opts.DBPath)
			if dir != "." && dir != "" {
				if err := os.MkdirAll(dir, 0755); err != nil {
					return nil, fmt.Errorf("create db directory %s: %w", dir, err)
				}
			}
		}

		var err error
		dbPath := opts.DBPath
		if dbPath != ":memory:" && !strings.Contains(dbPath, "?") {
			dbPath += "?_busy_timeout=5000"
		}
		db, err = sql.Open("sqlite", dbPath)
		if err != nil {
			return nil, fmt.Errorf("open sqlite database: %w", err)
		}
		// Limit open connections to 1 for write serialization or configure busy timeout
		db.SetMaxOpenConns(1)
		ownsDB = true
	}

	if opts.EnableWAL {
		if _, err := db.Exec("PRAGMA journal_mode = WAL;"); err != nil {
			if ownsDB {
				_ = db.Close()
			}
			return nil, fmt.Errorf("enable wal mode: %w", err)
		}
		if _, err := db.Exec("PRAGMA synchronous = NORMAL;"); err != nil {
			if ownsDB {
				_ = db.Close()
			}
			return nil, fmt.Errorf("set synchronous normal: %w", err)
		}
	}

	if err := InitSchema(db); err != nil {
		if ownsDB {
			_ = db.Close()
		}
		return nil, fmt.Errorf("initialize schema: %w", err)
	}

	return &SQLiteRepository{
		db:     db,
		ownsDB: ownsDB,
	}, nil
}

// SaveTransaction inserts or updates a transaction log entry.
func (r *SQLiteRepository) SaveTransaction(ctx context.Context, tx TransactionRecord) error {
	if tx.TransactionID == "" || tx.MessageID == "" || tx.TransactionType == "" || tx.TransactionAPI == "" {
		return ErrInvalidInput
	}

	if tx.Payload == "" {
		tx.Payload = "{}"
	}

	if tx.ReqReceivedTime.IsZero() {
		tx.ReqReceivedTime = time.Now().UTC()
	}

	query := `
	INSERT INTO transactions (
		transaction_id, transaction_type, transaction_api, message_id, request_id,
		payload, provider_id, message_status, error_type, error_code, error_path,
		error_message, req_received_time, additional_data, created_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	ON CONFLICT(transaction_id, transaction_type, message_id, request_id) DO UPDATE SET
		message_status = excluded.message_status,
		error_type = excluded.error_type,
		error_code = excluded.error_code,
		error_path = excluded.error_path,
		error_message = excluded.error_message,
		payload = excluded.payload,
		additional_data = excluded.additional_data,
		updated_at = CURRENT_TIMESTAMP;
	`

	var addData *string
	if tx.AdditionalData != "" {
		addData = &tx.AdditionalData
	}

	_, err := r.db.ExecContext(ctx, query,
		tx.TransactionID, tx.TransactionType, tx.TransactionAPI, tx.MessageID, tx.RequestID,
		tx.Payload, tx.ProviderID, tx.MessageStatus, tx.ErrorType, tx.ErrorCode, tx.ErrorPath,
		tx.ErrorMessage, tx.ReqReceivedTime.Format(time.RFC3339), addData,
	)
	if err != nil {
		return fmt.Errorf("save transaction: %w", err)
	}

	if r.changeTracker != nil {
		entityID := fmt.Sprintf("%s:%s:%s:%s", tx.TransactionID, tx.TransactionType, tx.MessageID, tx.RequestID)
		payloadBytes, err := json.Marshal(tx)
		if err == nil {
			_ = r.changeTracker.RecordMutation(ctx, "transactions", entityID, "INSERT", string(payloadBytes))
		}
	}

	return nil
}

// GetTransaction retrieves a transaction by TransactionID and MessageID.
func (r *SQLiteRepository) GetTransaction(ctx context.Context, transactionID, messageID string) (*TransactionRecord, error) {
	if transactionID == "" || messageID == "" {
		return nil, ErrInvalidInput
	}

	query := `
	SELECT
		transaction_id, transaction_type, transaction_api, message_id, request_id,
		payload, provider_id, COALESCE(message_status, ''), COALESCE(error_type, ''),
		COALESCE(error_code, ''), COALESCE(error_path, ''), COALESCE(error_message, ''),
		COALESCE(req_received_time, ''), COALESCE(additional_data, ''), created_at, updated_at
	FROM transactions
	WHERE transaction_id = ? AND message_id = ?
	ORDER BY updated_at DESC
	LIMIT 1;
	`

	var tx TransactionRecord
	var reqRecv, createdAt, updatedAt string

	row := r.db.QueryRowContext(ctx, query, transactionID, messageID)
	err := row.Scan(
		&tx.TransactionID, &tx.TransactionType, &tx.TransactionAPI, &tx.MessageID, &tx.RequestID,
		&tx.Payload, &tx.ProviderID, &tx.MessageStatus, &tx.ErrorType, &tx.ErrorCode, &tx.ErrorPath,
		&tx.ErrorMessage, &reqRecv, &tx.AdditionalData, &createdAt, &updatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get transaction: %w", err)
	}

	tx.ReqReceivedTime, _ = time.Parse(time.RFC3339, reqRecv)
	tx.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	tx.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)

	return &tx, nil
}

// ListTransactions retrieves transactions matching the filter.
func (r *SQLiteRepository) ListTransactions(ctx context.Context, filter TransactionFilter) ([]TransactionRecord, error) {
	var whereClauses []string
	var args []any

	if filter.TransactionID != "" {
		whereClauses = append(whereClauses, "transaction_id = ?")
		args = append(args, filter.TransactionID)
	}
	if filter.MessageID != "" {
		whereClauses = append(whereClauses, "message_id = ?")
		args = append(args, filter.MessageID)
	}
	if filter.ProviderID != "" {
		whereClauses = append(whereClauses, "provider_id = ?")
		args = append(args, filter.ProviderID)
	}

	whereSQL := ""
	if len(whereClauses) > 0 {
		whereSQL = "WHERE " + strings.Join(whereClauses, " AND ")
	}

	limit := 100
	if filter.Limit > 0 {
		limit = filter.Limit
	}
	offset := 0
	if filter.Offset > 0 {
		offset = filter.Offset
	}

	query := fmt.Sprintf(`
	SELECT
		transaction_id, transaction_type, transaction_api, message_id, request_id,
		payload, provider_id, COALESCE(message_status, ''), COALESCE(error_type, ''),
		COALESCE(error_code, ''), COALESCE(error_path, ''), COALESCE(error_message, ''),
		COALESCE(req_received_time, ''), COALESCE(additional_data, ''), created_at, updated_at
	FROM transactions
	%s
	ORDER BY created_at DESC
	LIMIT %d OFFSET %d;
	`, whereSQL, limit, offset)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list transactions: %w", err)
	}
	defer rows.Close()

	var records []TransactionRecord
	for rows.Next() {
		var tx TransactionRecord
		var reqRecv, createdAt, updatedAt string
		if err := rows.Scan(
			&tx.TransactionID, &tx.TransactionType, &tx.TransactionAPI, &tx.MessageID, &tx.RequestID,
			&tx.Payload, &tx.ProviderID, &tx.MessageStatus, &tx.ErrorType, &tx.ErrorCode, &tx.ErrorPath,
			&tx.ErrorMessage, &reqRecv, &tx.AdditionalData, &createdAt, &updatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan transaction row: %w", err)
		}
		tx.ReqReceivedTime, _ = time.Parse(time.RFC3339, reqRecv)
		tx.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		tx.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
		records = append(records, tx)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows err: %w", err)
	}

	return records, nil
}

// SaveRegistration saves or updates a participant registration.
func (r *SQLiteRepository) SaveRegistration(ctx context.Context, reg RegistrationRecord) error {
	if reg.RegistrationID == "" || reg.SubscriberID == "" || reg.SubscriberURL == "" {
		return ErrInvalidInput
	}

	query := `
	INSERT INTO registrations (
		registration_id, ondc_environment, ondc_registry_url, entity_name,
		business_address, gst_details, pan_no, address_signatory, email_id,
		mobile_number, domains_enabled, app_type, subscriber_id, subscriber_url,
		signing_public_key, encr_public_key, creation_time, last_modified_time,
		creation_by, last_modified_by, additional_data
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, ?, ?, ?)
	ON CONFLICT(registration_id) DO UPDATE SET
		ondc_environment = excluded.ondc_environment,
		ondc_registry_url = excluded.ondc_registry_url,
		entity_name = excluded.entity_name,
		business_address = excluded.business_address,
		gst_details = excluded.gst_details,
		pan_no = excluded.pan_no,
		address_signatory = excluded.address_signatory,
		email_id = excluded.email_id,
		mobile_number = excluded.mobile_number,
		domains_enabled = excluded.domains_enabled,
		app_type = excluded.app_type,
		subscriber_id = excluded.subscriber_id,
		subscriber_url = excluded.subscriber_url,
		signing_public_key = excluded.signing_public_key,
		encr_public_key = excluded.encr_public_key,
		last_modified_time = CURRENT_TIMESTAMP,
		last_modified_by = excluded.last_modified_by,
		additional_data = excluded.additional_data;
	`

	var addData *string
	if reg.AdditionalData != "" {
		addData = &reg.AdditionalData
	}

	_, err := r.db.ExecContext(ctx, query,
		reg.RegistrationID, reg.ONDCEnvironment, reg.ONDCRegistryURL, reg.EntityName,
		reg.BusinessAddress, reg.GSTDetails, reg.PANNo, reg.AddressSignatory, reg.EmailID,
		reg.MobileNumber, reg.DomainsEnabled, reg.AppType, reg.SubscriberID, reg.SubscriberURL,
		reg.SigningPublicKey, reg.EncrPublicKey, reg.CreationBy, reg.LastModifiedBy, addData,
	)
	if err != nil {
		return fmt.Errorf("save registration: %w", err)
	}

	if r.changeTracker != nil {
		entityID := reg.RegistrationID
		payloadBytes, err := json.Marshal(reg)
		if err == nil {
			_ = r.changeTracker.RecordMutation(ctx, "registrations", entityID, "INSERT", string(payloadBytes))
		}
	}

	return nil
}

// GetRegistration retrieves a registration by RegistrationID.
func (r *SQLiteRepository) GetRegistration(ctx context.Context, registrationID string) (*RegistrationRecord, error) {
	if registrationID == "" {
		return nil, ErrInvalidInput
	}

	query := `
	SELECT
		registration_id, ondc_environment, ondc_registry_url, entity_name,
		COALESCE(business_address, ''), COALESCE(gst_details, ''), COALESCE(pan_no, ''),
		COALESCE(address_signatory, ''), COALESCE(email_id, ''), COALESCE(mobile_number, ''),
		COALESCE(domains_enabled, ''), COALESCE(app_type, ''), subscriber_id, subscriber_url,
		COALESCE(signing_public_key, ''), COALESCE(encr_public_key, ''), creation_time, last_modified_time,
		COALESCE(creation_by, ''), COALESCE(last_modified_by, ''), COALESCE(additional_data, '')
	FROM registrations
	WHERE registration_id = ?;
	`

	var reg RegistrationRecord
	var creationTime, lastModified string

	row := r.db.QueryRowContext(ctx, query, registrationID)
	err := row.Scan(
		&reg.RegistrationID, &reg.ONDCEnvironment, &reg.ONDCRegistryURL, &reg.EntityName,
		&reg.BusinessAddress, &reg.GSTDetails, &reg.PANNo, &reg.AddressSignatory, &reg.EmailID,
		&reg.MobileNumber, &reg.DomainsEnabled, &reg.AppType, &reg.SubscriberID, &reg.SubscriberURL,
		&reg.SigningPublicKey, &reg.EncrPublicKey, &creationTime, &lastModified,
		&reg.CreationBy, &reg.LastModifiedBy, &reg.AdditionalData,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get registration: %w", err)
	}

	reg.CreationTime, _ = time.Parse(time.RFC3339, creationTime)
	reg.LastModifiedTime, _ = time.Parse(time.RFC3339, lastModified)

	return &reg, nil
}

// GetRegistrationBySubscriberID retrieves a registration by SubscriberID.
func (r *SQLiteRepository) GetRegistrationBySubscriberID(ctx context.Context, subscriberID string) (*RegistrationRecord, error) {
	if subscriberID == "" {
		return nil, ErrInvalidInput
	}

	query := `
	SELECT
		registration_id, ondc_environment, ondc_registry_url, entity_name,
		COALESCE(business_address, ''), COALESCE(gst_details, ''), COALESCE(pan_no, ''),
		COALESCE(address_signatory, ''), COALESCE(email_id, ''), COALESCE(mobile_number, ''),
		COALESCE(domains_enabled, ''), COALESCE(app_type, ''), subscriber_id, subscriber_url,
		COALESCE(signing_public_key, ''), COALESCE(encr_public_key, ''), creation_time, last_modified_time,
		COALESCE(creation_by, ''), COALESCE(last_modified_by, ''), COALESCE(additional_data, '')
	FROM registrations
	WHERE subscriber_id = ?;
	`

	var reg RegistrationRecord
	var creationTime, lastModified string

	row := r.db.QueryRowContext(ctx, query, subscriberID)
	err := row.Scan(
		&reg.RegistrationID, &reg.ONDCEnvironment, &reg.ONDCRegistryURL, &reg.EntityName,
		&reg.BusinessAddress, &reg.GSTDetails, &reg.PANNo, &reg.AddressSignatory, &reg.EmailID,
		&reg.MobileNumber, &reg.DomainsEnabled, &reg.AppType, &reg.SubscriberID, &reg.SubscriberURL,
		&reg.SigningPublicKey, &reg.EncrPublicKey, &creationTime, &lastModified,
		&reg.CreationBy, &reg.LastModifiedBy, &reg.AdditionalData,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get registration by subscriber id: %w", err)
	}

	reg.CreationTime, _ = time.Parse(time.RFC3339, creationTime)
	reg.LastModifiedTime, _ = time.Parse(time.RFC3339, lastModified)

	return &reg, nil
}

// OpenSQLiteDB opens a pure Go SQLite database connection.
func OpenSQLiteDB(dbPath string) (*sql.DB, error) {
	return sql.Open("sqlite", dbPath)
}

// Close closes the underlying database connection if owned.
func (r *SQLiteRepository) Close() error {
	if r.ownsDB && r.db != nil {
		return r.db.Close()
	}
	return nil
}
