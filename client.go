package xoxno

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/big"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const xoxnoHTTPTimeout = 15 * time.Second

// Config pins the network and router independently of the quote server.
type Config struct {
	Now                                               func() time.Time
	QuoteURL, Router, NetworkPassphrase, TokenListURL string
	HTTPClient                                        *http.Client
}
type Client struct {
	cfg        Config
	httpClient *http.Client
	now        func() time.Time
}

func NewClient(cfg Config) *Client {
	c := cfg.HTTPClient
	if c == nil {
		c = &http.Client{Timeout: xoxnoHTTPTimeout}
	}
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	return &Client{cfg: cfg, httpClient: c, now: now}
}

type xoxnoPair struct {
	cfg                  Config
	passphrase, src, dst string
	dstClassic           bool
}

func (s *Client) resolve(req QuoteRequest) (xoxnoPair, error) {
	if s.cfg.QuoteURL == "" || s.cfg.Router == "" || s.cfg.NetworkPassphrase == "" {
		return xoxnoPair{}, ErrUnsupported
	}
	if !validContract(req.SourceToken) || !validContract(req.DestToken) || req.SourceToken == req.DestToken {
		return xoxnoPair{}, ErrUnsupported
	}
	return xoxnoPair{s.cfg, s.cfg.NetworkPassphrase, req.SourceToken, req.DestToken, req.DestClassic}, nil
}

type xoxnoQuoteResponse struct {
	Mode         string   `json:"mode"`
	From         string   `json:"from"`
	To           string   `json:"to"`
	AmountIn     string   `json:"amountIn"`
	AmountOut    string   `json:"amountOut"`
	AmountOutMin string   `json:"amountOutMin"`
	DecimalsOut  *int     `json:"decimalsOut"`
	PriceImpact  *float64 `json:"priceImpact"`
	Hops         []struct {
		Dex     string `json:"dex"`
		Kind    string `json:"kind"`
		Address string `json:"address"`
		From    string `json:"from"`
		To      string `json:"to"`
	} `json:"hops"`
	Transaction *Transaction `json:"transaction"`
}

func (s *Client) Quote(ctx context.Context, req QuoteRequest) (_ *Quote, err error) {
	if err := validateRequest(req, true); err != nil {
		return nil, err
	}

	p, err := s.resolve(req)
	if err != nil {
		return nil, err
	}
	srcAtoms, err := ParseDecimalAmount(req.SourceAmount, req.SourceDecimals)
	if err != nil {
		return nil, err
	}

	quoteOnly := !req.PrepareTransaction
	quote, err := s.fetchQuote(ctx, p.cfg.QuoteURL, forwardQuery(req, p, srcAtoms, !quoteOnly))
	if err != nil {
		return nil, err
	}
	cand, err := validateXoxnoQuote(quote, p, srcAtoms, req, !quoteOnly)
	if err != nil {
		return nil, err
	}
	if quoteOnly {
		return cand, nil
	}
	if err := s.attachTransaction(cand, quote.Transaction, req, p, srcAtoms, req.AccountSequence); err != nil {
		return nil, err
	}
	return cand, nil
}

// forwardQuery is the aggregator's forward quote request for a fixed input.
func forwardQuery(req QuoteRequest, p xoxnoPair, srcAtoms *big.Int, simulate bool) url.Values {
	return url.Values{
		"from":          {p.src},
		"to":            {p.dst},
		"amount_in":     {srcAtoms.String()},
		"slippage":      {strconv.FormatFloat(req.SlippagePercent/100, 'f', -1, 64)},
		"sender":        {req.Sender},
		"simulate":      {strconv.FormatBool(simulate)},
		"include_paths": {"false"},
	}
}

