package p2p

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"ondc/shared/storage"
	_ "modernc.org/sqlite"
)

// DaemonStats holds operational telemetry for the BridgeDaemon.
type DaemonStats struct {
	NodeID       string    `json:"node_id"`
	SyncCount    uint64    `json:"sync_count"`
	ExportCount  uint64    `json:"export_count"`
	ImportCount  uint64    `json:"import_count"`
	ErrorCount   uint64    `json:"error_count"`
	LastSyncTime time.Time `json:"last_sync_time"`
	LastError    string    `json:"last_error,omitempty"`
	LamportTime  uint64    `json:"lamport_time"`
}

// BridgeDaemon manages the continuous bidirectional replication process between
// an ondc-go SQLite database and a sqlite-p2p crm_store database.
type BridgeDaemon struct {
	cfg         *BridgeConfig
	ondcDB      *sql.DB
	crmDB       *sql.DB
	repo        *storage.SQLiteRepository
	changeStore *ChangeLogStore
	clock       *LamportClock
	syncService *SyncService
	bridge      *SQLiteP2PBridge

	mu           sync.RWMutex
	syncCount    uint64
	exportCount  uint64
	importCount  uint64
	errorCount   uint64
	lastSyncTime time.Time
	lastError    error
	running      atomic.Bool
	stopCh       chan struct{}
	doneCh       chan struct{}

	lastExportLamport uint64
	lastImportSeq     uint64
}

// NewBridgeDaemon initializes a new BridgeDaemon using the provided BridgeConfig.
// It creates directory structures, connects to both SQLite databases, initializes schemas,
// and wires up the ChangeTracker, SyncService, and SQLiteP2PBridge.
func NewBridgeDaemon(cfg *BridgeConfig) (*BridgeDaemon, error) {
	if cfg == nil {
		return nil, errors.New("bridge config cannot be nil")
	}

	// Ensure parent directories exist
	if cfg.DBPath != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(cfg.DBPath), 0755); err != nil {
			return nil, fmt.Errorf("create dir for db-path: %w", err)
		}
	}
	if cfg.CRMPath != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(cfg.CRMPath), 0755); err != nil {
			return nil, fmt.Errorf("create dir for crm-path: %w", err)
		}
	}

	// Open ondcDB
	ondcDB, err := sql.Open("sqlite", cfg.DBPath)
	if err != nil {
		return nil, fmt.Errorf("open ondc db: %w", err)
	}

	if err := storage.InitSchema(ondcDB); err != nil {
		ondcDB.Close()
		return nil, fmt.Errorf("init ondc schema: %w", err)
	}

	if cfg.DBPath != ":memory:" {
		_, _ = ondcDB.Exec("PRAGMA journal_mode = WAL;")
	}

	// Open crmDB
	crmDB, err := sql.Open("sqlite", cfg.CRMPath)
	if err != nil {
		ondcDB.Close()
		return nil, fmt.Errorf("open crm db: %w", err)
	}

	// Initialize sqlite-p2p crm_store schema
	if err := InitSQLiteP2PCRMStore(crmDB); err != nil {
		ondcDB.Close()
		crmDB.Close()
		return nil, fmt.Errorf("init crm store schema: %w", err)
	}

	clock := NewLamportClock(0)
	changeStore, err := NewChangeLogStore(ondcDB, cfg.NodeID, clock)
	if err != nil {
		ondcDB.Close()
		crmDB.Close()
		return nil, fmt.Errorf("init change store: %w", err)
	}

	// Initialize SQLiteRepository
	repo, err := storage.NewSQLiteRepository(storage.SQLiteOptions{
		DB: ondcDB,
	})
	if err != nil {
		ondcDB.Close()
		crmDB.Close()
		return nil, fmt.Errorf("init ondc repository: %w", err)
	}
	repo.SetChangeTracker(changeStore)

	// Initialize SyncService
	syncSvc := NewSyncService(SyncServiceConfig{
		NodeID:   cfg.NodeID,
		DB:       ondcDB,
		Store:    changeStore,
		Clock:    clock,
		Registry: NewPeerRegistry(time.Minute),
	})

	// Initialize SQLiteP2PBridge
	bridge := NewSQLiteP2PBridge(ondcDB, changeStore, syncSvc, crmDB)

	return &BridgeDaemon{
		cfg:         cfg,
		ondcDB:      ondcDB,
		crmDB:       crmDB,
		repo:        repo,
		changeStore: changeStore,
		clock:       clock,
		syncService: syncSvc,
		bridge:      bridge,
		stopCh:      make(chan struct{}),
		doneCh:      make(chan struct{}),
	}, nil
}

// ONDCDB returns the underlying ondc SQL database handle.
func (d *BridgeDaemon) ONDCDB() *sql.DB {
	return d.ondcDB
}

// CRMDB returns the underlying sqlite-p2p crm_store SQL database handle.
func (d *BridgeDaemon) CRMDB() *sql.DB {
	return d.crmDB
}

