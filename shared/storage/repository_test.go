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
	"context"
	"testing"
	"time"

	"ondc/shared/storage"
)

type mockRepository struct {
	transactions  map[string]storage.TransactionRecord
	registrations map[string]storage.RegistrationRecord
}

func newMockRepository() *mockRepository {
	return &mockRepository{
		transactions:  make(map[string]storage.TransactionRecord),
		registrations: make(map[string]storage.RegistrationRecord),
	}
}

func (m *mockRepository) SaveTransaction(ctx context.Context, tx storage.TransactionRecord) error {
	key := tx.TransactionID + ":" + tx.MessageID
	m.transactions[key] = tx
	return nil
}

func (m *mockRepository) GetTransaction(ctx context.Context, transactionID, messageID string) (*storage.TransactionRecord, error) {
	key := transactionID + ":" + messageID
	tx, ok := m.transactions[key]
	if !ok {
		return nil, storage.ErrNotFound
	}
	return &tx, nil
}

func (m *mockRepository) ListTransactions(ctx context.Context, filter storage.TransactionFilter) ([]storage.TransactionRecord, error) {
	var list []storage.TransactionRecord
	for _, tx := range m.transactions {
		if filter.TransactionID != "" && tx.TransactionID != filter.TransactionID {
			continue
		}
		if filter.MessageID != "" && tx.MessageID != filter.MessageID {
			continue
		}
		list = append(list, tx)
	}
	return list, nil
}

func (m *mockRepository) SaveRegistration(ctx context.Context, reg storage.RegistrationRecord) error {
	m.registrations[reg.RegistrationID] = reg
	return nil
}

func (m *mockRepository) GetRegistration(ctx context.Context, registrationID string) (*storage.RegistrationRecord, error) {
	reg, ok := m.registrations[registrationID]
	if !ok {
		return nil, storage.ErrNotFound
	}
	return &reg, nil
}

func (m *mockRepository) GetRegistrationBySubscriberID(ctx context.Context, subscriberID string) (*storage.RegistrationRecord, error) {
	for _, reg := range m.registrations {
		if reg.SubscriberID == subscriberID {
			return &reg, nil
		}
	}
	return nil, storage.ErrNotFound
}

func (m *mockRepository) Close() error {
	return nil
}

// Verify that mockRepository satisfies storage.Repository interface.
var _ storage.Repository = (*mockRepository)(nil)

func TestRepositoryInterface_TransactionOperations(t *testing.T) {
	ctx := context.Background()
	repo := newMockRepository()

	now := time.Now().UTC().Truncate(time.Second)
	tx := storage.TransactionRecord{
		TransactionID:   "txn-1234-5678",
		TransactionType: "REQUEST-ACTION",
		TransactionAPI:  "search",
		MessageID:       "msg-9876-5432",
		RequestID:       "req-1111",
		Payload:         `{"context":{"action":"search"}}`,
		ProviderID:      "prv-retail-1",
		MessageStatus:   "ACK",
		ReqReceivedTime: now,
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	// Test Save
	if err := repo.SaveTransaction(ctx, tx); err != nil {
		t.Fatalf("SaveTransaction failed: %v", err)
	}

	// Test Get
	got, err := repo.GetTransaction(ctx, tx.TransactionID, tx.MessageID)
	if err != nil {
		t.Fatalf("GetTransaction failed: %v", err)
	}
	if got.TransactionID != tx.TransactionID || got.MessageID != tx.MessageID {
		t.Errorf("GetTransaction returned mismatch: got %+v, want %+v", got, tx)
	}

	// Test Get Not Found
	_, err = repo.GetTransaction(ctx, "non-existent", "msg")
	if err != storage.ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}

	// Test List Filter
	results, err := repo.ListTransactions(ctx, storage.TransactionFilter{TransactionID: tx.TransactionID})
	if err != nil {
		t.Fatalf("ListTransactions failed: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 result, got %d", len(results))
	}
}

func TestRepositoryInterface_RegistrationOperations(t *testing.T) {
	ctx := context.Background()
	repo := newMockRepository()

	now := time.Now().UTC().Truncate(time.Second)
	reg := storage.RegistrationRecord{
		RegistrationID:   "reg-001",
		ONDCEnvironment:  "staging",
		ONDCRegistryURL:  "https://staging.registry.ondc.org",
		EntityName:       "Retail Corp",
		SubscriberID:     "bap.retail.com",
		SubscriberURL:    "https://bap.retail.com/ondc",
		SigningPublicKey: "ed25519-public-key-base64",
		EncrPublicKey:    "x25519-public-key-base64",
		CreationTime:     now,
		LastModifiedTime: now,
	}

	// Test Save
	if err := repo.SaveRegistration(ctx, reg); err != nil {
		t.Fatalf("SaveRegistration failed: %v", err)
	}

	// Test Get by ID
	got, err := repo.GetRegistration(ctx, reg.RegistrationID)
	if err != nil {
		t.Fatalf("GetRegistration failed: %v", err)
	}
	if got.SubscriberID != reg.SubscriberID {
		t.Errorf("SubscriberID mismatch: got %q, want %q", got.SubscriberID, reg.SubscriberID)
	}

	// Test Get by Subscriber ID
	gotSub, err := repo.GetRegistrationBySubscriberID(ctx, reg.SubscriberID)
	if err != nil {
		t.Fatalf("GetRegistrationBySubscriberID failed: %v", err)
	}
	if gotSub.RegistrationID != reg.RegistrationID {
		t.Errorf("RegistrationID mismatch: got %q, want %q", gotSub.RegistrationID, reg.RegistrationID)
	}

	// Test Not Found
	_, err = repo.GetRegistration(ctx, "non-existent")
	if err != storage.ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}
