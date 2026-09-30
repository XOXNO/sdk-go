package xoxno

import (
	"encoding/json"
	"math/big"
	"os"
	"testing"

	"github.com/stellar/go-stellar-sdk/xdr"
	"github.com/stretchr/testify/require"
)

func TestXoxnoCapturedEnvelopes(t *testing.T) {
	for _, fixture := range []struct{ file, input, output, minimum string }{
		{"xoxno.json", "100000000", "CCW67TSZV3SSS2HXMBQ5JFGCKJNXKZM7UQUWUZPUTHXSTZLEO7SJMI75", "22719551"},
		{"xoxno-soroban.json", "1000000000", "CBI7UCH5KGSVQRO5H4SUCZUTZABCITZLRHQQZTWL2TK4RZ72TAR6IHRV", "22225325112351813029"},
	} {
		t.Run(fixture.file, func(t *testing.T) {
			var data struct {
				EnvelopeXDR string `json:"envelopeXdr"`
			}
			readEnvelopeJSON(t, fixture.file, &data)
			for _, mutation := range []string{"valid", "negative resource fee", "excess resource fee", "duplicate field", "extra field", "unordered fields", "shared spend", "excess shared spend"} {
				t.Run(mutation, func(t *testing.T) {
					env, err := decodeEnvelope(data.EnvelopeXDR)
					require.NoError(t, err)
					tx := &env.V1.Tx
					op := tx.Operations[0].Body.InvokeHostFunctionOp
					call := op.HostFunction.InvokeContract
					want := envelopeExpectation{
						Sender: "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVN", Router: func() string { a, _ := call.ContractAddress.String(); return a }(),
						SrcToken: "CAS3J7GYLGXMF6TDJBBYYSE3HQ6BBSMLNUQ34T6TZMYMW2EVH34XOWMA", DstToken: fixture.output,
						SrcAtoms: atoms(fixture.input), MinOut: atoms(fixture.minimum),
					}
					switch mutation {
					case "negative resource fee":
						tx.Ext.SorobanData.ResourceFee = -1
					case "excess resource fee":
						tx.Ext.SorobanData.ResourceFee = xdr.Int64(tx.Fee) + 1
					case "duplicate field", "extra field", "unordered fields":
						var payload xdr.ScVal
						require.NoError(t, xdr.SafeUnmarshal(*call.Args[2].Bytes, &payload))
						mutateStruct(&payload, mutation)
						encoded, err := payload.MarshalBinary()
						require.NoError(t, err)
						b := xdr.ScBytes(encoded)
						call.Args[2].Bytes = &b
					case "shared spend", "excess shared spend":
						other, err := decodeEnvelope(data.EnvelopeXDR)
						require.NoError(t, err)
						op.Auth = append(op.Auth, other.V1.Tx.Operations[0].Body.InvokeHostFunctionOp.Auth[0])
						for i, fraction := range []int64{60, 40} {
							amount := new(big.Int).Quo(new(big.Int).Mul(want.SrcAtoms, big.NewInt(fraction)), big.NewInt(100))
							if mutation == "excess shared spend" && i == 1 {
								amount.Add(amount, big.NewInt(1))
							}
							op.Auth[i].RootInvocation.SubInvocations[0].Function.ContractFn.Args[2] = bigI128(amount)
						}
					}
					op.Auth[0].RootInvocation.Function.ContractFn = call
					encoded, err := xdr.MarshalBase64(env)
					require.NoError(t, err)
					out, _, err := prepareEnvelope(encoded, want, 777, 1700000000)
					if mutation != "valid" && mutation != "shared spend" {
						require.Error(t, err)
						return
					}
					require.NoError(t, err)
					stamped, err := decodeEnvelope(out)
					require.NoError(t, err)
					tx.SeqNum, tx.Cond = stamped.V1.Tx.SeqNum, stamped.V1.Tx.Cond
					require.Equal(t, env, stamped, "only sequence and expiry may change")
				})
			}
		})
	}
}

func readEnvelopeJSON(t *testing.T, file string, out any) {
	t.Helper()
	data, err := os.ReadFile("testdata/envelopes/" + file)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(data, out))
}

func mutateStruct(v *xdr.ScVal, mutation string) {
	entries := **v.Map
	switch mutation {
	case "duplicate field":
		entries = append(entries, entries[0])
	case "extra field":
		symbol := xdr.ScSymbol("zzz")
		entries = append(entries, xdr.ScMapEntry{Key: xdr.ScVal{Type: xdr.ScValTypeScvSymbol, Sym: &symbol}, Val: i128Val(0)})
	case "unordered fields":
		entries[0], entries[1] = entries[1], entries[0]
	}
	*v.Map = &entries
}

func setStructField(v *xdr.ScVal, name string, value xdr.ScVal) {
	for i, entry := range **v.Map {
		if string(*entry.Key.Sym) == name {
			(**v.Map)[i].Val = value
			return
		}
	}
	panic("missing fixture field " + name)
}

func scVector(values ...xdr.ScVal) xdr.ScVal {
	v := xdr.ScVec(values)
	p := &v
	return xdr.ScVal{Type: xdr.ScValTypeScvVec, Vec: &p}
}

func bigI128(n *big.Int) xdr.ScVal {
	hi := new(big.Int).Rsh(new(big.Int).Set(n), 64).Int64()
	lo := new(big.Int).And(n, new(big.Int).SetUint64(^uint64(0))).Uint64()
	return xdr.ScVal{Type: xdr.ScValTypeScvI128, I128: &xdr.Int128Parts{Hi: xdr.Int64(hi), Lo: xdr.Uint64(lo)}}
}

func bigU128(n *big.Int) xdr.ScVal {
	hi := new(big.Int).Rsh(new(big.Int).Set(n), 64).Uint64()
	return xdr.ScVal{Type: xdr.ScValTypeScvU128, U128: &xdr.UInt128Parts{Hi: xdr.Uint64(hi), Lo: xdr.Uint64(n.Uint64())}}
}
