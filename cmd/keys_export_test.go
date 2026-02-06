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
	defer func() {
		keyName = origKeyName
		keyFormat = origKeyFormat
	}()
	keyName = testKeyName
	keyFormat = "cb58"

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
