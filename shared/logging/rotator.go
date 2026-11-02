package logging

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// LogRotator is a thread-safe io.Writer and io.Closer that rotates files when
// they exceed a specified size and prunes older backup files beyond a retention limit.
type LogRotator struct {
	mu         sync.Mutex
	dir        string
	filename   string
	maxSize    int64 // In bytes
	maxBackups int
	file       *os.File
	size       int64
}

// NewLogRotator creates a LogRotator with size specified in megabytes.
func NewLogRotator(dir, filename string, maxSizeMB int, maxBackups int) (*LogRotator, error) {
	if maxSizeMB <= 0 {
		maxSizeMB = 100
	}
	return NewLogRotatorWithBytes(dir, filename, int64(maxSizeMB)*1024*1024, maxBackups)
}

// NewLogRotatorWithBytes creates a LogRotator with size specified in bytes.
func NewLogRotatorWithBytes(dir, filename string, maxBytes int64, maxBackups int) (*LogRotator, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create log dir: %w", err)
	}

	if maxBytes <= 0 {
		maxBytes = 100 * 1024 * 1024
	}
	if maxBackups <= 0 {
		maxBackups = 5
	}

	r := &LogRotator{
		dir:        dir,
		filename:   filename,
		maxSize:    maxBytes,
		maxBackups: maxBackups,
	}

	if err := r.open(); err != nil {
		return nil, err
	}

	return r, nil
}

func (r *LogRotator) open() error {
	path := filepath.Join(r.dir, r.filename)
	info, err := os.Stat(path)
	if err == nil {
		r.size = info.Size()
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat log file: %w", err)
	} else {
		r.size = 0
	}

	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("open log file: %w", err)
	}

	r.file = file
	return nil
}

// Write appends bytes to the active log file, rotating if the write will exceed maxSize.
func (r *LogRotator) Write(p []byte) (n int, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	writeSize := int64(len(p))
	if r.size+writeSize > r.maxSize && r.size > 0 {
		if err := r.rotate(); err != nil {
			fmt.Fprintf(os.Stderr, "log rotation failed: %v\n", err)
		}
	}

	if r.file == nil {
		if err := r.open(); err != nil {
			return 0, err
		}
	}

	n, err = r.file.Write(p)
	r.size += int64(n)
	return n, err
}

func (r *LogRotator) rotate() error {
	if r.file != nil {
		_ = r.file.Close()
		r.file = nil
	}

	path := filepath.Join(r.dir, r.filename)
	ext := filepath.Ext(r.filename)
	base := strings.TrimSuffix(r.filename, ext)

	// Generate timestamped backup name with subsecond resolution to prevent collisions
	timestamp := time.Now().Format("20060102-150405.000")
	backupName := fmt.Sprintf("%s.%s%s", base, timestamp, ext)
	backupPath := filepath.Join(r.dir, backupName)

	// If collision still exists, append sequence number
	seq := 1
	for {
		if _, err := os.Stat(backupPath); os.IsNotExist(err) {
			break
		}
		backupPath = filepath.Join(r.dir, fmt.Sprintf("%s.%s_%d%s", base, timestamp, seq, ext))
		seq++
	}

	if err := os.Rename(path, backupPath); err != nil {
		_ = r.open() // Recover active file
		return fmt.Errorf("rename log file: %w", err)
	}

	r.size = 0
	if err := r.open(); err != nil {
		return fmt.Errorf("open new log file after rotate: %w", err)
	}

	r.pruneBackups()
	return nil
}

func (r *LogRotator) pruneBackups() {
	files, err := os.ReadDir(r.dir)
	if err != nil {
		return
	}

	ext := filepath.Ext(r.filename)
	base := strings.TrimSuffix(r.filename, ext)
	prefix := base + "."

	type backupInfo struct {
		path string
		mod  time.Time
	}

	var backups []backupInfo
	for _, f := range files {
		if f.IsDir() {
			continue
		}
		name := f.Name()
		if strings.HasPrefix(name, prefix) && strings.HasSuffix(name, ext) && name != r.filename {
			path := filepath.Join(r.dir, name)
			if info, err := os.Stat(path); err == nil {
				backups = append(backups, backupInfo{
					path: path,
					mod:  info.ModTime(),
				})
			}
		}
	}

	if len(backups) <= r.maxBackups {
		return
	}

	// Sort oldest first
	sort.Slice(backups, func(i, j int) bool {
		return backups[i].mod.Before(backups[j].mod)
	})

	toDelete := len(backups) - r.maxBackups
	for i := 0; i < toDelete; i++ {
		_ = os.Remove(backups[i].path)
	}
}

// Size returns the current size of the active log file in bytes.
func (r *LogRotator) Size() int64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.size
}

// Close closes the currently opened log file.
func (r *LogRotator) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.file != nil {
		err := r.file.Close()
		r.file = nil
		return err
	}
	return nil
}
