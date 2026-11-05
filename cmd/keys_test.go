package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/ava-labs/avalanchego/ids"
)

// Known addresses of the ewoq test key.
const (
	ewoqShortID  = "6Y3kysjF9jnHnYkdS9yGAuoHyae2eNmeV"
	ewoqPMainnet = "P-avax18jma8ppw3nhx5r4ap8clazz0dps7rv5ukulre5"
	ewoqPFuji    = "P-fuji18jma8ppw3nhx5r4ap8clazz0dps7rv5u6wmu4t"
)

func TestPrintKeySummary(t *testing.T) {
	addr, err := ids.ShortFromString(ewoqShortID)
	if err != nil {
		t.Fatalf("ids.ShortFromString() error = %v", err)
	}

	var buf bytes.Buffer
	printKeySummary(&buf, "Key generated successfully!", "mykey", addr, "0x8db97C7cEcE249c2b98bDC0226Cc4C2A57BF52FC", true, true)
	out := buf.String()

	for _, want := range []string{
		"P-Chain (mainnet):  " + ewoqPMainnet + "\n",
		"P-Chain (testnet):  " + ewoqPFuji + "\n",
		"Default:            yes\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("printKeySummary() output missing %q\noutput:\n%s", want, out)
		}
	}
	if strings.Contains(out, " "+ewoqShortID+"\n") {
		t.Errorf("printKeySummary() output contains the raw short ID\noutput:\n%s", out)
	}
}

func TestFormatStoredPChainAddresses(t *testing.T) {
	tests := []struct {
		name        string
		stored      string
		wantMainnet string
		wantTestnet string
	}{
		{name: "cb58 short ID", stored: ewoqShortID, wantMainnet: ewoqPMainnet, wantTestnet: ewoqPFuji},
		{name: "fuji bech32", stored: ewoqPFuji, wantMainnet: ewoqPMainnet, wantTestnet: ewoqPFuji},
		{name: "unparsable", stored: "garbage", wantMainnet: "garbage", wantTestnet: "garbage"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mainnet, testnet := formatStoredPChainAddresses(tt.stored)
			if mainnet != tt.wantMainnet || testnet != tt.wantTestnet {
				t.Errorf("formatStoredPChainAddresses(%q) = (%s, %s), want (%s, %s)",
					tt.stored, mainnet, testnet, tt.wantMainnet, tt.wantTestnet)
			}
		})
	}
}
