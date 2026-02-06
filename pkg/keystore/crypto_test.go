package keystore

import (
	"bytes"
	"encoding/base64"
	"testing"
)

func TestGenerateSalt(t *testing.T) {
	salt1, err := GenerateSalt()
	if err != nil {
		t.Fatalf("GenerateSalt() error = %v", err)
	}
	if len(salt1) != saltSize {
		t.Errorf("GenerateSalt() length = %d, want %d", len(salt1), saltSize)
	}

	// Ensure salts are random (not the same)
	salt2, err := GenerateSalt()
	if err != nil {
		t.Fatalf("GenerateSalt() second call error = %v", err)
	}
	if bytes.Equal(salt1, salt2) {
		t.Error("GenerateSalt() returned identical salts")
	}
}

func TestGenerateNonce(t *testing.T) {
	nonce1, err := GenerateNonce()
	if err != nil {
		t.Fatalf("GenerateNonce() error = %v", err)
	}
	if len(nonce1) != nonceSize {
		t.Errorf("GenerateNonce() length = %d, want %d", len(nonce1), nonceSize)
	}

	// Ensure nonces are random
	nonce2, err := GenerateNonce()
	if err != nil {
		t.Fatalf("GenerateNonce() second call error = %v", err)
	}
	if bytes.Equal(nonce1, nonce2) {
		t.Error("GenerateNonce() returned identical nonces")
	}
}

func TestDeriveKey(t *testing.T) {
	password := []byte("testpassword123")
	salt := make([]byte, saltSize)

	key := DeriveKey(password, salt)
	if len(key) != argon2KeyLen {
		t.Errorf("DeriveKey() length = %d, want %d", len(key), argon2KeyLen)
	}

	// Same password and salt should produce same key
	key2 := DeriveKey(password, salt)
	if !bytes.Equal(key, key2) {
		t.Error("DeriveKey() with same inputs produced different keys")
	}

	// Different password should produce different key
	key3 := DeriveKey([]byte("differentpassword"), salt)
	if bytes.Equal(key, key3) {
		t.Error("DeriveKey() with different password produced same key")
	}

	// Different salt should produce different key
	differentSalt := make([]byte, saltSize)
	differentSalt[0] = 1
	key4 := DeriveKey(password, differentSalt)
	if bytes.Equal(key, key4) {
		t.Error("DeriveKey() with different salt produced same key")
	}
}

func TestEncryptDecrypt(t *testing.T) {
	tests := []struct {
		name      string
		plaintext []byte
		password  []byte
	}{
		{
			name:      "simple text",
			plaintext: []byte("hello world"),
			password:  []byte("password123"),
		},
		{
			name:      "private key bytes",
			plaintext: make([]byte, 32), // secp256k1 key length
			password:  []byte("strongpassword!@#$"),
		},
		{
			name:      "empty plaintext",
			plaintext: []byte{},
			password:  []byte("password"),
		},
		{
			name:      "long password",
			plaintext: []byte("test data"),
			password:  []byte("this is a very long password that exceeds typical length requirements"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Encrypt
			salt, nonce, ciphertext, err := Encrypt(tt.plaintext, tt.password)
			if err != nil {
				t.Fatalf("Encrypt() error = %v", err)
			}

			if salt == "" || nonce == "" || ciphertext == "" {
				t.Error("Encrypt() returned empty values")
			}

			// Decrypt
			decrypted, err := Decrypt(salt, nonce, ciphertext, tt.password)
			if err != nil {
				t.Fatalf("Decrypt() error = %v", err)
			}

			if !bytes.Equal(decrypted, tt.plaintext) {
				t.Errorf("Decrypt() = %v, want %v", decrypted, tt.plaintext)
			}
		})
	}
}

func TestDecryptWrongPassword(t *testing.T) {
	plaintext := []byte("secret data")
	password := []byte("correctpassword")
	wrongPassword := []byte("wrongpassword")

	salt, nonce, ciphertext, err := Encrypt(plaintext, password)
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}

	_, err = Decrypt(salt, nonce, ciphertext, wrongPassword)
	if err == nil {
		t.Error("Decrypt() with wrong password should fail")
	}
}

func TestDecryptInvalidBase64(t *testing.T) {
	password := []byte("password")

	// Invalid salt
	_, err := Decrypt("not-valid-base64!!!", "dGVzdA==", "dGVzdA==", password)
	if err == nil {
		t.Error("Decrypt() with invalid salt should fail")
	}

	// Invalid nonce
	_, err = Decrypt("dGVzdA==", "not-valid-base64!!!", "dGVzdA==", password)
	if err == nil {
		t.Error("Decrypt() with invalid nonce should fail")
	}

	// Invalid ciphertext
	_, err = Decrypt("dGVzdA==", "dGVzdA==", "not-valid-base64!!!", password)
	if err == nil {
		t.Error("Decrypt() with invalid ciphertext should fail")
	}
}

func TestDecryptInvalidNonceLength(t *testing.T) {
	password := []byte("password")
	salt := base64.StdEncoding.EncodeToString(make([]byte, saltSize))
	nonce := base64.StdEncoding.EncodeToString(make([]byte, 1))
	ciphertext := base64.StdEncoding.EncodeToString(make([]byte, 16))

	_, err := Decrypt(salt, nonce, ciphertext, password)
	if err == nil {
		t.Fatal("Decrypt() with invalid nonce length should fail")
	}
}

func TestClearBytes(t *testing.T) {
	data := []byte{1, 2, 3, 4, 5}
	clearBytes(data)

	for i, b := range data {
		if b != 0 {
			t.Errorf("clearBytes() did not zero byte at index %d: got %d", i, b)
		}
	}
}

func TestEncryptProducesDifferentCiphertext(t *testing.T) {
	plaintext := []byte("same plaintext")
	password := []byte("same password")

	salt1, nonce1, ciphertext1, err := Encrypt(plaintext, password)
	if err != nil {
		t.Fatalf("First Encrypt() error = %v", err)
	}

	salt2, nonce2, ciphertext2, err := Encrypt(plaintext, password)
	if err != nil {
		t.Fatalf("Second Encrypt() error = %v", err)
	}

	// Same plaintext and password should produce different ciphertext
	// due to random salt and nonce
	if salt1 == salt2 {
		t.Error("Encrypt() produced identical salts")
	}
	if nonce1 == nonce2 {
		t.Error("Encrypt() produced identical nonces")
	}
	if ciphertext1 == ciphertext2 {
		t.Error("Encrypt() produced identical ciphertext (salt/nonce should make it different)")
	}
}