// Repository returns the underlying ondc SQLite repository.
func (d *BridgeDaemon) Repository() *storage.SQLiteRepository {
	return d.repo
}

// Config returns the active bridge configuration.
func (d *BridgeDaemon) Config() *BridgeConfig {
	return d.cfg
}

// Stats returns a snapshot of daemon telemetry.
func (d *BridgeDaemon) Stats() DaemonStats {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var lastErrStr string
	if d.lastError != nil {
		lastErrStr = d.lastError.Error()
	}

	var lamport uint64
	if d.clock != nil {
		lamport = d.clock.Current()
	}

	return DaemonStats{
		NodeID:       d.cfg.NodeID,
		SyncCount:    atomic.LoadUint64(&d.syncCount),
		ExportCount:  atomic.LoadUint64(&d.exportCount),
		ImportCount:  atomic.LoadUint64(&d.importCount),
		ErrorCount:   atomic.LoadUint64(&d.errorCount),
		LastSyncTime: d.lastSyncTime,
		LastError:    lastErrStr,
		LamportTime:  lamport,
	}
}

// SyncOnce performs a single pass of bidirectional synchronization:
// 1. Export local uncommitted ONDC mutations to sqlite-p2p crm_store.
// 2. Import remote sqlite-p2p changesets from crm_store into local ONDC tables with LWW conflict resolution.
func (d *BridgeDaemon) SyncOnce(ctx context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	// 1. Export ONDC -> sqlite-p2p crm_store
	exported, err := d.bridge.ExportToSQLiteP2P(ctx, d.lastExportLamport)
	if err != nil {
		atomic.AddUint64(&d.errorCount, 1)
		d.lastError = fmt.Errorf("export to sqlite-p2p: %w", err)
		return d.lastError
	}
	for _, cs := range exported {
		var meta SQLiteP2PMetadata
		if err := json.Unmarshal(cs.Metadata, &meta); err == nil {
			if meta.LamportTime > d.lastExportLamport {
				d.lastExportLamport = meta.LamportTime
			}
		}
	}
	atomic.AddUint64(&d.exportCount, uint64(len(exported)))

	// 2. Import sqlite-p2p crm_store -> ONDC
	// Read pending changesets from crm_store (crm_store is WITHOUT ROWID)
	rows, err := d.crmDB.QueryContext(ctx, "SELECT key, metadata, data FROM crm_store")
	if err != nil {
		atomic.AddUint64(&d.errorCount, 1)
		d.lastError = fmt.Errorf("query crm_store: %w", err)
		return d.lastError
	}
	defer rows.Close()

	var changesets []*SQLiteP2PChangeset
	for rows.Next() {
		var (
			key      string
			metaJSON sql.NullString
			data     []byte
		)
		if err := rows.Scan(&key, &metaJSON, &data); err != nil {
			atomic.AddUint64(&d.errorCount, 1)
			d.lastError = fmt.Errorf("scan crm_store: %w", err)
			return d.lastError
		}

		var rawMeta json.RawMessage
		if metaJSON.Valid {
			rawMeta = json.RawMessage(metaJSON.String)
		}

		changesets = append(changesets, &SQLiteP2PChangeset{
			Key:       key,
			Operation: SQLiteP2POpUpdate,
			Metadata:  rawMeta,
			Data:      data,
		})
	}

	if len(changesets) > 0 {
		applied, err := d.bridge.ImportFromSQLiteP2P(ctx, changesets)
		if err != nil {
			atomic.AddUint64(&d.errorCount, 1)
			d.lastError = fmt.Errorf("import from sqlite-p2p: %w", err)
			return d.lastError
		}
		atomic.AddUint64(&d.importCount, uint64(applied))
	}

	atomic.AddUint64(&d.syncCount, 1)
	d.lastSyncTime = time.Now().UTC()
	d.lastError = nil
	return nil
}

// Start begins the continuous synchronization loop. It blocks until ctx is cancelled
// or Stop() is invoked.
func (d *BridgeDaemon) Start(ctx context.Context) error {
	if !d.running.CompareAndSwap(false, true) {
		return errors.New("daemon is already running")
	}
	defer d.running.Store(false)
	defer close(d.doneCh)

	ticker := time.NewTicker(d.cfg.SyncInterval)
	defer ticker.Stop()

	// Initial sync pass
	_ = d.SyncOnce(ctx)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-d.stopCh:
			return nil
		case <-ticker.C:
			_ = d.SyncOnce(ctx)
		}
	}
}

// Stop signals the background daemon loop to terminate cleanly.
func (d *BridgeDaemon) Stop() {
	if d.running.Load() {
		select {
		case <-d.stopCh:
		default:
			close(d.stopCh)
		}
		<-d.doneCh
	}
}

// Close closes the underlying SQLite database connections.
func (d *BridgeDaemon) Close() error {
	d.Stop()
	var errs []error
	if d.ondcDB != nil {
		if err := d.ondcDB.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if d.crmDB != nil {
		if err := d.crmDB.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("errors closing databases: %v", errs)
	}
	return nil
}
