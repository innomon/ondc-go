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

// Package storage defines the storage abstractions and domain records for ONDC transaction and registration persistence.
package storage

import (
	"context"
	"errors"
	"time"
)

var (
	// ErrNotFound is returned when a requested record is not found in the repository.
	ErrNotFound = errors.New("record not found")

	// ErrInvalidInput is returned when input data is missing required fields.
	ErrInvalidInput = errors.New("invalid input data")
)

// TransactionRecord represents persistent state for an ONDC transaction.
type TransactionRecord struct {
	TransactionID   string    `json:"transaction_id"`
	TransactionType string    `json:"transaction_type"`
	TransactionAPI  string    `json:"transaction_api"`
	MessageID       string    `json:"message_id"`
	RequestID       string    `json:"request_id"`
	Payload         string    `json:"payload"`
	ProviderID      string    `json:"provider_id"`
	MessageStatus   string    `json:"message_status"`
	ErrorType       string    `json:"error_type,omitempty"`
	ErrorCode       string    `json:"error_code,omitempty"`
	ErrorPath       string    `json:"error_path,omitempty"`
	ErrorMessage    string    `json:"error_message,omitempty"`
	ReqReceivedTime time.Time `json:"req_received_time"`
	AdditionalData  string    `json:"additional_data,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// TransactionFilter specifies filtering options for listing transactions.
type TransactionFilter struct {
	TransactionID string
	MessageID     string
	ProviderID    string
	Limit         int
	Offset        int
}

// RegistrationRecord represents network participant registration data.
type RegistrationRecord struct {
	RegistrationID   string    `json:"registration_id"`
	ONDCEnvironment  string    `json:"ondc_environment"`
	ONDCRegistryURL  string    `json:"ondc_registry_url"`
	EntityName       string    `json:"entity_name"`
	BusinessAddress  string    `json:"business_address"`
	GSTDetails       string    `json:"gst_details"`
	PANNo            string    `json:"pan_no"`
	AddressSignatory string    `json:"address_signatory"`
	EmailID          string    `json:"email_id"`
	MobileNumber     string    `json:"mobile_number"`
	DomainsEnabled   string    `json:"domains_enabled"`
	AppType          string    `json:"app_type"`
	SubscriberID     string    `json:"subscriber_id"`
	SubscriberURL    string    `json:"subscriber_url"`
	SigningPublicKey string    `json:"signing_public_key"`
	EncrPublicKey    string    `json:"encr_public_key"`
	CreationTime     time.Time `json:"creation_time"`
	LastModifiedTime time.Time `json:"last_modified_time"`
	CreationBy       string    `json:"creation_by"`
	LastModifiedBy   string    `json:"last_modified_by"`
	AdditionalData   string    `json:"additional_data,omitempty"`
}

// Repository defines the abstract storage contract for transactions and registrations.
type Repository interface {
	// SaveTransaction persists or updates a transaction log record.
	SaveTransaction(ctx context.Context, tx TransactionRecord) error

	// GetTransaction retrieves a transaction by TransactionID and MessageID.
	GetTransaction(ctx context.Context, transactionID, messageID string) (*TransactionRecord, error)

	// ListTransactions retrieves transactions matching the provided filter.
	ListTransactions(ctx context.Context, filter TransactionFilter) ([]TransactionRecord, error)

	// SaveRegistration persists or updates a participant registration record.
	SaveRegistration(ctx context.Context, reg RegistrationRecord) error

	// GetRegistration retrieves a registration record by RegistrationID.
	GetRegistration(ctx context.Context, registrationID string) (*RegistrationRecord, error)

	// GetRegistrationBySubscriberID retrieves a registration record by SubscriberID.
	GetRegistrationBySubscriberID(ctx context.Context, subscriberID string) (*RegistrationRecord, error)

	// Close terminates any open database resources.
	Close() error
}

// ChangeTracker records mutation operations on stored entities for distributed sync.
type ChangeTracker interface {
	RecordMutation(ctx context.Context, table, entityID, operation, payload string) error
}
