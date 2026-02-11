package persistence_test

import (
	"bufio"
	"encoding/binary"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Winston-Lin-9527/redis-in-go/app/config"
	"github.com/Winston-Lin-9527/redis-in-go/app/persistence"
)

// Helper function to create a minimal RDB file for testing
func createTestRDBFile(t *testing.T, dir, filename string) string {
	t.Helper()

	// Ensure directory exists
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("Failed to create test directory: %v", err)
	}

	filePath := filepath.Join(dir, filename)
	file, err := os.Create(filePath)
	if err != nil {
		t.Fatalf("Failed to create test RDB file: %v", err)
	}

	w := bufio.NewWriter(file)
	defer func() {
		w.Flush()
		file.Close()
	}()

	// Write RDB header: "REDIS" + version "0011"
	header := []byte("REDIS0011")
	if _, err := w.Write(header); err != nil {
		t.Fatalf("Failed to write header: %v", err)
	}

	// Write a metadata section (optional but good to test)
	// 0xFA (metadata opcode) + key + value
	w.WriteByte(0xFA) // METADATA_OPCODE
	writeRdbString(t, w, "redis-ver")
	writeRdbString(t, w, "7.0.0")

	// Write database selector (0xFE) + database index (0)
	w.WriteByte(0xFE) // DATABASE_OPCODE
	w.WriteByte(0x00) // DB index 0 (6-bit encoding)

	// Write hashtable size info (0xFB)
	w.WriteByte(0xFB) // DB_HT_SZ_OPCODE
	w.WriteByte(0x02) // 2 total keys
	w.WriteByte(0x01) // 1 key with expiry

	// Write a simple key-value pair without expiration
	// Note: DB_STRING_KV_PAIR_OPCODE (0x00) is also the value type byte
	// So we write 0x00 once, and it serves both purposes
	w.WriteByte(0x00) // This is both the opcode and value type: string
	writeRdbString(t, w, "mykey")
	writeRdbString(t, w, "myvalue")

	// Write a key-value pair with millisecond expiration
	// 0xFC (expiry in ms) + 8-byte timestamp + 0x00 (type) + key + value
	w.WriteByte(0xFC) // DB_KEY_EXPIRE_MILLI_OPCODE
	expireTime := time.Now().Add(1 * time.Hour).UnixMilli()
	binary.Write(w, binary.LittleEndian, uint64(expireTime))
	w.WriteByte(0x00) // value type: string
	writeRdbString(t, w, "expirekey")
	writeRdbString(t, w, "expirevalue")

	// Write EOF section
	// 0xFF + 8-byte checksum (we'll use dummy checksum)
	w.WriteByte(0xFF) // EOF_OPCODE
	binary.Write(w, binary.LittleEndian, uint64(0x0000000000000000))

	return filePath
}

// Helper to write an RDB string with length encoding
func writeRdbString(t *testing.T, w io.Writer, s string) {
	t.Helper()
	length := len(s)

	// Simple 6-bit length encoding (for strings < 64 bytes)
	if length < 64 {
		bw := w.(io.ByteWriter)
		if err := bw.WriteByte(byte(length)); err != nil {
			t.Fatalf("Failed to write string length: %v", err)
		}
	} else {
		t.Fatalf("String too long for simple test encoding: %d", length)
	}

	if _, err := w.Write([]byte(s)); err != nil {
		t.Fatalf("Failed to write string data: %v", err)
	}
}

