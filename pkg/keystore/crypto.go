package keystore

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"

	"golang.org/x/crypto/argon2"
)

const (
	// Argon2id parameters - these are the OWASP recommended values
	argon2Time    = 3          // Number of iterations
	argon2Memory  = 64 * 1024  // Memory in KiB (64 MiB)
	argon2Threads = 4          // Number of threads
	argon2KeyLen  = 32         // Output key length (256 bits for AES-256)

	// Salt and nonce sizes
	saltSize  = 16 // 128 bits
	nonceSize = 12 // 96 bits for GCM
)

// DeriveKey derives an encryption key from a password using Argon2id.
func DeriveKey(password []byte, salt []byte) []byte {
	return argon2.IDKey(password, salt, argon2Time, argon2Memory, argon2Threads, argon2KeyLen)
}

// GenerateSalt generates a random salt for key derivation.
func GenerateSalt() ([]byte, error) {
	salt := make([]byte, saltSize)
	if _, err := rand.Read(salt); err != nil {
		return nil, fmt.Errorf("failed to generate salt: %w", err)
	}
	return salt, nil
}

// GenerateNonce generates a random nonce for AES-GCM.
func GenerateNonce() ([]byte, error) {
	nonce := make([]byte, nonceSize)
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}
	return nonce, nil
}

// Encrypt encrypts plaintext using AES-256-GCM with the given password.
// Returns salt, nonce, and ciphertext (all base64 encoded).
func Encrypt(plaintext []byte, password []byte) (salt, nonce, ciphertext string, err error) {
	// Generate salt
	saltBytes, err := GenerateSalt()
	if err != nil {
		return "", "", "", err
	}

	// Derive key
	key := DeriveKey(password, saltBytes)

	// Create AES cipher
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to create AES cipher: %w", err)
	}

	// Create GCM mode
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to create GCM: %w", err)
	}

	// Generate nonce
	nonceBytes, err := GenerateNonce()
	if err != nil {
		return "", "", "", err
	}

	// Encrypt
	ciphertextBytes := gcm.Seal(nil, nonceBytes, plaintext, nil)

	// Encode to base64
	salt = base64.StdEncoding.EncodeToString(saltBytes)
	nonce = base64.StdEncoding.EncodeToString(nonceBytes)
	ciphertext = base64.StdEncoding.EncodeToString(ciphertextBytes)

	return salt, nonce, ciphertext, nil
}

// Decrypt decrypts ciphertext using AES-256-GCM with the given password.
// Salt, nonce, and ciphertext should be base64 encoded.
func Decrypt(saltB64, nonceB64, ciphertextB64 string, password []byte) ([]byte, error) {
	// Decode base64
	salt, err := base64.StdEncoding.DecodeString(saltB64)
	if err != nil {
		return nil, fmt.Errorf("failed to decode salt: %w", err)
	}

	nonce, err := base64.StdEncoding.DecodeString(nonceB64)
	if err != nil {
		return nil, fmt.Errorf("failed to decode nonce: %w", err)
	}

	ciphertext, err := base64.StdEncoding.DecodeString(ciphertextB64)
	if err != nil {
		return nil, fmt.Errorf("failed to decode ciphertext: %w", err)
	}

	// Derive key
	key := DeriveKey(password, salt)

	// Create AES cipher
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create AES cipher: %w", err)
	}

	// Create GCM mode
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	// Decrypt
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("decryption failed (wrong password?): %w", err)
	}

	return plaintext, nil
}
