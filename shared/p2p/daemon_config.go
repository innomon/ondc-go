package p2p

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/google/uuid"
)

// BridgeConfig encapsulates configuration parameters for the standalone SQLite-P2P bridge daemon.
type BridgeConfig struct {
	DBPath       string        `json:"db_path"`
	CRMPath      string        `json:"crm_path"`
	NodeID       string        `json:"node_id"`
	SyncInterval time.Duration `json:"sync_interval"`
	MaxBatchSize int           `json:"max_batch_size"`
	HTTPAddr     string        `json:"http_addr"`
}

// LoadBridgeConfig resolves configuration with strict precedence:
// 1. CLI Flags (if provided)
// 2. Environment Variables (if set)
// 3. Sensible Defaults
//
// In accordance with project rules, this implementation uses Go's standard library
// flag package without any third-party CLI dependencies (spf13/cobra).
func LoadBridgeConfig(args []string, lookupEnv func(string) (string, bool)) (*BridgeConfig, error) {
	if lookupEnv == nil {
		lookupEnv = os.LookupEnv
	}

	// 1. Determine Defaults
	defaultDBPath := "./data/ondc.db"
	defaultCRMPath := "./data/sqlite_p2p_crm.db"
	defaultNodeID := ""
	if hostname, err := os.Hostname(); err == nil && hostname != "" {
		defaultNodeID = "node-" + hostname
	} else {
		defaultNodeID = "node-" + uuid.NewString()[:8]
	}
	defaultSyncInterval := 2 * time.Second
	defaultMaxBatchSize := 500
	defaultHTTPAddr := ":8085"

	// 2. Overlay Environment Variables (if present)
	if val, ok := lookupEnv("ONDC_SQLITE_DB_PATH"); ok && val != "" {
		defaultDBPath = val
	}
	if val, ok := lookupEnv("ONDC_SQLITE_P2P_CRM_PATH"); ok && val != "" {
		defaultCRMPath = val
	}
	if val, ok := lookupEnv("ONDC_NODE_ID"); ok && val != "" {
		defaultNodeID = val
	}
	if val, ok := lookupEnv("ONDC_P2P_SYNC_INTERVAL"); ok && val != "" {
		d, err := time.ParseDuration(val)
		if err != nil {
			return nil, fmt.Errorf("invalid ONDC_P2P_SYNC_INTERVAL %q: %w", val, err)
		}
		defaultSyncInterval = d
	}
	if val, ok := lookupEnv("ONDC_P2P_MAX_BATCH_SIZE"); ok && val != "" {
		n, err := strconv.Atoi(val)
		if err != nil || n <= 0 {
			return nil, fmt.Errorf("invalid ONDC_P2P_MAX_BATCH_SIZE %q: must be positive integer", val)
		}
		defaultMaxBatchSize = n
	}
	if val, ok := lookupEnv("ONDC_BRIDGE_HTTP_ADDR"); ok && val != "" {
		defaultHTTPAddr = val
	}

	// 3. Overlay Command-Line Flags (highest precedence)
	fs := flag.NewFlagSet("ondc-p2p-bridge", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	dbPathFlag := fs.String("db-path", defaultDBPath, "Path to the primary ONDC SQLite database")
	crmPathFlag := fs.String("crm-path", defaultCRMPath, "Path to the sqlite-p2p crm_store SQLite database")
	nodeIDFlag := fs.String("node-id", defaultNodeID, "Unique identity of this bridge node")
	syncIntervalFlag := fs.Duration("sync-interval", defaultSyncInterval, "Interval between bidirectional sync passes")
	maxBatchSizeFlag := fs.Int("max-batch-size", defaultMaxBatchSize, "Maximum number of mutations per sync batch")
	httpAddrFlag := fs.String("http-addr", defaultHTTPAddr, "Address to bind the HTTP status and healthcheck server")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil, flag.ErrHelp
		}
		return nil, fmt.Errorf("failed to parse command-line flags: %w", err)
	}

	if *syncIntervalFlag <= 0 {
		return nil, fmt.Errorf("sync-interval must be greater than zero, got: %v", *syncIntervalFlag)
	}
	if *maxBatchSizeFlag <= 0 {
		return nil, fmt.Errorf("max-batch-size must be greater than zero, got: %d", *maxBatchSizeFlag)
	}

	cfg := &BridgeConfig{
		DBPath:       *dbPathFlag,
		CRMPath:      *crmPathFlag,
		NodeID:       *nodeIDFlag,
		SyncInterval: *syncIntervalFlag,
		MaxBatchSize: *maxBatchSizeFlag,
		HTTPAddr:     *httpAddrFlag,
	}

	return cfg, nil
}
