package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadGenesisJSON_Success(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "genesis.json")
	want := []byte(`{"config":{"chainId":43114}}`)
	if err := os.WriteFile(path, want, 0600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	got, err := loadGenesisJSON(path)
	if err != nil {
		t.Fatalf("loadGenesisJSON() error = %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("loadGenesisJSON() = %q, want %q", got, want)
	}
}

func TestLoadGenesisJSON_RequiresJSONExtension(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "genesis.txt")
	if err := os.WriteFile(path, []byte(`{"ok":true}`), 0600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	_, err := loadGenesisJSON(path)
	if err == nil {
		t.Fatal("loadGenesisJSON() expected error for non-json extension")
	}
	if !strings.Contains(err.Error(), ".json extension") {
		t.Fatalf("error = %q, want mention of .json extension", err)
	}
}

func TestLoadGenesisJSON_RejectsNonRegularFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "genesis.json")
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatalf("os.Mkdir() error = %v", err)
	}

	_, err := loadGenesisJSON(path)
	if err == nil {
		t.Fatal("loadGenesisJSON() expected error for non-regular file")
	}
	if !strings.Contains(err.Error(), "regular file") {
		t.Fatalf("error = %q, want mention of regular file", err)
	}
}

func TestLoadGenesisJSON_RejectsInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "genesis.json")
	if err := os.WriteFile(path, []byte(`{"config":`), 0600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	_, err := loadGenesisJSON(path)
	if err == nil {
		t.Fatal("loadGenesisJSON() expected error for invalid json")
	}
	if !strings.Contains(err.Error(), "valid JSON") {
		t.Fatalf("error = %q, want mention of valid JSON", err)
	}
}

func TestLoadGenesisJSON_RejectsOversizedFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "genesis.json")
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("os.Create() error = %v", err)
	}
	if err := file.Truncate(maxGenesisLen + 1); err != nil {
		_ = file.Close()
		t.Fatalf("file.Truncate() error = %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("file.Close() error = %v", err)
	}

	_, err = loadGenesisJSON(path)
	if err == nil {
		t.Fatal("loadGenesisJSON() expected error for oversized genesis")
	}
	if !strings.Contains(err.Error(), "too large") {
		t.Fatalf("error = %q, want mention of too large", err)
	}
}
