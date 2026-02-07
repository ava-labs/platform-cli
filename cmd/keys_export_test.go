package cmd

import (
	"io"
	"os"
	"strings"
	"testing"

	"github.com/ava-labs/platform-cli/pkg/keystore"
)

func TestKeysExportUsesEnvPassword(t *testing.T) {
	const (
		testKeyName  = "env-password-export"
		testPassword = "testpassword123"
	)

	t.Setenv("HOME", t.TempDir())
	t.Setenv("PLATFORM_CLI_KEY_PASSWORD", testPassword)

	ks, err := keystore.Load()
	if err != nil {
		t.Fatalf("keystore.Load() error = %v", err)
	}
	keyCopy := make([]byte, len(ewoqPrivateKey))
	copy(keyCopy, ewoqPrivateKey)
	if err := ks.ImportKey(testKeyName, keyCopy, []byte(testPassword)); err != nil {
		t.Fatalf("ImportKey() error = %v", err)
	}

	origKeyName := keyName
	origKeyFormat := keyFormat
	origKeyExportUnsafe := keyExportUnsafe
	origKeyExportFile := keyExportFile
	defer func() {
		keyName = origKeyName
		keyFormat = origKeyFormat
		keyExportUnsafe = origKeyExportUnsafe
		keyExportFile = origKeyExportFile
	}()
	keyName = testKeyName
	keyFormat = "cb58"
	keyExportUnsafe = true
	keyExportFile = ""

	origStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() error = %v", err)
	}
	os.Stdout = w

	runErr := keysExportCmd.RunE(keysExportCmd, nil)

	_ = w.Close()
	os.Stdout = origStdout
	out, _ := io.ReadAll(r)
	_ = r.Close()

	if runErr != nil {
		t.Fatalf("keys export run error = %v", runErr)
	}
	if !strings.Contains(string(out), "PrivateKey-") {
		t.Fatalf("keys export output = %q, want PrivateKey- prefix", string(out))
	}
}

func TestKeysExportWritesToFileByDefault(t *testing.T) {
	const (
		testKeyName  = "file-export"
		testPassword = "testpassword123"
	)

	t.Setenv("HOME", t.TempDir())
	t.Setenv("PLATFORM_CLI_KEY_PASSWORD", testPassword)

	ks, err := keystore.Load()
	if err != nil {
		t.Fatalf("keystore.Load() error = %v", err)
	}
	keyCopy := make([]byte, len(ewoqPrivateKey))
	copy(keyCopy, ewoqPrivateKey)
	if err := ks.ImportKey(testKeyName, keyCopy, []byte(testPassword)); err != nil {
		t.Fatalf("ImportKey() error = %v", err)
	}

	exportPath := t.TempDir() + "/exported.key"

	origKeyName := keyName
	origKeyFormat := keyFormat
	origKeyExportUnsafe := keyExportUnsafe
	origKeyExportFile := keyExportFile
	defer func() {
		keyName = origKeyName
		keyFormat = origKeyFormat
		keyExportUnsafe = origKeyExportUnsafe
		keyExportFile = origKeyExportFile
	}()
	keyName = testKeyName
	keyFormat = "cb58"
	keyExportUnsafe = false
	keyExportFile = exportPath

	runErr := keysExportCmd.RunE(keysExportCmd, nil)
	if runErr != nil {
		t.Fatalf("keys export run error = %v", runErr)
	}

	data, err := os.ReadFile(exportPath)
	if err != nil {
		t.Fatalf("os.ReadFile() error = %v", err)
	}
	if !strings.Contains(string(data), "PrivateKey-") {
		t.Fatalf("export file content = %q, want PrivateKey- prefix", string(data))
	}

	info, err := os.Stat(exportPath)
	if err != nil {
		t.Fatalf("os.Stat() error = %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("export file mode = %o, want 600", info.Mode().Perm())
	}
}

func TestKeysExportRequiresExplicitStdoutFlag(t *testing.T) {
	const (
		testKeyName = "stdout-guard"
	)

	t.Setenv("HOME", t.TempDir())

	ks, err := keystore.Load()
	if err != nil {
		t.Fatalf("keystore.Load() error = %v", err)
	}
	keyCopy := make([]byte, len(ewoqPrivateKey))
	copy(keyCopy, ewoqPrivateKey)
	if err := ks.ImportKey(testKeyName, keyCopy, nil); err != nil {
		t.Fatalf("ImportKey() error = %v", err)
	}

	origKeyName := keyName
	origKeyFormat := keyFormat
	origKeyExportUnsafe := keyExportUnsafe
	origKeyExportFile := keyExportFile
	defer func() {
		keyName = origKeyName
		keyFormat = origKeyFormat
		keyExportUnsafe = origKeyExportUnsafe
		keyExportFile = origKeyExportFile
	}()
	keyName = testKeyName
	keyFormat = "cb58"
	keyExportUnsafe = false
	keyExportFile = ""

	err = keysExportCmd.RunE(keysExportCmd, nil)
	if err == nil {
		t.Fatal("keys export expected guard error without --unsafe-stdout or --output-file")
	}
	if !strings.Contains(err.Error(), "unsafe-stdout") {
		t.Fatalf("unexpected error: %v", err)
	}
}
