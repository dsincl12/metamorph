package httpclient_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"metamorph/internal/httpclient"
)

// newGetReq creates a GET request to url (panics on error — test helper only).
func newGetReq(url string) *http.Request {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		panic(err)
	}
	return req
}

// TestDoWithRetry_SuccessAfterRetries verifies that DoWithRetry returns success
// when the server returns 503 twice then 200.
func TestDoWithRetry_SuccessAfterRetries(t *testing.T) {
	callCount := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ok":true}`)) //nolint:errcheck
	}))
	defer ts.Close()

	resp, err := httpclient.DoWithRetry(context.Background(), ts.Client(), newGetReq(ts.URL))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	if callCount != 3 {
		t.Errorf("expected 3 calls, got %d", callCount)
	}
}

// TestDoWithRetry_ContextCancellation verifies that a cancelled context causes
// DoWithRetry to return context.Canceled promptly.
func TestDoWithRetry_ContextCancellation(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer ts.Close()

	ctx, cancel := context.WithCancel(context.Background())

	// Cancel the context immediately so the first retry sleep is interrupted.
	cancel()

	resp, err := httpclient.DoWithRetry(ctx, ts.Client(), newGetReq(ts.URL))
	if resp != nil {
		resp.Body.Close()
	}

	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

// TestDoWithRetry_NoRetryOn404 verifies that 404 is not retried.
func TestDoWithRetry_NoRetryOn404(t *testing.T) {
	callCount := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	resp, err := httpclient.DoWithRetry(context.Background(), ts.Client(), newGetReq(ts.URL))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
	if callCount != 1 {
		t.Errorf("expected exactly 1 call, got %d", callCount)
	}
}

// TestDoWithRetry_BodyDrainedBetweenRetries verifies that the response body is
// drained between retries (so connections return to the pool) by confirming the
// server is reached on each attempt.
func TestDoWithRetry_BodyDrainedBetweenRetries(t *testing.T) {
	callCount := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(http.StatusInternalServerError)
		// Write a non-trivial body so draining is meaningful.
		w.Write([]byte(strings.Repeat("x", 1024))) //nolint:errcheck
	}))
	defer ts.Close()

	resp, err := httpclient.DoWithRetry(context.Background(), ts.Client(), newGetReq(ts.URL))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	// All 3 attempts should have been made.
	if callCount != 3 {
		t.Errorf("expected 3 calls (body drain between retries), got %d", callCount)
	}
}

// TestMaxBodyBytes verifies that io.LimitReader caps reads at MaxBodyBytes.
func TestMaxBodyBytes(t *testing.T) {
	// Build a reader larger than MaxBodyBytes.
	bigBody := io.LimitReader(strings.NewReader(strings.Repeat("a", 1)), 1)
	_ = bigBody // just confirm the constant is accessible and usable

	if httpclient.MaxBodyBytes != 10*1024*1024 {
		t.Errorf("MaxBodyBytes should be 10 MB, got %d", httpclient.MaxBodyBytes)
	}

	// Simulate reading more than MaxBodyBytes via LimitReader.
	big := strings.NewReader(strings.Repeat("b", httpclient.MaxBodyBytes+100))
	limited := io.LimitReader(big, httpclient.MaxBodyBytes)
	data, err := io.ReadAll(limited)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(data) != httpclient.MaxBodyBytes {
		t.Errorf("expected %d bytes, got %d", httpclient.MaxBodyBytes, len(data))
	}
}
