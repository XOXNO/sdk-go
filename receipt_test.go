package xoxno

import (
	"crypto/sha256"
	"github.com/stellar/go-stellar-sdk/xdr"
	"github.com/stretchr/testify/require"
	"math/big"
	"testing"
)

func receiptFixture(t *testing.T) (xdr.TransactionEnvelope, xdr.TransactionResult, xdr.ContractEvent) {
	o, _ := goodOpts()
	encoded := buildEnvelope(t, o)
	var env xdr.TransactionEnvelope
	require.NoError(t, xdr.SafeUnmarshalBase64(encoded, &env))
	hash := xdr.Hash{}
	ops := []xdr.OperationResult{{Code: xdr.OperationResultCodeOpInner, Tr: &xdr.OperationResultTr{Type: xdr.OperationTypeInvokeHostFunction, InvokeHostFunctionResult: &xdr.InvokeHostFunctionResult{Code: xdr.InvokeHostFunctionResultCodeInvokeHostFunctionSuccess, Success: &hash}}}}
	result := xdr.TransactionResult{Result: xdr.TransactionResultResult{Code: xdr.TransactionResultCodeTxSuccess, Results: &ops}}
	token := mustAddr(address(testContract(4))).ContractId
	sym := xdr.ScSymbol("transfer")
	ev := xdr.ContractEvent{Type: xdr.ContractEventTypeContract, ContractId: token, Body: xdr.ContractEventBody{V: 0, V0: &xdr.ContractEventV0{Topics: []xdr.ScVal{{Type: xdr.ScValTypeScvSymbol, Sym: &sym}, addrVal(mustAddr(address(testContract(2)))), addrVal(mustAddr(address(testSender)))}, Data: i128Val(1234)}}}
	return env, result, ev
}
func TestReceiptConfirmedEvents(t *testing.T) {
	for _, version := range []int{3, 4} {
		for _, variant := range []string{"valid", "self transfer", "wrong recipient", "wrong token", "failed", "multiop", "missing events", "fee bump", "malformed amount", "unrelated metadata"} {
			t.Run(string(rune('0'+version))+"/"+variant, func(t *testing.T) {
				env, res, event := receiptFixture(t)
				switch variant {
				case "self transfer":
					event.Body.V0.Topics[1] = addrVal(mustAddr(address(testSender)))
				case "wrong recipient":
					event.Body.V0.Topics[2] = addrVal(mustAddr(address(testIssuer)))
				case "wrong token":
					event.ContractId = mustAddr(address(testContract(9))).ContractId
				case "failed":
					res.Result.Code = xdr.TransactionResultCodeTxFailed
				case "malformed amount":
					event.Body.V0.Data = xdr.ScVal{Type: xdr.ScValTypeScvVoid}
				case "multiop":
					env.V1.Tx.Operations = append(env.V1.Tx.Operations, env.V1.Tx.Operations[0])
					ops := append(*res.Result.Results, (*res.Result.Results)[0])
					res.Result.Results = &ops
				case "fee bump":
					inner := env.V1
					env = xdr.TransactionEnvelope{Type: xdr.EnvelopeTypeEnvelopeTypeTxFeeBump, FeeBump: &xdr.FeeBumpTransactionEnvelope{Tx: xdr.FeeBumpTransaction{FeeSource: inner.Tx.SourceAccount, InnerTx: xdr.FeeBumpTransactionInnerTx{Type: xdr.EnvelopeTypeEnvelopeTypeTx, V1: inner}}}}
					res.Result = xdr.TransactionResultResult{Code: xdr.TransactionResultCodeTxFeeBumpInnerSuccess, InnerResultPair: &xdr.InnerTransactionResultPair{Result: xdr.InnerTransactionResult{Result: xdr.InnerTransactionResultResult{Code: xdr.TransactionResultCodeTxSuccess, Results: res.Result.Results}}}}
				}
				events := []xdr.ContractEvent{event}
				if variant == "missing events" {
					events = nil
				}
				meta := xdr.TransactionMeta{V: 3, V3: &xdr.TransactionMetaV3{Operations: make([]xdr.OperationMeta, len(*func() *[]xdr.OperationResult {
					if res.Result.Results != nil {
						return res.Result.Results
					}
					return res.Result.InnerResultPair.Result.Result.Results
				}())), SorobanMeta: &xdr.SorobanTransactionMeta{ReturnValue: xdr.ScVal{Type: xdr.ScValTypeScvVoid}, Events: events}}}
				if version == 4 {
					ops := []xdr.OperationMetaV2{{Events: events}}
					if variant == "multiop" {
						ops = append(ops, xdr.OperationMetaV2{Events: events})
					}
					meta = xdr.TransactionMeta{V: 4, V4: &xdr.TransactionMetaV4{Operations: ops, SorobanMeta: &xdr.SorobanTransactionMetaV2{ReturnValue: &xdr.ScVal{Type: xdr.ScValTypeScvVoid}}}}
				}
				pre, e := (xdr.InvokeHostFunctionSuccessPreImage{ReturnValue: xdr.ScVal{Type: xdr.ScValTypeScvVoid}, Events: events}).MarshalBinary()
				require.NoError(t, e)
				h := xdr.Hash(sha256.Sum256(pre))
				var results *[]xdr.OperationResult
				if res.Result.Results != nil {
					results = res.Result.Results
				} else {
					results = res.Result.InnerResultPair.Result.Result.Results
				}
				(*results)[0].Tr.InvokeHostFunctionResult.Success = &h
				if variant == "unrelated metadata" {
					if version == 3 {
						meta.V3.SorobanMeta.Events[0].Body.V0.Data = i128Val(9999)
					} else {
						meta.V4.Operations[0].Events[0].Body.V0.Data = i128Val(9999)
					}
				}

				enc, e := xdr.MarshalBase64(env)
				require.NoError(t, e)
				r, e := xdr.MarshalBase64(res)
				require.NoError(t, e)
				m, e := xdr.MarshalBase64(meta)
				require.NoError(t, e)
				got, e := ReadReceipt(enc, r, m, testContract(2), testSender, 0)
				if variant == "valid" || variant == "fee bump" || (version == 4 && variant == "multiop") {
					require.NoError(t, e)
					require.NotNil(t, got)
					require.Equal(t, big.NewInt(1234), got.AmountOut)
				} else {
					require.Nil(t, got)
				}
			})
		}
	}
}
