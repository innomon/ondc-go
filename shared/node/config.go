package node

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	ModeTunnel = "tunnel"
	ModeVM     = "vm"
	ModeLocal  = "local"
)

var (
	ErrInvalidMode = errors.New("invalid deployment mode (must be 'tunnel', 'vm', or 'local')")
)

// NodeConfig encapsulates the configuration for running a unified ONDC node.
type NodeConfig struct {
	Mode         string        `json:"mode"`
	HTTPPort     int           `json:"http_port"`
	HTTPSPort    int           `json:"https_port"`
	Domain       string        `json:"domain"`
	CertDir      string        `json:"cert_dir"`
	DBPath       string        `json:"db_path"`
	CRMPath      string        `json:"crm_path"`
	NodeID       string        `json:"node_id"`
	SyncInterval time.Duration `json:"sync_interval"`
	MaxBatchSize int           `json:"max_batch_size"`
	EnableBuyer  bool          `json:"enable_buyer"`
	EnableSeller bool          `json:"enable_seller"`
	EnableP2P    bool          `json:"enable_p2p"`
	DrainTimeout time.Duration `json:"drain_timeout"`
}

// IsLocalhost checks whether a given domain refers to a loopback address or local test environment.
func IsLocalhost(domain string) bool {
	clean := strings.TrimSpace(strings.ToLower(domain))
	if clean == "" || clean == "localhost" || clean == "127.0.0.1" || clean == "::1" {
		return true
	}
	// Check prefix
	if strings.HasPrefix(clean, "localhost:") || strings.HasPrefix(clean, "127.0.0.1:") {
		return true
	}
	return false
}