// attachTransaction verifies the aggregator's envelope against the request,
// stamps it for signing and sets it on cand together with its fee.
func (s *Client) attachTransaction(cand *Quote, tx *Transaction, req QuoteRequest, p xoxnoPair, srcAtoms *big.Int, sequence int64) error {
	envelope, fee, err := prepareEnvelope(tx.EnvelopeXDR, envelopeExpectation{
		Sender:   req.Sender,
		Router:   p.cfg.Router,
		SrcToken: p.src,
		SrcAtoms: srcAtoms,
		DstToken: p.dst,
		MinOut:   cand.DestAmountMin,
	}, sequence+1, s.now().Unix()+req.TimeoutSeconds)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidQuote, err)
	}
	tx.EnvelopeXDR = envelope
	if err := tx.setMetadata(); err != nil {
		return err
	}
	cand.Transaction = tx
	cand.NetworkFee = big.NewInt(int64(fee))
	return nil
}

func (s *Client) fetchQuote(ctx context.Context, baseURL string, q url.Values) (*xoxnoQuoteResponse, error) {
	var out xoxnoQuoteResponse
	status, err := getJSON(ctx, s.httpClient, baseURL+"/api/v1/quote?"+q.Encode(), &out)
	if err != nil {
		return nil, err
	}
	switch status {
	case http.StatusOK:
		return &out, nil
	case http.StatusNotFound, http.StatusUnprocessableEntity:
		return nil, ErrNoRoute
	case http.StatusBadRequest:
		return nil, ErrUnsupported
	default:
		return nil, statusError("xoxno quote", status)
	}
}

// QuoteInput asks the aggregator what input it needs to deliver the requested
// amount. The reverse quote is only used to size the input: its route floors the
// output at exactly the target, which has no room for price movement, so the
// swap itself is quoted forward at this input with the user's slippage.
func (s *Client) QuoteInput(ctx context.Context, req QuoteRequest) (_ *big.Int, err error) {
	if err := validateRequest(req, false); err != nil {
		return nil, err
	}

	p, err := s.resolve(req)
	if err != nil {
		return nil, err
	}
	destAtoms, err := ParseDecimalAmount(req.DestAmount, req.DestDecimals)
	if err != nil {
		return nil, err
	}

	quote, err := s.fetchQuote(ctx, p.cfg.QuoteURL, url.Values{
		"from":       {p.src},
		"to":         {p.dst},
		"amount_out": {destAtoms.String()},
	})
	if err != nil {
		return nil, err
	}
	if err := checkOutputDecimals(quote, p, req.DestDecimals); err != nil {
		return nil, err
	}
	input, out := atoms(quote.AmountIn), atoms(quote.AmountOut)
	if quote.Mode != "reverse" || quote.From != p.src || quote.To != p.dst ||
		input == nil || out == nil || out.Cmp(destAtoms) < 0 {
		return nil, fmt.Errorf("%w: quote does not answer the requested output", ErrInvalidQuote)
	}
	return input, nil
}

// atoms parses a positive base-10 integer, or returns nil.
func atoms(s string) *big.Int {
	if s != "" && len(s) <= 39 && strings.Trim(s, "0123456789") == "" {
		if v, ok := new(big.Int).SetString(s, 10); ok && v.Sign() > 0 && v.BitLen() <= 127 {
			return v
		}
	}
	return nil
}

// invalidQuote wraps ErrInvalidQuote with the reason.
func invalidQuote(format string, args ...any) error {
	return fmt.Errorf("%w: "+format, append([]any{ErrInvalidQuote}, args...)...)
}

