package wallet

import (
	"testing"
)

// Known addresses of the ewoq test key.
const (
	ewoqShortID  = "6Y3kysjF9jnHnYkdS9yGAuoHyae2eNmeV"
	ewoqPMainnet = "P-avax18jma8ppw3nhx5r4ap8clazz0dps7rv5ukulre5"
	ewoqPFuji    = "P-fuji18jma8ppw3nhx5r4ap8clazz0dps7rv5u6wmu4t"
)

func TestFormatPChainAddresses(t *testing.T) {
	key, err := ToPrivateKey(testKeyBytes)
	if err != nil {
		t.Fatalf("ToPrivateKey() error = %v", err)
	}

	mainnet, testnet := FormatPChainAddresses(key.Address())
	if mainnet != ewoqPMainnet {
		t.Errorf("FormatPChainAddresses() mainnet = %s, want %s", mainnet, ewoqPMainnet)
	}
	if testnet != ewoqPFuji {
		t.Errorf("FormatPChainAddresses() testnet = %s, want %s", testnet, ewoqPFuji)
	}
}

func TestParsePChainAddressID(t *testing.T) {
	key, err := ToPrivateKey(testKeyBytes)
	if err != nil {
		t.Fatalf("ToPrivateKey() error = %v", err)
	}
	want := key.Address()

	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{name: "cb58 short ID", input: ewoqShortID},
		{name: "mainnet bech32", input: ewoqPMainnet},
		{name: "fuji bech32", input: ewoqPFuji},
		{name: "invalid", input: "not-an-address", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParsePChainAddressID(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ParsePChainAddressID(%q) error = nil, want error", tt.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParsePChainAddressID(%q) error = %v", tt.input, err)
			}
			if got != want {
				t.Errorf("ParsePChainAddressID(%q) = %s, want %s", tt.input, got, want)
			}
		})
	}
}
