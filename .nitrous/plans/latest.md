# Plan: Better HTTP Fetch Function

## Context

The existing HTTP client lives in `internal/httpclient/client.go` (121 lines). It provides:
- A `DefaultClient` singleton with connection pooling and a 30s timeout
- `DoWithRetry` with 3 fixed attempts, exponential backoff, and jitter
- A `MaxBodyBytes` constant (10 MB) that callers must manually enforce

Key weaknesses identified:
- All configuration is hardcoded — no per-call control over retry count, timeout, or backoff
- No support for retrying requests with a body (POST/PUT/PATCH) — the body stream is consumed on the first attempt and cannot be replayed
- `search_web_tool.go:88` passes `context.Background()` instead of the caller's `ctx`, silently ignoring cancellation and deadlines from the agent loop
- No high-level convenience wrapper: callers must build the request, call `DoWithRetry`, then manually wrap the body in `io.LimitReader`

---

## Files to Change

| File | Change |
|------|--------|
| `internal/httpclient/client.go` | Replace `DoWithRetry` with a configurable `Fetch` function using functional options; keep `DefaultClient` and `MaxBodyBytes` |
| `internal/httpclient/client_test.go` | Update tests to cover the new API; add cases for body retry and custom options |
| `internal/agent/tools/search_web_tool.go` | Switch to `Fetch`; fix `context.Background()` → caller `ctx` |

---

## Approach

### 1. Functional options for configuration

Add an `Options` struct and `Option` functional-option type so callers override only what they need, with sensible defaults:

```go
type Options struct {
    MaxAttempts    int           // default 3
    InitialBackoff time.Duration // default 500ms
    MaxBackoff     time.Duration // default 16s
    BodySizeLimit  int64         // default MaxBodyBytes (10 MB)
}

type Option func(*Options)

func WithMaxAttempts(n int) Option           { return func(o *Options) { o.MaxAttempts = n } }
func WithInitialBackoff(d time.Duration) Option { ... }
func WithBodySizeLimit(n int64) Option       { ... }
```

### 2. `Fetch` — high-level convenience function

Replace `DoWithRetry` with `Fetch`, which owns the full request lifecycle:

```go
// Fetch executes req using client (DefaultClient if nil), retrying on transient
// failures. It returns the response with Body limited to opts.BodySizeLimit.
// The caller is responsible for closing resp.Body.
func Fetch(ctx context.Context, client *http.Client, req *http.Request, opts ...Option) (*http.Response, error)
```

Internally it:
1. Reads `req.Body` into `[]byte` once upfront (only when non-nil) so it can be replayed on each attempt
2. Resets `req.Body` to a fresh `io.NopCloser(bytes.NewReader(buf))` before each attempt
3. Applies exponential backoff with ±25% jitter between attempts (same algorithm as current code)
4. Wraps the successful response body in `io.LimitReader` before returning
5. Drains and closes `resp.Body` between retries (existing behaviour, preserved)

Keeping the function signature close to the old `DoWithRetry` minimises the diff in call sites.

### 3. Fix context propagation in `search_web_tool.go`

`SearchWeb` already receives a `ctx` parameter but line 88 discards it:

```go
// Before (line 88):
req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, searchURL, nil)

// After:
req, err := http.NewRequestWithContext(ctx, http.MethodGet, searchURL, nil)
```

Also update the `DoWithRetry` call (line 99) to `httpclient.Fetch(ctx, httpclient.DefaultClient, req)`.

---

## Potential Risks

| Risk | Mitigation |
|------|------------|
| Buffering large request bodies increases peak memory | Document that `Fetch` is not suitable for streaming uploads; callers sending large bodies should use `WithMaxAttempts(1)` to skip body buffering |
| Retry of non-idempotent methods (POST) may cause duplicate side effects | Document this clearly; callers that cannot tolerate duplicates should pass `WithMaxAttempts(1)` |
| Renaming `DoWithRetry` → `Fetch` breaks call sites | Only one call site exists (`search_web_tool.go:99`); update it in the same change. Package is internal, so no external API concern |
| Body buffering adds one extra allocation per request with a body | Negligible for the current use case (search API GET requests have no body) |

---

## Verification

1. **Unit tests** (`internal/httpclient/client_test.go`):
   - Existing test cases ported to `Fetch` API
   - New: success on retry with non-nil body (verifies body rewind works)
   - New: `WithMaxAttempts(1)` disables retries
   - New: `WithBodySizeLimit` truncates oversized responses
   - Existing: context cancellation mid-retry exits immediately

2. **Race detector**: `go test -race ./internal/httpclient/...` — validates no data races on the shared client

3. **Compile check**: `go build ./...` passes clean

4. **Smoke test**: With `BRAVE_API_KEY` set, run a search query end-to-end and confirm JSON results are returned correctly (validates context fix didn't break the happy path)

---

## Out of Scope

- Circuit breaker / bulkhead patterns (separate concern, adds a dependency)
- Metrics / tracing (no observability infrastructure exists yet)
- `Retry-After` header parsing for 429 responses (backoff provides partial protection)
- DNS caching (Go's default is adequate for current load)
