package xoxno

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

const maxResponseBytes = 1 << 20

// HTTPError preserves upstream status and transport causes for caller metrics.
type HTTPError struct {
	Code int
	Err  error
}

func (e *HTTPError) Error() string { return e.Err.Error() }
func (e *HTTPError) Unwrap() error { return e.Err }

func getJSON(ctx context.Context, c *http.Client, u string, out any) (int, error) {
	r, e := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if e != nil {
		return 0, e
	}
	r.Header.Set("Accept", "application/json")
	res, e := c.Do(r)
	if e != nil {
		return 0, &HTTPError{Err: e}
	}
	defer res.Body.Close()
	body := io.LimitReader(res.Body, maxResponseBytes)
	if res.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, body)
		return res.StatusCode, nil
	}
	return res.StatusCode, json.NewDecoder(body).Decode(out)
}
func statusError(what string, status int) error {
	return &HTTPError{Code: status, Err: fmt.Errorf("%s status %d", what, status)}
}
