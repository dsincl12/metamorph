// Package httpclient provides a shared HTTP client with connection pooling,
// timeouts, and retry logic for idempotent requests.
package httpclient

import (
	"context"
	"io"
	"math/rand"
	"net/http"
	"time"
)

// MaxBodyBytes is the maximum number of bytes read from a response body.
// Callers should wrap resp.Body with io.LimitReader(resp.Body, MaxBodyBytes).
const MaxBodyBytes = 10 * 1024 * 1024 // 10 MB

// DefaultClient is a package-level singleton HTTP client with connection
// pooling and sensible timeouts. Share this client across all callers instead
// of creating a new http.Client per request.
var DefaultClient = &http.Client{
	Timeout: 30 * time.Second,
	Transport: &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
	},
}

// retryableStatus returns true for HTTP status codes that are safe to retry.
func retryableStatus(code int) bool {
	switch code {
	case http.StatusTooManyRequests,
		http.StatusInternalServerError,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout:
		return true
	}
	return false
}

// DoWithRetry executes req using client, retrying up to 3 times on transient
// errors. It is only safe for idempotent methods (GET, HEAD); do not use for
// POST, PUT, PATCH, or DELETE requests.
//
// Retry behaviour:
//   - Retries on network errors and HTTP 429/500/502/503/504.
//   - Exponential backoff: ~500 ms, ~1 s, ~2 s with ±25 % jitter.
//   - Respects ctx: if the context is cancelled before an attempt, the function
//     returns immediately with ctx.Err().
//   - Drains and closes resp.Body between retries so TCP connections are
//     returned to the pool.
//
// A default User-Agent header is set if none is present on req.
func DoWithRetry(ctx context.Context, client *http.Client, req *http.Request) (*http.Response, error) {
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", "go-httpclient/1.0")
	}

	const maxAttempts = 3
	backoff := 500 * time.Millisecond

	var (
		resp *http.Response
		err  error
	)

	for attempt := 0; attempt < maxAttempts; attempt++ {
		// Honour context cancellation before each attempt.
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		resp, err = client.Do(req.WithContext(ctx))

		if err != nil {
			// Network error — retry if we have attempts left.
			if attempt < maxAttempts-1 {
				sleep(ctx, jitter(backoff))
				backoff *= 2
				continue
			}
			return nil, err
		}

		if !retryableStatus(resp.StatusCode) {
			// Success or a non-retryable status (e.g. 200, 404).
			return resp, nil
		}

		// Retryable HTTP status — drain and close the body before retrying so
		// the TCP connection is returned to the pool.
		if attempt < maxAttempts-1 {
			io.Copy(io.Discard, resp.Body) //nolint:errcheck
			resp.Body.Close()
			sleep(ctx, jitter(backoff))
			backoff *= 2
			continue
		}
	}

	// All attempts exhausted; return the last response (caller must close body).
	return resp, err
}

// jitter adds ±25 % random variation to d.
func jitter(d time.Duration) time.Duration {
	// rand.Float64 returns [0.0, 1.0); map to [0.75, 1.25).
	factor := 0.75 + rand.Float64()*0.5
	return time.Duration(float64(d) * factor)
}

// sleep waits for d or until ctx is done, whichever comes first.
func sleep(ctx context.Context, d time.Duration) {
	select {
	case <-time.After(d):
	case <-ctx.Done():
	}
}
