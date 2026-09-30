package xoxno

import (
	"crypto/sha256"
	"errors"
	"github.com/stellar/go-stellar-sdk/strkey"
	"github.com/stellar/go-stellar-sdk/xdr"
	"math/big"
)

// SwapReceipt describes one confirmed XOXNO operation. All amounts are atoms.
type SwapReceipt struct {
	TokenIn, TokenOut   string
	AmountIn, AmountOut *big.Int
}

// ReadReceipt reads confirmed output, never quoted output or a route minimum.
// Nil means unavailable or not an attributable XOXNO receipt. Signed envelopes
// are accepted here; this reader is not an unsigned signing-policy validator.
func ReadReceipt(envelopeXDR, resultXDR, metaXDR, router, viewer string, operationIndex int) (*SwapReceipt, error) {
	var env xdr.TransactionEnvelope
	if e := xdr.SafeUnmarshalBase64(envelopeXDR, &env); e != nil {
		return nil, e
	}
	tx := env.V1
	if env.Type == xdr.EnvelopeTypeEnvelopeTypeTxFeeBump && env.FeeBump != nil {
		tx = env.FeeBump.Tx.InnerTx.V1
	}
	if tx == nil || operationIndex < 0 || operationIndex >= len(tx.Tx.Operations) {
		return nil, nil
	}
	var result xdr.TransactionResult
	if e := xdr.SafeUnmarshalBase64(resultXDR, &result); e != nil {
		return nil, e
	}
	results := result.Result.Results
	switch result.Result.Code {
	case xdr.TransactionResultCodeTxSuccess:
	case xdr.TransactionResultCodeTxFeeBumpInnerSuccess:
		pair := result.Result.InnerResultPair
		if pair == nil || pair.Result.Result.Code != xdr.TransactionResultCodeTxSuccess {
			return nil, nil
		}
		results = pair.Result.Result.Results
	default:
		return nil, nil
	}
	if results == nil || len(*results) != len(tx.Tx.Operations) {
		return nil, nil
	}
	opResult := (*results)[operationIndex]
	if opResult.Code != xdr.OperationResultCodeOpInner || opResult.Tr == nil || opResult.Tr.Type != xdr.OperationTypeInvokeHostFunction || opResult.Tr.InvokeHostFunctionResult == nil || opResult.Tr.InvokeHostFunctionResult.Code != xdr.InvokeHostFunctionResultCodeInvokeHostFunctionSuccess {
		return nil, nil
	}
	op := tx.Tx.Operations[operationIndex]
	if op.Body.Type != xdr.OperationTypeInvokeHostFunction || op.Body.InvokeHostFunctionOp == nil {
		return nil, nil
	}
	host := op.Body.InvokeHostFunctionOp.HostFunction
	if host.Type != xdr.HostFunctionTypeHostFunctionTypeInvokeContract || host.InvokeContract == nil {
		return nil, nil
	}
	call := host.InvokeContract
	id, e := call.ContractAddress.String()
	if e != nil || id != router || string(call.FunctionName) != routerFunction || len(call.Args) != 3 {
		return nil, nil
	}
	sender, e := scValAddress(call.Args[0])
	if e != nil || sender != viewer {
		return nil, nil
	}
	input, e := scValI128(call.Args[1])
	if e != nil || input.Sign() <= 0 || call.Args[2].Bytes == nil || call.Args[2].Type != xdr.ScValTypeScvBytes {
		return nil, nil
	}
	fields, e := decodeRoutePayload(*call.Args[2].Bytes)
	if e != nil {
		return nil, e
	}
	header, e := routeHeader(fields["ops"])
	if e != nil {
		return nil, e
	}
	in, e := vecAt(fields["assets"], int(header[routeHdrTokenIn]), scValAddress)
	if e != nil {
		return nil, e
	}
	out, e := vecAt(fields["assets"], int(header[routeHdrTokenOut]), scValAddress)
	if e != nil {
		return nil, e
	}
	if !validContract(in) || !validContract(out) || in == out {
		return nil, nil
	}
	var meta xdr.TransactionMeta
	if e := xdr.SafeUnmarshalBase64(metaXDR, &meta); e != nil {
		return nil, e
	}
	var events []xdr.ContractEvent
	var returnValue xdr.ScVal
	switch meta.V {
	case 3:
		// v3 events are transaction-scoped. Multiple invocations cannot be safely
		// attributed to a selected operation without operation-scoped events.
		if meta.V3 == nil || meta.V3.SorobanMeta == nil || len(tx.Tx.Operations) != 1 || len(meta.V3.Operations) != 1 {
			return nil, nil
		}
		events = meta.V3.SorobanMeta.Events
		returnValue = meta.V3.SorobanMeta.ReturnValue
	case 4:
		if meta.V4 == nil || len(meta.V4.Operations) != len(tx.Tx.Operations) {
			return nil, nil
		}
		if meta.V4.SorobanMeta == nil || meta.V4.SorobanMeta.ReturnValue == nil {
			return nil, nil
		}
		returnValue = *meta.V4.SorobanMeta.ReturnValue
		events = meta.V4.Operations[operationIndex].Events
	default:
		return nil, nil
	}
	preimage, e := (xdr.InvokeHostFunctionSuccessPreImage{ReturnValue: returnValue, Events: events}).MarshalBinary()
	if e != nil {
		return nil, e
	}
	expected := opResult.Tr.InvokeHostFunctionResult.Success
	if expected == nil || xdr.Hash(sha256.Sum256(preimage)) != *expected {
		return nil, nil
	}
	received := new(big.Int)
	for _, event := range events {
		if event.Type != xdr.ContractEventTypeContract || event.ContractId == nil || event.Body.V != 0 || event.Body.V0 == nil {
			continue
		}
		id, e := strkey.Encode(strkey.VersionByteContract, event.ContractId[:])
		if e != nil || id != out {
			continue
		}
		v := event.Body.V0
		topics := v.Topics
		if len(topics) < 3 || topics[0].Sym == nil || topics[0].Type != xdr.ScValTypeScvSymbol || string(*topics[0].Sym) != "transfer" {
			continue
		}
		from, e := scValAddress(topics[1])
		if e != nil || from != router {
			continue
		}
		to, e := scValAddress(topics[2])
		if e != nil || to != viewer {
			continue
		}
		amount := v.Data
		if amount.Type == xdr.ScValTypeScvMap && amount.Map != nil && *amount.Map != nil {
			found := false
			for _, entry := range **amount.Map {
				if entry.Key.Type == xdr.ScValTypeScvSymbol && entry.Key.Sym != nil && string(*entry.Key.Sym) == "amount" {
					if found {
						return nil, errors.New("duplicate transfer amount")
					}
					amount = entry.Val
					found = true
				}
			}
			if !found {
				return nil, nil
			}
		}
		n, e := scValI128(amount)
		if e != nil || n.Sign() <= 0 {
			return nil, nil
		}
		received.Add(received, n)
	}
	if received.Sign() == 0 {
		return nil, nil
	}
	return &SwapReceipt{TokenIn: in, TokenOut: out, AmountIn: input, AmountOut: received}, nil
}
