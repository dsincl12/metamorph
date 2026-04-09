# Plan: Better HTTP Fetch Function

## Problems with the Current Implementation

The only direct HTTP usage is in `internal/agent/tools/search_web_tool.go`. Current issues:

| Issue | Location | Impact |
|---|---|---|
| New `http.Client{}` per call | line 98 | No connection pooling; new TCP connection every call |
| No timeout | line 98 | Hung server blocks goroutine indefinitely |
| No retry logic | line 99 | Transient 5xx/network failures are not retried |
| No context propagation | line 87 | Caller cannot cancel an in-flight request |
| Manual gzip handling | lines 112–122 | Brittle; Go's transport does this automatically |

---

## Approach

### 1. New file: `internal/httpclient/client.go`

A shared package exposing:

- A **package-level singleton** `DefaultClient *http.Client` configured with:
  - `Timeout: 15 * time.Second`
  - Tuned `http.Transport`: `MaxIdleConns: 100`, `MaxIdleConnsPerHost: 10`, `IdleConnTimeout: 90s`, `TLSHandshakeTimeout: 10s`
  - The transport accepts an optional `http.RoundTripper` override for testing

- A `DoWithRetry(ctx context.Context, client *http.Client, req *http.Request) (*http.Response, error)` helper:
  - Up to 3 attempts
  - Retries on: network errors, 429, 500, 502, 503, 504
  - Exponential backoff: 500ms → 1s between attempts
  - Checks `ctx.Err()` before each attempt; returns immediately if context is cancelled
  - Drains and closes `resp.Body` before retrying (to return TCP connection to pool)
  - Only safe for idempotent methods (GET, HEAD); document this constraint

### 2. Update `internal/agent/tools/search_web_tool.go`

- Replace `http.NewRequest` with `http.NewRequestWithContext(ctx, ...)` — needs a `ctx` param
- Replace `&http.Client{}` with `httpclient.DefaultClient`
- Replace `client.Do(req)` with `httpclient.DoWithRetry(ctx, httpclient.DefaultClient, req)`
- Remove manual gzip block (lines 112–122) — don't set `Accept-Encoding: gzip` header; let the transport decompress transparently

### 3. Thread context through tool functions (Option A — recommended)

Change `ToolDefinition.Function` type from `func(json.RawMessage) (string, error)` to `func(context.Context, json.RawMessage) (string, error)`.

Update all affected files:
- `internal/agent/tools/tool_registry.go` — update the type definition
- `internal/agent/agent.go` — pass `ctx` when invoking tools
- All `*_tool.go` files — update signatures (mechanical, compiler-enforced)

---

## Files to Change

| File | Change |
|---|---|
| `internal/httpclient/client.go` | **New** — shared client + `DoWithRetry` |
| `internal/agent/tools/search_web_tool.go` | Use shared client, add ctx, remove manual gzip |
| `internal/agent/tools/tool_registry.go` | Update `Function` type to include `context.Context` |
| `internal/agent/agent.go` | Pass `ctx` when calling tool functions |
| All other `*_tool.go` files | Update signatures to match new `Function` type |

---

## Potential Risks

1. **Response body leak on retry**: Must drain + close `resp.Body` before each retry or the TCP connection won't return to the pool.
2. **Removing gzip header breaks parsing**: Removing `Accept-Encoding: gzip` means Go's transport auto-decompresses; the body will already be plain JSON. Verify with a real API call.
3. **Signature change blast radius**: All tool `Function` fields break at compile time until updated. The compiler catches every site — safe but the diff is larger. Go's type system makes this mechanical.
4. **Retrying 429 without backoff respect**: If the API returns a `Retry-After` header, ideally honour it; minimum: apply backoff before retrying 429.

---

## Verification

1. **Unit tests** — `internal/httpclient/client_test.go`:
   - Mock `http.RoundTripper` returning 503 twice then 200; assert success after retries
   - Cancel context mid-flight; assert `context.Canceled` is returned promptly
   - Verify body is drained between retries (mock transport can track call count)

2. **Compile check**: `go build ./...` must pass after all signature updates.

3. **Race detector**: `go test -race ./...` — `http.Client` and `http.Transport` are goroutine-safe by design, but the test validates no regressions.

4. **Smoke test**: Set `BRAVE_API_KEY`, run a search query end-to-end, confirm JSON results are returned correctly (validates gzip change didn't break response parsing).
