package test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ondc/shared/logging"
	"ondc/shared/node"

	_ "modernc.org/sqlite"
)

func TestLoggingE2E_UnifiedServerJSONLAndCallerSource(t *testing.T) {
	tmpDir := t.TempDir()
	logDir := filepath.Join(tmpDir, "logs")
	logFileName := "unified-node.jsonl"
	logFilePath := filepath.Join(logDir, logFileName)

	// 1. Initialize hybrid logger with file and console
	logger, err := logging.Init(logging.Config{
		Level:          "DEBUG",
		ConsoleEnabled: false,
		FileEnabled:    true,
		Dir:            logDir,
		FileName:       logFileName,
		MaxSizeMB:      10,
		MaxBackups:     3,
	})
	if err != nil {
		t.Fatalf("Failed to initialize logging: %v", err)
	}
	if logger == nil {
		t.Fatalf("Expected non-nil logger")
	}

	// 2. Setup Unified Node with temporary databases
	ondcDBPath := filepath.Join(tmpDir, "ondc.db")
	crmDBPath := filepath.Join(tmpDir, "crm.db")

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to bind listener: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port

	cfg := &node.NodeConfig{
		Mode:          node.ModeTunnel,
		HTTPPort:      port,
		Domain:        "localhost",
		CertDir:       filepath.Join(tmpDir, "certs"),
		DBPath:        ondcDBPath,
		CRMPath:       crmDBPath,
		NodeID:        "node-logging-e2e",
		SyncInterval:  100 * time.Millisecond,
		MaxBatchSize:  100,
		EnableBuyer:   true,
		EnableSeller:  true,
		EnableP2P:     false,
		DrainTimeout:  2 * time.Second,
		LogDir:        logDir,
		LogLevel:      "DEBUG",
		LogMaxSizeMB:  10,
		LogMaxBackups: 3,
		LogConsole:    false,
		LogFile:       true,
	}

	server, err := node.NewUnifiedServer(cfg, node.NodeServices{}, nil)
	if err != nil {
		t.Fatalf("Failed to create UnifiedServer: %v", err)
	}

	serverErrCh := make(chan error, 1)
	go func() {
		if err := server.Serve(listener, nil); err != nil && err != http.ErrServerClosed {
			serverErrCh <- err
		}
	}()

	// Wait for server ready
	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	var ready bool
	for i := 0; i < 20; i++ {
		resp, err := http.Get(baseURL + "/healthz")
		if err == nil && resp.StatusCode == http.StatusOK {
			resp.Body.Close()
			ready = true
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !ready {
		t.Fatalf("Server failed to respond on /healthz")
	}

	// 3. Emit structured log via CustomLogger / compat and slog
	logging.Infof("Unified server handled healthz check from client %s", "e2e-tester")

	// 4. Shutdown server cleanly
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer shutdownCancel()
	_ = server.Shutdown(shutdownCtx)

	// 5. Inspect the generated JSONL log file
	fileData, err := os.ReadFile(logFilePath)
	if err != nil {
		t.Fatalf("Failed to read log file %s: %v", logFilePath, err)
	}

	var foundE2ELog bool
	scanner := bufio.NewScanner(bytes.NewReader(fileData))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}

		var entry map[string]interface{}
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatalf("Log line is not valid JSON (%v): %s", err, line)
		}

		// Verify basic fields
		if _, ok := entry["time"]; !ok {
			t.Errorf("Missing 'time' in entry: %s", line)
		}
		if _, ok := entry["level"]; !ok {
			t.Errorf("Missing 'level' in entry: %s", line)
		}
		if _, ok := entry["msg"]; !ok {
			t.Errorf("Missing 'msg' in entry: %s", line)
		}

		msg, _ := entry["msg"].(string)
		if strings.Contains(msg, "Unified server handled healthz check from client e2e-tester") {
			foundE2ELog = true

			// Verify source attribution points to this test file!
			source, ok := entry["source"].(map[string]interface{})
			if !ok {
				t.Fatalf("Source attribute missing in entry: %s", line)
			}
			fileStr, _ := source["file"].(string)
			if !strings.HasSuffix(fileStr, "logging_e2e_test.go") {
				t.Errorf("Expected caller file to be logging_e2e_test.go, got: %s", fileStr)
			}
		}
	}

	if err := scanner.Err(); err != nil {
		t.Fatalf("Scanner error reading log file: %v", err)
	}

	if !foundE2ELog {
		t.Fatalf("Target log line not found in %s:\n%s", logFilePath, string(fileData))
	}
}

func TestLoggingE2E_HighVolumeRotationAndPruning(t *testing.T) {
	tmpDir := t.TempDir()
	logDir := filepath.Join(tmpDir, "logs_rotation")
	logFileName := "high-volume.jsonl"

	// Create rotator with 10 KB threshold and 2 backups limit
	rotator, err := logging.NewLogRotatorWithBytes(logDir, logFileName, 10*1024, 2)
	if err != nil {
		t.Fatalf("Failed to create rotator: %v", err)
	}
	defer rotator.Close()

	// Write 50 KB of log records to force multiple rotations
	payload := []byte(strings.Repeat("A", 500) + "\n")
	for i := 0; i < 100; i++ {
		if _, err := rotator.Write(payload); err != nil {
			t.Fatalf("Write failed at %d: %v", i, err)
		}
		// Brief sleep between writes to ensure timestamp difference in rotated backup filenames
		time.Sleep(15 * time.Millisecond)
	}

	// Verify directory contents: active file + at most 2 backups
	files, err := os.ReadDir(logDir)
	if err != nil {
		t.Fatalf("Failed to read logDir: %v", err)
	}

	var backupCount int
	var activeFound bool
	for _, f := range files {
		name := f.Name()
		if name == logFileName {
			activeFound = true
		} else if strings.HasPrefix(name, "high-volume.") && strings.HasSuffix(name, ".jsonl") {
			backupCount++
		}
	}

	if !activeFound {
		t.Errorf("Active log file %s not found in directory", logFileName)
	}
	if backupCount > 2 {
		t.Errorf("Expected at most 2 backups after pruning, found %d", backupCount)
	}
	if backupCount == 0 {
		t.Errorf("Expected at least 1 rotated backup file, found 0")
	}
}
