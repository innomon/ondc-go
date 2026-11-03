package logging

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
)

func TestCompat_Infof_Errorf_Warningf(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		Level:     slog.LevelDebug,
		AddSource: true,
	})

	// Set active logger to our test buffer
	SetLogger(slog.New(handler))

	Infof("user %s logged in with id %d", "alice", 42)
	Errorf("failed connection to host %s", "127.0.0.1")
	Warningf("high memory usage: %d%%", 85)

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 3 {
		t.Fatalf("Expected 3 log lines, got %d: %s", len(lines), buf.String())
	}

	var entry1, entry2, entry3 map[string]interface{}
	if err := json.Unmarshal([]byte(lines[0]), &entry1); err != nil {
		t.Fatalf("Unmarshal line 1 failed: %v", err)
	}
	if err := json.Unmarshal([]byte(lines[1]), &entry2); err != nil {
		t.Fatalf("Unmarshal line 2 failed: %v", err)
	}
	if err := json.Unmarshal([]byte(lines[2]), &entry3); err != nil {
		t.Fatalf("Unmarshal line 3 failed: %v", err)
	}

	if entry1["level"] != "INFO" || entry1["msg"] != "user alice logged in with id 42" {
		t.Errorf("Unexpected entry1: %v", entry1)
	}
	if entry2["level"] != "ERROR" || entry2["msg"] != "failed connection to host 127.0.0.1" {
		t.Errorf("Unexpected entry2: %v", entry2)
	}
	if entry3["level"] != "WARN" || entry3["msg"] != "high memory usage: 85%" {
		t.Errorf("Unexpected entry3: %v", entry3)
	}

	// Verify caller source for entry1 points to compat_test.go
	source, ok := entry1["source"].(map[string]interface{})
	if !ok {
		t.Fatalf("Source not present in entry1")
	}
	filePath, _ := source["file"].(string)
	if !strings.HasSuffix(filePath, "compat_test.go") {
		t.Errorf("Expected source file to be compat_test.go, got %s", filePath)
	}
}

func TestCompat_VerboseV(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewTextHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})
	SetLogger(slog.New(handler))

	// Level is INFO, so V(1) (Debug) should not be enabled
	if V(1) {
		t.Errorf("Expected V(1) to be false when level is INFO")
	}

	// Now set level to DEBUG
	debugHandler := slog.NewTextHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})
	SetLogger(slog.New(debugHandler))

	if !V(1) {
		t.Errorf("Expected V(1) to be true when level is DEBUG")
	}

	V(1).Infof("verbose debug message: %s", "detailed data")
	if !strings.Contains(buf.String(), "verbose debug message: detailed data") {
		t.Errorf("Expected log buffer to contain verbose message, got %s", buf.String())
	}
}

func TestCompat_ExitAndExitf(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewTextHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})
	SetLogger(slog.New(handler))

	var exitedCode int
	var exitCalled bool
	setExitHandler(func(code int) {
		exitedCode = code
		exitCalled = true
	})
	defer resetExitHandler()

	Exitf("fatal exit code %d", 1)

	if !exitCalled {
		t.Fatalf("Expected exit handler to be called")
	}
	if exitedCode != 1 {
		t.Errorf("Expected exit code 1, got %d", exitedCode)
	}
	if !strings.Contains(buf.String(), "fatal exit code 1") {
		t.Errorf("Expected log buffer to contain fatal message, got %s", buf.String())
	}

	exitCalled = false
	Exit("simple exit")
	if !exitCalled {
		t.Fatalf("Expected Exit to invoke exit handler")
	}

	exitCalled = false
	Exitln("exit with newline")
	if !exitCalled {
		t.Fatalf("Expected Exitln to invoke exit handler")
	}
}

func TestCompat_BasicVariadicMethods(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewTextHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})
	SetLogger(slog.New(handler))

	Info("info", "msg")
	Infoln("infoln", "msg")
	Warning("warning", "msg")
	Warningln("warningln", "msg")
	Error("error", "msg")
	Errorln("errorln", "msg")

	v0 := V(0)
	if !v0 {
		t.Errorf("Expected V(0) to be true for Info level")
	}
	v1 := V(1)
	v1.Info("v1", "info")
	v1.Infoln("v1", "infoln")

	out := buf.String()
	for _, term := range []string{"infomsg", "warningmsg", "errormsg", "v1info"} {
		if !strings.Contains(out, term) {
			t.Errorf("Expected log output to contain %q, got: %s", term, out)
		}
	}
}
