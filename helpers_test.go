package xoxno

import (
	"fmt"
	"github.com/stellar/go-stellar-sdk/strkey"
	"github.com/stellar/go-stellar-sdk/xdr"
	"strings"
)

func ScAddressFromAccountString(address string) (*xdr.ScAddress, error) {
	raw, err := strkey.Decode(strkey.VersionByteAccountID, address)
	if err != nil {
		return nil, fmt.Errorf("invalid account ID: %w", err)
	}
	if len(raw) != 32 {
		return nil, fmt.Errorf("decoded account ID should be 32 bytes, got %d", len(raw))
	}

	var ed25519 xdr.Uint256
	copy(ed25519[:], raw)

	return &xdr.ScAddress{
		Type: xdr.ScAddressTypeScAddressTypeAccount,
		AccountId: &xdr.AccountId{
			Type:    xdr.PublicKeyTypePublicKeyTypeEd25519,
			Ed25519: &ed25519,
		},
	}, nil
}

func ScAddressFromContractString(address string) (*xdr.ScAddress, error) {
	raw, err := strkey.Decode(strkey.VersionByteContract, address)
	if err != nil {
		return nil, fmt.Errorf("invalid contract ID: %w", err)
	}
	if len(raw) != 32 {
		return nil, fmt.Errorf("decoded contract ID should be 32 bytes, got %d", len(raw))
	}

	var contractId xdr.ContractId
	copy(contractId[:], raw)

	return &xdr.ScAddress{
		Type:       xdr.ScAddressTypeScAddressTypeContract,
		ContractId: &contractId,
	}, nil
}

func address(addr string) (*xdr.ScAddress, error) {
	switch {
	case strings.HasPrefix(addr, "G"):
		return ScAddressFromAccountString(addr)
	case strings.HasPrefix(addr, "C"):
		return ScAddressFromContractString(addr)
	default:
		return nil, fmt.Errorf("unsupported address prefix")
	}
}

const testIssuer = "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVN"
const testSender = "GBRPYHIL2CI3FNQ4BXLFMNDLFJUNPU2HY3ZMFSHONUCEOASW7QC7OX2H"

func testContract(b byte) string {
	raw := make([]byte, 32)
	raw[0] = b
	return strkey.MustEncode(strkey.VersionByteContract, raw)
}
func mustAddr(a *xdr.ScAddress, e error) xdr.ScAddress {
	if e != nil {
		panic(e)
	}
	return *a
}
