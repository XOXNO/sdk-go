// Package xoxno quotes and validates XOXNO Stellar swaps. It never signs or submits transactions.
package xoxno

import (
	"errors"
	"github.com/stellar/go-stellar-sdk/strkey"
	"math/big"
	"strconv"
)

var (
	ErrUnsupported  = errors.New("swap pair not supported")
	ErrInvalidQuote = errors.New("swap quote failed validation")
	ErrNoRoute      = errors.New("no swap route")
)

type QuoteRequest struct {
	SourceToken, DestToken, SourceAmount, DestAmount, Sender string
	SourceDecimals, DestDecimals                             int
	DestClassic, PrepareTransaction                          bool
	AccountSequence, TimeoutSeconds                          int64
	SlippagePercent                                          float64
}
type Quote struct {
	Source                    string
	DestAmount, DestAmountMin *big.Int
	DestDecimals              int
	Route                     []RouteHop
	Transaction               *Transaction
	PriceImpact               *float64
	NetworkFee                *big.Int
}
type RouteHop struct{ Venue, Kind, Pool, From, To string }
type Transaction struct {
	EnvelopeXDR        string `json:"envelopeXdr"`
	RouterContract     string `json:"routerContract"`
	NetworkPassphrase  string `json:"networkPassphrase"`
	Simulated          bool   `json:"simulated"`
	FeeStroops         string `json:"feeStroops"`
	ResourceFeeStroops string `json:"resourceFeeStroops"`
	ExpiresAt          int64  `json:"expiresAt"`
}

func (t *Transaction) setMetadata() error {
	e, err := decodeEnvelope(t.EnvelopeXDR)
	if err != nil {
		return err
	}
	tx := e.V1.Tx
	t.FeeStroops = strconv.FormatUint(uint64(tx.Fee), 10)
	t.ResourceFeeStroops = strconv.FormatInt(int64(tx.Ext.SorobanData.ResourceFee), 10)
	t.ExpiresAt = int64(tx.Cond.TimeBounds.MaxTime)
	return nil
}
func validContract(id string) bool {
	b, e := strkey.Decode(strkey.VersionByteContract, id)
	return e == nil && len(b) == 32
}
