package xoxno

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

const maxResponseBytes = 1 << 20

func getJSON(ctx context.Context, c *http.Client, u string, out any) (int, error) {
	r, e := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if e != nil {
		return 0, e
	}
	r.Header.Set("Accept", "application/json")
	res, e := c.Do(r)
	if e != nil {
		return 0, e
	}
	defer res.Body.Close()
	body := io.LimitReader(res.Body, maxResponseBytes)
	if res.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, body)
		return res.StatusCode, nil
	}
	return res.StatusCode, json.NewDecoder(body).Decode(out)
}
func statusError(what string, status int) error { return fmt.Errorf("%s status %d", what, status) }
