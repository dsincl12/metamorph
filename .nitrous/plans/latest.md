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
| No response body size limit | line 125 | Unbounded `io.ReadAll` can exhaust memory |

---

## Approach

### 1. New file: `internal/httpclient/client.go`

A shared package exposing:

- A **package-level singleton** `DefaultClient *http.Client` configured with:
  - `Timeout: 30 * time.Second`
  - Tuned `http.Transport`: `MaxIdleConns: 100`, `MaxIdleConnsPerHost: 10`, `IdleConnTimeout: 90s`, `TLSHandshakeTimeout: 10s`

- A `DoWithRetry(ctx context.Context, client *http.Client, req *http.Request) (*http.Response, error)` helper:
  - Up to 3 attempts
  - Retries on: network errors, 429, 500, 502, 503, 504
  - Exponential backoff: 500ms → 1s → 2s between attempts, with jitter
  - Checks `ctx.Err()` before each attempt; returns immediately if context is cancelled
  - Drains and closes `resp.Body` before retrying (to return TCP connection to pool)
  - Only safe for idempotent methods (GET, HEAD); document this constraint
  - Sets a default `User-Agent` header if none is present

- A `MaxBodyBytes` constant (10 MB) used with `io.LimitReader` to bound response body reads

### 2. Update `internal/agent/tools/search_web_tool.go`

- Replace `http.NewRequest` with `http.NewRequestWithContext(context.Background(), ...)` (immediate win; context can be threaded in a follow-up)
- Replace `&http.Client{}` with `httpclient.DefaultClient`
- Replace `client.Do(req)` with `httpclient.DoWithRetry(ctx, httpclient.DefaultClient, req)`
- Remove manual gzip block (lines 112–122) — don't set `Accept-Encoding: gzip` header; let the transport decompress transparently via Go's built-in handling
- Wrap body read with `io.LimitReader(resp.Body, httpclient.MaxBodyBytes)`

---

## Files to Change

| File | Change |
|---|---|
| `internal/httpclient/client.go` | **New** — shared client, `DoWithRetry`, `MaxBodyBytes` |
| `internal/agent/tools/search_web_tool.go` | Use shared client, add context, remove manual gzip, add body size limit |

No changes needed to `tool_registry.go`, `agent.go`, or other tool files. Threading context through the `Function` type is a larger refactor that can be done independently later.

---

## Potential Risks

1. **Response body leak on retry**: Must drain + close `resp.Body` before each retry or the TCP connection won't return to the pool. Mitigate by using `io.Copy(io.Discard, resp.Body)` before `resp.Body.Close()`.

2. **Removing gzip header changes response format**: Removing the explicit `Accept-Encoding: gzip` header means Go's transport handles decompression transparently. The body will arrive as plain JSON. Verify with a live API call that the existing JSON parsing still works.

3. **Retrying 429 without honouring `Retry-After`**: Ideally parse the `Retry-After` header and sleep accordingly. At minimum, the exponential backoff provides partial protection.

4. **Body size limit breaks large responses**: 10 MB is generous for JSON search results, but it's a named constant (`httpclient.MaxBodyBytes`) so it can be adjusted without code changes elsewhere.

---

## Verification

1. **Unit tests** — `internal/httpclient/client_test.go` using `net/http/httptest`:
   - Mock server returning 503 twice then 200; assert success after 3 attempts
   - Cancel context mid-flight; assert `context.Canceled` is returned promptly
   - Verify body is drained between retries (mock transport tracks call count)
   - Verify 404 is **not** retried (non-retryable status)
   - Verify response larger than `MaxBodyBytes` is rejected

2. **Compile check**: `go build ./...` must pass.

3. **Race detector**: `go test -race ./...` — validates no data races in the shared client.

4. **Smoke test**: Set `BRAVE_API_KEY`, run a search query end-to-end, confirm JSON results are returned correctly (validates the gzip change didn't break response parsing).
