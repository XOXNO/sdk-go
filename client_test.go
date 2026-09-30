package xoxno

import (
	"context"
	"encoding/json"
	"github.com/stretchr/testify/require"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestClientQuotesAndRejectsMismatches(t *testing.T) {
	for _, mutation := range []string{"valid", "wrong input", "wrong pair", "wrong mode", "wrong decimals", "slippage", "missing simulation", "upstream no route", "quote only"} {
		t.Run(mutation, func(t *testing.T) {
			o, _ := goodOpts()
			tx := Transaction{EnvelopeXDR: buildEnvelope(t, o), RouterContract: testContract(2), NetworkPassphrase: "network", Simulated: true}
			q := map[string]any{"mode": "forward", "from": testContract(3), "to": testContract(4), "amountIn": "1000000000", "amountOut": "1000", "amountOutMin": "990", "decimalsOut": 7, "transaction": tx}
			switch mutation {
			case "wrong input":
				q["amountIn"] = "1"
			case "wrong pair":
				q["to"] = testContract(9)
			case "wrong mode":
				q["mode"] = "reverse"
			case "wrong decimals":
				q["decimalsOut"] = 18
			case "slippage":
				q["amountOutMin"] = "1"
			case "missing simulation":
				tx.Simulated = false
				q["transaction"] = tx
			}
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, "1000000000", r.URL.Query().Get("amount_in"))
				require.Equal(t, "false", r.URL.Query().Get("include_paths"))
				if mutation == "upstream no route" {
					w.WriteHeader(422)
					return
				}
				require.NoError(t, json.NewEncoder(w).Encode(q))
			}))
			defer srv.Close()
			c := NewClient(Config{QuoteURL: srv.URL, Router: testContract(2), NetworkPassphrase: "network", Now: func() time.Time { return time.Unix(1700000000, 0) }})
			r := QuoteRequest{SourceToken: testContract(3), DestToken: testContract(4), SourceAmount: "100", SourceDecimals: 7, DestDecimals: 7, Sender: testSender, SlippagePercent: 1, AccountSequence: 41, TimeoutSeconds: 180, PrepareTransaction: mutation != "quote only"}
			got, e := c.Quote(context.Background(), r)
			if mutation == "valid" {
				require.NoError(t, e)
				require.Equal(t, "1000", got.DestAmount.String())
				require.Equal(t, "500000", got.Transaction.FeeStroops)
				require.EqualValues(t, 1700000180, got.Transaction.ExpiresAt)
			} else if mutation == "quote only" {
				require.NoError(t, e)
				require.Nil(t, got.Transaction)
			} else {
				require.Error(t, e)
				require.Nil(t, got)
			}
		})
	}
}
func TestClientReverseSizingAndRequestBounds(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Empty(t, r.URL.Query().Get("sender"))
		require.Empty(t, r.URL.Query().Get("simulate"))
		require.Equal(t, "10000000", r.URL.Query().Get("amount_out"))
		json.NewEncoder(w).Encode(map[string]any{"mode": "reverse", "from": testContract(3), "to": testContract(4), "amountIn": "20000000", "amountOut": "10000000", "decimalsOut": 7})
	}))
	defer srv.Close()
	c := NewClient(Config{QuoteURL: srv.URL, Router: testContract(2), NetworkPassphrase: "network"})
	req := QuoteRequest{SourceToken: testContract(3), DestToken: testContract(4), DestAmount: "1", DestDecimals: 7}
	got, e := c.QuoteInput(context.Background(), req)
	require.NoError(t, e)
	require.Equal(t, "20000000", got.String())
	require.Error(t, validateRequest(QuoteRequest{PrepareTransaction: true, SlippagePercent: 1, TimeoutSeconds: 0}, true))
	require.Error(t, validateRequest(QuoteRequest{SlippagePercent: math.NaN()}, true))
	_, e = ParseDecimalAmount(strings.Repeat("9", 39), 38)
	require.Error(t, e)
	for _, s := range []string{"+1", "-1", "0", "1e2", strings.Repeat("9", 40)} {
		require.Nil(t, atoms(s))
	}
}
