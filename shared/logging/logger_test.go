package logging

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInit_Configuration(t *testing.T) {
	tempDir := t.TempDir()
	cfg := Config{
		Level:          "DEBUG",
		ConsoleEnabled: true,
		FileEnabled:    true,
		Dir:            tempDir,
		FileName:       "app.jsonl",
		MaxSizeMB:      10,
		MaxBackups:     3,
	}

	logger, err := Init(cfg)
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	if logger == nil {
		t.Fatalf("Expected non-nil logger")
	}

	// Verify file is created and writable
	logger.Info("init test log", slog.String("env", "test"))

	logPath := filepath.Join(tempDir, "app.jsonl")
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	var entry map[string]interface{}
	if err := json.Unmarshal(bytes.TrimSpace(data), &entry); err != nil {
		t.Fatalf("Failed to parse log JSON: %v, raw: %s", err, string(data))
	}

	if entry["msg"] != "init test log" {
		t.Errorf("Expected msg 'init test log', got %v", entry["msg"])
	}
	if entry["env"] != "test" {
		t.Errorf("Expected env 'test', got %v", entry["env"])
	}
}

func TestLogger_CallerSourcePreservation(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		Level:     slog.LevelDebug,
		AddSource: true,
	})

	customLogger := NewCustomLogger(slog.New(handler))

	// The line right below this comment will be the emitter line
	customLogger.Infof("calling from test file")

	var record map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &record); err != nil {
		t.Fatalf("Failed to parse JSON record: %v", err)
	}

	source, ok := record["source"].(map[string]interface{})
	if !ok {
		t.Fatalf("Source not present in JSON record: %v", record)
	}

	filePath, _ := source["file"].(string)
	if !strings.HasSuffix(filePath, "logger_test.go") {
		t.Errorf("Expected source file to end with logger_test.go, got %s", filePath)
	}
}

func TestLogger_StructuredAttributesAndContext(t *testing.T) {
	tempDir := t.TempDir()
	cfg := Config{
		Level:          "INFO",
		ConsoleEnabled: false,
		FileEnabled:    true,
		Dir:            tempDir,
		FileName:       "structured.jsonl",
		MaxSizeMB:      5,
		MaxBackups:     2,
	}

	logger, err := Init(cfg)
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	ctx := context.Background()
	logger.InfoContext(ctx, "transaction event",
		slog.String("tx_id", "tx-999"),
		slog.Int("retry_count", 2),
	)

	logPath := filepath.Join(tempDir, "structured.jsonl")
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}

	var entry map[string]interface{}
	if err := json.Unmarshal(bytes.TrimSpace(data), &entry); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if entry["tx_id"] != "tx-999" || entry["retry_count"] != float64(2) {
		t.Errorf("Unexpected entry attrs: %v", entry)
	}
}
