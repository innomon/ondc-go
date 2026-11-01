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

package transactionclient

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"ondc/shared/storage"
)

// SQLiteClient implements Client backed by an embedded SQLite repository.
type SQLiteClient struct {
	repo storage.Repository
}

func init() {
	RegisterTransactionClient("sqlite", func(ctx context.Context, projectID, instanceID, databaseID string) (Client, error) {
		// databaseID can be in format "sqlite:<path>" or "sqlite::memory:"
		dbPath := strings.TrimPrefix(databaseID, "sqlite:")
		if dbPath == "" {
			dbPath = ":memory:"
		}
		repo, err := storage.NewSQLiteRepository(storage.SQLiteOptions{
			DBPath:    dbPath,
			EnableWAL: true,
		})
		if err != nil {
			return nil, fmt.Errorf("init sqlite transaction client: %w", err)
		}
		return &SQLiteClient{repo: repo}, nil
	})
}

// NewSQLiteClient returns a transaction Client backed by the given Repository.
func NewSQLiteClient(repo storage.Repository) Client {
	return &SQLiteClient{repo: repo}
}

// StoreTransaction stores transaction details in the SQLite repository.
func (c *SQLiteClient) StoreTransaction(ctx context.Context, transaction TransactionData) error {
	if _, ok := transactionTypeMap[transaction.Type]; !ok {
		return fmt.Errorf("store transaction: invalid type %q", transaction.Type)
	}

	if _, ok := transactionAPIMap[transaction.API]; !ok {
		return fmt.Errorf("store transaction: invalid API %q", transaction.API)
	}

	payloadJSON := "{}"
	if transaction.Payload != nil {
		if b, err := json.Marshal(transaction.Payload); err == nil {
			payloadJSON = string(b)
		}
	}

	record := storage.TransactionRecord{
		TransactionID:   transaction.ID,
		TransactionType: transaction.Type,
		TransactionAPI:  transaction.API,
		MessageID:       transaction.MessageID,
		RequestID:       fmt.Sprintf("%s-%s", transaction.ID, transaction.MessageID),
		Payload:         payloadJSON,
		ProviderID:      transaction.ProviderID,
		MessageStatus:   transaction.MessageStatus,
		ErrorType:       transaction.ErrorType,
		ErrorCode:       transaction.ErrorCode,
		ErrorPath:       transaction.ErrorPath,
		ErrorMessage:    transaction.ErrorMessage,
		ReqReceivedTime: transaction.ReqReceivedTime,
	}

	return c.repo.SaveTransaction(ctx, record)
}
