package logging

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestLogRotator_BasicWriteAndClose(t *testing.T) {
	tempDir := t.TempDir()
	fileName := "test.log"

	rotator, err := NewLogRotator(tempDir, fileName, 10, 3)
	if err != nil {
		t.Fatalf("Failed to create LogRotator: %v", err)
	}
	defer rotator.Close()

	payload := []byte("hello structured logging\n")
	n, err := rotator.Write(payload)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	if n != len(payload) {
		t.Fatalf("Expected %d bytes written, got %d", len(payload), n)
	}

	if rotator.Size() != int64(len(payload)) {
		t.Fatalf("Expected size %d, got %d", len(payload), rotator.Size())
	}

	content, err := os.ReadFile(filepath.Join(tempDir, fileName))
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}
	if !bytes.Equal(content, payload) {
		t.Fatalf("Expected %q, got %q", payload, content)
	}
}

func TestLogRotator_RotationOnSize(t *testing.T) {
	tempDir := t.TempDir()
	fileName := "rotation.log"

	// Create rotator with custom byte limit (e.g., 50 bytes)
	rotator, err := NewLogRotatorWithBytes(tempDir, fileName, 50, 3)
	if err != nil {
		t.Fatalf("Failed to create LogRotator: %v", err)
	}
	defer rotator.Close()

	// Write 30 bytes
	chunk1 := []byte("123456789012345678901234567890\n") // 31 bytes
	if _, err := rotator.Write(chunk1); err != nil {
		t.Fatalf("Write chunk1 failed: %v", err)
	}

	// Sleep slightly to guarantee different timestamp if needed
	time.Sleep(10 * time.Millisecond)

	// Write 30 more bytes (total 62 > 50), triggering rotation
	chunk2 := []byte("abcdefghijklmnopqrstuvwxyz1234\n") // 31 bytes
	if _, err := rotator.Write(chunk2); err != nil {
		t.Fatalf("Write chunk2 failed: %v", err)
	}

	// Verify primary log file contains chunk2
	primaryPath := filepath.Join(tempDir, fileName)
	content, err := os.ReadFile(primaryPath)
	if err != nil {
		t.Fatalf("Failed to read primary log: %v", err)
	}
	if !bytes.Equal(content, chunk2) {
		t.Fatalf("Expected primary log to have chunk2 %q, got %q", chunk2, content)
	}

	// Verify rotated backup file exists and contains chunk1
	entries, err := os.ReadDir(tempDir)
	if err != nil {
		t.Fatalf("Failed to read dir: %v", err)
	}

	var backups []string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "rotation.") && e.Name() != fileName {
			backups = append(backups, e.Name())
		}
	}

	if len(backups) != 1 {
		t.Fatalf("Expected 1 backup file, found %d: %v", len(backups), backups)
	}

	backupContent, err := os.ReadFile(filepath.Join(tempDir, backups[0]))
	if err != nil {
		t.Fatalf("Failed to read backup: %v", err)
	}
	if !bytes.Equal(backupContent, chunk1) {
		t.Fatalf("Expected backup to contain chunk1 %q, got %q", chunk1, backupContent)
	}
}

func TestLogRotator_PruneBackups(t *testing.T) {
	tempDir := t.TempDir()
	fileName := "prune.log"
	maxBackups := 2

	rotator, err := NewLogRotatorWithBytes(tempDir, fileName, 20, maxBackups)
	if err != nil {
		t.Fatalf("Failed to create LogRotator: %v", err)
	}
	defer rotator.Close()

	// Trigger 4 rotations by writing chunks of 25 bytes
	for i := 0; i < 4; i++ {
		time.Sleep(15 * time.Millisecond)
		msg := fmt.Sprintf("Rotation iteration #%d msg!\n", i)
		if _, err := rotator.Write([]byte(msg)); err != nil {
			t.Fatalf("Write iteration %d failed: %v", i, err)
		}
	}

	entries, err := os.ReadDir(tempDir)
	if err != nil {
		t.Fatalf("ReadDir failed: %v", err)
	}

	var backups []string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "prune.") && e.Name() != fileName {
			backups = append(backups, e.Name())
		}
	}

	if len(backups) > maxBackups {
		t.Fatalf("Expected at most %d backups, found %d: %v", maxBackups, len(backups), backups)
	}
}

func TestLogRotator_ConcurrentWrites(t *testing.T) {
	tempDir := t.TempDir()
	fileName := "concurrent.log"

	rotator, err := NewLogRotatorWithBytes(tempDir, fileName, 1024, 5)
	if err != nil {
		t.Fatalf("Failed to create LogRotator: %v", err)
	}
	defer rotator.Close()

	var wg sync.WaitGroup
	workers := 10
	messagesPerWorker := 50

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for m := 0; m < messagesPerWorker; m++ {
				line := fmt.Sprintf("Worker %d message %d: line data here\n", workerID, m)
				if _, err := rotator.Write([]byte(line)); err != nil {
					t.Errorf("Concurrent write failed: %v", err)
				}
			}
		}(w)
	}

	wg.Wait()

	if err := rotator.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
}

func TestLogRotator_ReopenExisting(t *testing.T) {
	tempDir := t.TempDir()
	fileName := "reopen.log"
	filePath := filepath.Join(tempDir, fileName)

	// Pre-create file with some data
	initial := []byte("line 1\nline 2\n")
	if err := os.WriteFile(filePath, initial, 0644); err != nil {
		t.Fatalf("Failed to write initial file: %v", err)
	}

	rotator, err := NewLogRotator(tempDir, fileName, 10, 3)
	if err != nil {
		t.Fatalf("Failed to open existing rotator: %v", err)
	}
	defer rotator.Close()

	if rotator.Size() != int64(len(initial)) {
		t.Fatalf("Expected initial size %d, got %d", len(initial), rotator.Size())
	}

	additional := []byte("line 3\n")
	if _, err := rotator.Write(additional); err != nil {
		t.Fatalf("Append write failed: %v", err)
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}
	expected := append(initial, additional...)
	if !bytes.Equal(data, expected) {
		t.Fatalf("Expected %q, got %q", expected, data)
	}
}
