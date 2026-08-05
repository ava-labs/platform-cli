package wallet

import (
	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/utils/crypto/keychain"
	"github.com/ava-labs/avalanchego/utils/set"
)

// augmentedKeychain wraps a keychain so Addresses() also reports addresses
// the wallet cannot sign for. The tx builder then allocates signature slots
// for those owners, which other signers fill later (partial signing).
type augmentedKeychain struct {
	keychain.Keychain
	extra set.Set[ids.ShortID]
}

// WithAdditionalAddresses returns a keychain that claims addrs in addition
// to kc's own addresses, without being able to sign for them.
func WithAdditionalAddresses(kc keychain.Keychain, addrs []ids.ShortID) keychain.Keychain {
	extra := set.NewSet[ids.ShortID](len(addrs))
	extra.Add(addrs...)
	return &augmentedKeychain{
		Keychain: kc,
		extra:    extra,
	}
}

func (a *augmentedKeychain) Addresses() set.Set[ids.ShortID] {
	union := set.NewSet[ids.ShortID](a.extra.Len() + a.Keychain.Addresses().Len())
	union.Union(a.Keychain.Addresses())
	union.Union(a.extra)
	return union
}