// LoadNodeConfig resolves configuration with strict precedence:
// 1. CLI Flags (if specified)
// 2. Environment Variables (if set)
// 3. Sensible Defaults
//
// In accordance with project rules, this implementation uses Go's standard library
// flag package without any third-party CLI dependencies (spf13/cobra).
func LoadNodeConfig(args []string, lookupEnv func(string) (string, bool)) (*NodeConfig, error) {
	if lookupEnv == nil {
		lookupEnv = os.LookupEnv
	}

	// 1. Determine Defaults
	defaultMode := ModeTunnel
	defaultHTTPPort := 8080
	defaultHTTPSPort := 443
	defaultDomain := "localhost"
	defaultCertDir := "./data/certs"
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
	defaultEnableBuyer := true
	defaultEnableSeller := true
	defaultEnableP2P := true
	defaultDrainTimeout := 10 * time.Second

	// 2. Overlay Environment Variables
	if val, ok := lookupEnv("ONDC_MODE"); ok && val != "" {
		defaultMode = strings.ToLower(strings.TrimSpace(val))
	}
	if val, ok := lookupEnv("ONDC_HTTP_PORT"); ok && val != "" {
		if p, err := strconv.Atoi(val); err == nil && p > 0 {
			defaultHTTPPort = p
		}
	}
	if val, ok := lookupEnv("ONDC_HTTPS_PORT"); ok && val != "" {
		if p, err := strconv.Atoi(val); err == nil && p > 0 {
			defaultHTTPSPort = p
		}
	}
	if val, ok := lookupEnv("ONDC_DOMAIN"); ok && val != "" {
		defaultDomain = strings.TrimSpace(val)
	}
	if val, ok := lookupEnv("ONDC_CERT_DIR"); ok && val != "" {
		defaultCertDir = strings.TrimSpace(val)
	}
	if val, ok := lookupEnv("ONDC_SQLITE_DB_PATH"); ok && val != "" {
		defaultDBPath = strings.TrimSpace(val)
	}
	if val, ok := lookupEnv("ONDC_SQLITE_CRM_PATH"); ok && val != "" {
		defaultCRMPath = strings.TrimSpace(val)
	}
	if val, ok := lookupEnv("ONDC_NODE_ID"); ok && val != "" {
		defaultNodeID = strings.TrimSpace(val)
	}
	if val, ok := lookupEnv("ONDC_P2P_SYNC_INTERVAL"); ok && val != "" {
		if d, err := time.ParseDuration(val); err == nil && d > 0 {
			defaultSyncInterval = d
		}
	}
	if val, ok := lookupEnv("ONDC_ENABLE_BUYER"); ok && val != "" {
		if b, err := strconv.ParseBool(val); err == nil {
			defaultEnableBuyer = b
		}
	}
	if val, ok := lookupEnv("ONDC_ENABLE_SELLER"); ok && val != "" {
		if b, err := strconv.ParseBool(val); err == nil {
			defaultEnableSeller = b
		}
	}
	if val, ok := lookupEnv("ONDC_ENABLE_P2P"); ok && val != "" {
		if b, err := strconv.ParseBool(val); err == nil {
			defaultEnableP2P = b
		}
	}
	if val, ok := lookupEnv("ONDC_DRAIN_TIMEOUT"); ok && val != "" {
		if d, err := time.ParseDuration(val); err == nil && d > 0 {
			defaultDrainTimeout = d
		}
	}

	// 3. Define and Parse CLI Flags
	fs := flag.NewFlagSet("ondc-node", flag.ContinueOnError)

	modeFlag := fs.String("mode", defaultMode, "Deployment mode: 'tunnel' (behind firewall/tunnel), 'vm' (direct internet with autocert), or 'local'")
	httpPortFlag := fs.Int("http-port", defaultHTTPPort, "Port to bind standard HTTP listener (or port 80 challenge/redirect in VM mode)")
	httpsPortFlag := fs.Int("https-port", defaultHTTPSPort, "Port to bind HTTPS listener in VM mode")
	domainFlag := fs.String("domain", defaultDomain, "Fully qualified domain name or localhost")
	certDirFlag := fs.String("cert-dir", defaultCertDir, "Directory path for caching TLS certificates")
	dbPathFlag := fs.String("db-path", defaultDBPath, "Path to primary SQLite database file")
	crmPathFlag := fs.String("crm-path", defaultCRMPath, "Path to sqlite-p2p crm_store database file")
	nodeIDFlag := fs.String("node-id", defaultNodeID, "Unique node ID")
	syncIntervalFlag := fs.Duration("sync-interval", defaultSyncInterval, "Interval between P2P replication passes")
	maxBatchSizeFlag := fs.Int("max-batch-size", defaultMaxBatchSize, "Max records per sync batch")
	enableBuyerFlag := fs.Bool("enable-buyer", defaultEnableBuyer, "Enable Buyer Platform endpoints")
	enableSellerFlag := fs.Bool("enable-seller", defaultEnableSeller, "Enable Seller Platform endpoints")
	enableP2PFlag := fs.Bool("enable-p2p", defaultEnableP2P, "Enable embedded SQLite P2P replication engine")
	drainTimeoutFlag := fs.Duration("drain-timeout", defaultDrainTimeout, "Timeout for graceful server shutdown")

	if err := fs.Parse(args); err != nil {
		return nil, err
	}

	cfg := &NodeConfig{
		Mode:         strings.ToLower(strings.TrimSpace(*modeFlag)),
		HTTPPort:     *httpPortFlag,
		HTTPSPort:    *httpsPortFlag,
		Domain:       strings.TrimSpace(*domainFlag),
		CertDir:      strings.TrimSpace(*certDirFlag),
		DBPath:       strings.TrimSpace(*dbPathFlag),
		CRMPath:      strings.TrimSpace(*crmPathFlag),
		NodeID:       strings.TrimSpace(*nodeIDFlag),
		SyncInterval: *syncIntervalFlag,
		MaxBatchSize: *maxBatchSizeFlag,
		EnableBuyer:  *enableBuyerFlag,
		EnableSeller: *enableSellerFlag,
		EnableP2P:    *enableP2PFlag,
		DrainTimeout: *drainTimeoutFlag,
	}

	// Validate mode
	if cfg.Mode != ModeTunnel && cfg.Mode != ModeVM && cfg.Mode != ModeLocal {
		return nil, fmt.Errorf("%w: %q", ErrInvalidMode, cfg.Mode)
	}

	return cfg, nil
}