func TestRDBLoadBasic(t *testing.T) {
	// Create temporary test directory
	testDir := t.TempDir()
	testFilename := "test.rdb"

	// Create test RDB file
	createTestRDBFile(t, testDir, testFilename)

	// Create config
	cfg := config.DefaultConfig()
	cfg.Set("dir", testDir)
	cfg.Set("dbfilename", testFilename)

	// Create RDB instance
	rdb := persistence.NewRDB(cfg)

	// Track loaded keys
	loadedKeys := make(map[string]string)
	loadedExpires := make(map[string]time.Time)

	// Load RDB file
	err := rdb.LoadRDB(func(key, val string, expires time.Time) {
		loadedKeys[key] = val
		if !expires.IsZero() {
			loadedExpires[key] = expires
		}
	})

	if err != nil {
		t.Fatalf("Failed to load RDB: %v", err)
	}

	// Verify loaded data
	if len(loadedKeys) != 2 {
		t.Errorf("Expected 2 keys, got %d", len(loadedKeys))
	}

	if val, ok := loadedKeys["mykey"]; !ok {
		t.Errorf("Key 'mykey' not found")
	} else if val != "myvalue" {
		t.Errorf("Expected value 'myvalue', got '%s'", val)
	}

	if val, ok := loadedKeys["expirekey"]; !ok {
		t.Errorf("Key 'expirekey' not found")
	} else if val != "expirevalue" {
		t.Errorf("Expected value 'expirevalue', got '%s'", val)
	}

	// Verify expiration
	if expires, ok := loadedExpires["expirekey"]; !ok {
		t.Errorf("Expiration for 'expirekey' not found")
	} else if expires.Before(time.Now()) {
		t.Errorf("Expiration time is in the past")
	} else if expires.After(time.Now().Add(2 * time.Hour)) {
		t.Errorf("Expiration time is too far in the future")
	}

	// Verify non-expiring key has no expiration
	if _, ok := loadedExpires["mykey"]; ok {
		t.Errorf("Key 'mykey' should not have an expiration")
	}
}

func TestRDBLoadNonExistentFile(t *testing.T) {
	testDir := t.TempDir()

	cfg := config.DefaultConfig()
	cfg.Set("dir", testDir)
	cfg.Set("dbfilename", "nonexistent.rdb")

	rdb := persistence.NewRDB(cfg)

	err := rdb.LoadRDB(func(key, val string, expires time.Time) {
		t.Errorf("Callback should not be called for non-existent file")
	})

	if err == nil {
		t.Error("Expected error when loading non-existent RDB file")
	}
}

func TestRDBLoadInvalidHeader(t *testing.T) {
	testDir := t.TempDir()
	testFilename := "invalid.rdb"
	filePath := filepath.Join(testDir, testFilename)

	// Create file with invalid header
	if err := os.WriteFile(filePath, []byte("INVALID!"), 0644); err != nil {
		t.Fatalf("Failed to create invalid RDB file: %v", err)
	}

	cfg := config.DefaultConfig()
	cfg.Set("dir", testDir)
	cfg.Set("dbfilename", testFilename)

	rdb := persistence.NewRDB(cfg)

	err := rdb.LoadRDB(func(key, val string, expires time.Time) {
		t.Errorf("Callback should not be called for invalid file")
	})

	if err == nil {
		t.Error("Expected error when loading invalid RDB file")
	}
}

func TestRDBLoadEmptyDatabase(t *testing.T) {
	testDir := t.TempDir()
	testFilename := "empty.rdb"
	filePath := filepath.Join(testDir, testFilename)

	file, err := os.Create(filePath)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	w := bufio.NewWriter(file)

	// Write minimal RDB with no keys
	w.Write([]byte("REDIS0011"))
	w.WriteByte(0xFE) // DATABASE_OPCODE
	w.WriteByte(0x00) // DB index 0
	w.WriteByte(0xFB) // DB_HT_SZ_OPCODE
	w.WriteByte(0x00) // 0 keys
	w.WriteByte(0x00) // 0 expiring keys
	w.WriteByte(0xFF) // EOF_OPCODE
	binary.Write(w, binary.LittleEndian, uint64(0))

	// Explicitly flush and close
	w.Flush()
	file.Close()

	// Verify file was written
	info, err := os.Stat(filePath)
	if err != nil {
		t.Fatalf("Failed to stat RDB file: %v", err)
	}
	t.Logf("RDB file size: %d bytes", info.Size())

	cfg := config.DefaultConfig()
	cfg.Set("dir", testDir)
	cfg.Set("dbfilename", testFilename)

	rdb := persistence.NewRDB(cfg)

	callbackCount := 0
	err = rdb.LoadRDB(func(key, val string, expires time.Time) {
		callbackCount++
	})

	if err != nil {
		t.Fatalf("Failed to load empty RDB: %v", err)
	}

	if callbackCount != 0 {
		t.Errorf("Expected 0 callbacks for empty database, got %d", callbackCount)
	}
}
