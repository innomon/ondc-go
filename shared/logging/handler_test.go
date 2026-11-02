package logging

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"
)

type mockHandler struct {
	enabledLevel slog.Level
	records      []slog.Record
	err          error
	attrs        []slog.Attr
	group        string
}

func (m *mockHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= m.enabledLevel
}

func (m *mockHandler) Handle(_ context.Context, r slog.Record) error {
	if m.err != nil {
		return m.err
	}
	m.records = append(m.records, r)
	return nil
}

func (m *mockHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &mockHandler{
		enabledLevel: m.enabledLevel,
		records:      m.records,
		err:          m.err,
		attrs:        append(m.attrs, attrs...),
		group:        m.group,
	}
}

func (m *mockHandler) WithGroup(name string) slog.Handler {
	return &mockHandler{
		enabledLevel: m.enabledLevel,
		records:      m.records,
		err:          m.err,
		attrs:        m.attrs,
		group:        name,
	}
}

func TestMultiHandler_RoutingAndLevelFiltering(t *testing.T) {
	h1 := &mockHandler{enabledLevel: slog.LevelInfo}
	h2 := &mockHandler{enabledLevel: slog.LevelError}

	multi := NewMultiHandler(h1, h2)
	ctx := context.Background()

	// 1. Check Enabled
	if !multi.Enabled(ctx, slog.LevelInfo) {
		t.Fatalf("Expected LevelInfo to be enabled since h1 enables it")
	}
	if !multi.Enabled(ctx, slog.LevelError) {
		t.Fatalf("Expected LevelError to be enabled")
	}
	if multi.Enabled(ctx, slog.LevelDebug) {
		t.Fatalf("Expected LevelDebug to be disabled across all handlers")
	}

	// 2. Handle Info record
	infoRecord := slog.NewRecord(time.Now(), slog.LevelInfo, "info message", 0)
	if err := multi.Handle(ctx, infoRecord); err != nil {
		t.Fatalf("Handle info failed: %v", err)
	}

	if len(h1.records) != 1 {
		t.Fatalf("Expected h1 to receive 1 record, got %d", len(h1.records))
	}
	if len(h2.records) != 0 {
		t.Fatalf("Expected h2 to receive 0 records, got %d", len(h2.records))
	}

	// 3. Handle Error record
	errRecord := slog.NewRecord(time.Now(), slog.LevelError, "error message", 0)
	if err := multi.Handle(ctx, errRecord); err != nil {
		t.Fatalf("Handle error failed: %v", err)
	}

	if len(h1.records) != 2 {
		t.Fatalf("Expected h1 to have 2 records, got %d", len(h1.records))
	}
	if len(h2.records) != 1 {
		t.Fatalf("Expected h2 to have 1 record, got %d", len(h2.records))
	}
}

func TestMultiHandler_WithAttrsAndWithGroup(t *testing.T) {
	var buf bytes.Buffer
	textH := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	var jsonBuf bytes.Buffer
	jsonH := slog.NewJSONHandler(&jsonBuf, &slog.HandlerOptions{Level: slog.LevelDebug})

	multi := NewMultiHandler(textH, jsonH)
	wrapped := multi.WithAttrs([]slog.Attr{slog.String("service", "bap")}).WithGroup("request")

	logger := slog.New(wrapped)
	logger.Info("processed order", slog.String("order_id", "ord-123"))

	// Check text output
	textOutput := buf.String()
	if !strings.Contains(textOutput, "service=bap") || !strings.Contains(textOutput, "request.order_id=ord-123") {
		t.Errorf("Text output missing attrs/groups: %s", textOutput)
	}

	// Check JSON output
	var parsed map[string]interface{}
	if err := json.Unmarshal(jsonBuf.Bytes(), &parsed); err != nil {
		t.Fatalf("Failed to parse JSON output: %v, raw: %s", err, jsonBuf.String())
	}

	if parsed["service"] != "bap" {
		t.Errorf("Expected service=bap, got %v", parsed["service"])
	}
	reqGroup, ok := parsed["request"].(map[string]interface{})
	if !ok || reqGroup["order_id"] != "ord-123" {
		t.Errorf("Expected request.order_id=ord-123, got %v", parsed["request"])
	}
}

func TestMultiHandler_HandleError(t *testing.T) {
	expectedErr := errors.New("write failure")
	hErr := &mockHandler{enabledLevel: slog.LevelInfo, err: expectedErr}
	multi := NewMultiHandler(hErr)

	record := slog.NewRecord(time.Now(), slog.LevelInfo, "test", 0)
	err := multi.Handle(context.Background(), record)
	if !errors.Is(err, expectedErr) {
		t.Fatalf("Expected error %v, got %v", expectedErr, err)
	}
}