// validateXoxnoQuote checks that the aggregator answered the question that was
// asked. Anything that does not match is dropped, never trusted.
func validateXoxnoQuote(q *xoxnoQuoteResponse, p xoxnoPair, srcAtoms *big.Int, req QuoteRequest, wantTx bool) (*Quote, error) {
	if q.Mode != "forward" || q.From != p.src || q.To != p.dst {
		return nil, invalidQuote("quote is for a different pair or mode")
	}
	if in := atoms(q.AmountIn); in == nil || in.Cmp(srcAtoms) != 0 {
		return nil, invalidQuote("quote input %q differs from requested input", q.AmountIn)
	}

	out, minOut, err := checkSlippage(q, req.SlippagePercent)
	if err != nil {
		return nil, err
	}

	if err := checkOutputDecimals(q, p, req.DestDecimals); err != nil {
		return nil, err
	}

	if wantTx {
		if err := checkSimulatedTransaction(q.Transaction, p); err != nil {
			return nil, err
		}
	}

	return &Quote{
		Source:        "xoxno",
		DestAmount:    out,
		DestAmountMin: minOut,
		DestDecimals:  *q.DecimalsOut,
		Route:         routeHops(q),
		PriceImpact:   q.PriceImpact,
	}, nil
}

// checkOutputDecimals binds both forward and reverse amounts to the requested
// precision. A classic destination always uses 7 decimals.
func checkOutputDecimals(q *xoxnoQuoteResponse, p xoxnoPair, requested int) error {
	if q.DecimalsOut == nil || *q.DecimalsOut < 0 || *q.DecimalsOut > MaxAmountDecimals {
		return invalidQuote("quote output decimals are missing or out of range")
	}
	if p.dstClassic && *q.DecimalsOut != 7 {
		return invalidQuote("classic asset quoted with %d decimals", *q.DecimalsOut)
	}
	if *q.DecimalsOut != requested {
		return invalidQuote("quote output decimals %d differ from requested %d", *q.DecimalsOut, requested)
	}
	return nil
}

// checkSlippage returns the quoted output and minimum output after requiring
// 0 < minimum <= output and a minimum no lower than the requested slippage allows.
func checkSlippage(q *xoxnoQuoteResponse, slippagePercent float64) (out, minOut *big.Int, err error) {
	if out = atoms(q.AmountOut); out == nil {
		return nil, nil, invalidQuote("quote output %q is not positive", q.AmountOut)
	}
	if minOut = atoms(q.AmountOutMin); minOut == nil || minOut.Cmp(out) > 0 {
		return nil, nil, invalidQuote("quote minimum output %q is not in (0, output]", q.AmountOutMin)
	}
	if minOut.Cmp(minAmountOut(out, slippagePercent)) < 0 {
		return nil, nil, invalidQuote("quote minimum output %q is below the requested slippage", q.AmountOutMin)
	}
	return out, minOut, nil
}

// checkSimulatedTransaction requires a simulated transaction for the pinned
// router and the requested network.
func checkSimulatedTransaction(tx *Transaction, p xoxnoPair) error {
	if tx == nil || tx.EnvelopeXDR == "" || !tx.Simulated {
		return invalidQuote("quote carries no simulated transaction")
	}
	if tx.RouterContract != p.cfg.Router || tx.NetworkPassphrase != p.passphrase {
		return invalidQuote("transaction targets an unexpected router or network")
	}
	return nil
}

func routeHops(q *xoxnoQuoteResponse) []RouteHop {
	hops := make([]RouteHop, 0, len(q.Hops))
	for _, h := range q.Hops {
		hops = append(hops, RouteHop{Venue: h.Dex, Kind: h.Kind, Pool: h.Address, From: h.From, To: h.To})
	}
	return hops
}

func validateRequest(req QuoteRequest, forward bool) error {
	if req.DestDecimals < 0 || req.DestDecimals > MaxAmountDecimals || (req.DestClassic && req.DestDecimals != 7) {
		return errors.New("invalid output decimals")
	}
	if !forward {
		return nil
	}
	if math.IsNaN(req.SlippagePercent) || math.IsInf(req.SlippagePercent, 0) || req.SlippagePercent < 0.0001 || req.SlippagePercent > 50 {
		return errors.New("invalid slippage")
	}
	if req.PrepareTransaction && (req.AccountSequence < 0 || req.AccountSequence == math.MaxInt64 || req.TimeoutSeconds <= 0 || req.TimeoutSeconds > 900) {
		return errors.New("invalid sequence or expiry")
	}
	return nil
}
