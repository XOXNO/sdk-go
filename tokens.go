package xoxno

import (
	"context"
	"net/http"
	"strings"
)

type ListedToken struct {
	Identifier string  `json:"identifier"`
	Ticker     string  `json:"ticker"`
	Name       string  `json:"name"`
	Decimals   int     `json:"decimals"`
	PNGURL     string  `json:"pngUrl"`
	USDPrice   float64 `json:"usdPrice"`
	SwapListed bool    `json:"swapListed"`
	LPToken    bool    `json:"lpToken"`
}

type Token struct {
	ID       string `json:"id"`
	Decimals int    `json:"decimals"`
}

type Price struct {
	USD      float64 `json:"usd"`
	DepthUSD float64 `json:"depth_usd"`
}

func (c *Client) ListedTokens(ctx context.Context) (out []ListedToken, err error) {
	err = c.get(ctx, c.cfg.TokenListURL, &out)
	return
}
func (c *Client) Tokens(ctx context.Context) (out []Token, err error) {
	err = c.get(ctx, strings.TrimRight(c.cfg.QuoteURL, "/")+"/api/v1/tokens", &out)
	return
}
func (c *Client) Prices(ctx context.Context) (out map[string]Price, err error) {
	err = c.get(ctx, strings.TrimRight(c.cfg.QuoteURL, "/")+"/api/v1/prices", &out)
	return
}
func (c *Client) get(ctx context.Context, u string, out any) error {
	status, e := getJSON(ctx, c.httpClient, u, out)
	if e != nil {
		return e
	}
	if status != http.StatusOK {
		return statusError("xoxno tokens", status)
	}
	return nil
}
