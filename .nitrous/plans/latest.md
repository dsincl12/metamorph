# Plan: Better HTTP Fetch Function

## Context

The only direct HTTP usage in this codebase is in `internal/agent/tools/search_web_tool.go`. The current implementation creates a bare `&http.Client{}` on every call to `SearchWeb`, with no timeout, no connection pooling, no retry logic, and no context propagation. This is the target of improvement.

---

## Problems with the Current Implementation

| Issue | Location | Impact |
|---|---|---|
| New `http.Client{}` per call | `search_web_tool.go:98` | No connection pooling; each call opens a new TCP connection |
| No timeout set | `search_web_tool.go:98` | A hung server can block the goroutine indefinitely |
| No retry logic | `search_web_tool.go:99` | Transient failures (5xx, network glitch) are not retried |
| No context propagation | `search_web_tool.go:49` | Caller cannot cancel an in-flight request |

---

## Approach

### 1. Create a shared `httpclient` package

**New file**: `internal/httpclient/client.go`

Expose a package-level singleton `http.Client` configured with:
- A **30-second total timeout** (`http.Client.Timeout`)
- A tuned `http.Transport` with sensible connection pool settings:
  - `MaxIdleConns: 100`
  - `MaxIdleConnsPerHost: 10`
  - `IdleConnTimeout: 90s`
  - `TLSHandshakeTimeout: 10s`
  - `ExpectContinueTimeout: 1s`

Also expose a `DoWithRetry` helper that wraps `client.Do` with:
- Up to **3 attempts**
- Retry only on: network errors, `429 Too Many Requests`, `500`, `502`, `503`, `504`
- Exponential backoff: 500ms, 1s (jitter optional but recommended)
- Context-aware: stop retrying if `ctx.Done()` fires

```go
// Signature
func DoWithRetry(ctx context.Context, client *http.Client, req *http.Request) (*http.Response, error)
```

Because `http.Request` bodies are consumed on first use, retries must use `GetBody` (set from a `bytes.NewReader` or left nil for GET requests).

### 2. Update `search_web_tool.go`

- Change `SearchWeb` signature to accept `context.Context` as first argument:
  ```go
  func SearchWeb(ctx context.Context, input json.RawMessage) (string, error)
  ```
- Replace `http.NewRequest` with `http.NewRequestWithContext(ctx, ...)` so the request is tied to the caller's context.
- Replace `&http.Client{}` with the package-level client from `internal/httpclient`.
- Replace `client.Do(req)` with `httpclient.DoWithRetry(ctx, httpclient.DefaultClient, req)`.

### 3. Update `ToolDefinition` / function signature wiring

**File**: `internal/agent/tools/tool_registry.go` (and `internal/agent/tools/search_web_tool.go`)

The `ToolDefinition.Function` field currently has type `func(json.RawMessage) (string, error)`. Adding context requires either:

- **Option A (preferred)**: Change the function type to `func(context.Context, json.RawMessage) (string, error)` and update all tool implementations and the call site in `agent.go`.
- **Option B (simpler, lower risk)**: Keep the existing signature and capture context via closure when registering tools (pass `ctx` at registration time, not call time). This avoids touching every tool but means the context is the one from agent startup, not from the individual tool call — acceptable for timeouts, not ideal for cancellation.

**Recommended**: Option A — threads a per-call context through cleanly and is the idiomatic Go approach. There are only ~11 tools; the change is mechanical.

---

## Files to Change

| File | Change |
|---|---|
| `internal/httpclient/client.go` | **New file** — shared client + `DoWithRetry` |
| `internal/agent/tools/search_web_tool.go` | Use shared client; add ctx param; use `NewRequestWithContext` |
| `internal/agent/tools/tool_registry.go` | Update `ToolDefinition.Function` type if Option A chosen |
| `internal/agent/agent.go` | Pass `ctx` when invoking tool functions (if Option A) |
| All other `*_tool.go` files | Update signatures to match new `Function` type (Option A only; mechanical no-op changes) |

---

## Potential Risks

1. **Retry on non-idempotent requests**: The Brave Search API is GET-only here, so retries are safe. Document in `DoWithRetry` that it must only be used for idempotent requests, or check the method explicitly.
2. **Response body leak on retry**: Must `io.Copy(io.Discard, resp.Body); resp.Body.Close()` before retrying to return the connection to the pool.
3. **Context cancellation vs. retry loop**: Retry loop must check `ctx.Err()` before each attempt and return immediately if the context is cancelled.
4. **Signature change blast radius (Option A)**: All tool `Function` fields break at compile time until updated. The compiler catches every missed site, so this is safe — but the diff is larger.
5. **`http.Transport` is not `http.RoundTripper`-interface-friendly for testing**: Consider accepting an `http.RoundTripper` in the client constructor to allow injection of a mock transport in tests.

---

## Verification

1. **Unit tests** — `internal/httpclient/client_test.go`:
   - Mock `http.RoundTripper` that returns 500 twice then 200; assert `DoWithRetry` returns the 200 response.
   - Mock that never returns; cancel context; assert `DoWithRetry` returns `context.Canceled`.
   - Mock that hangs beyond 30s timeout (use `httptest.Server` with a sleep); assert timeout error.

2. **Integration smoke test** — set `BRAVE_API_KEY` and run a single search; confirm result is returned and the connection is reused (observable via `httpclient.DefaultClient.Transport.(*http.Transport)` stats, or just verify no regression).

3. **Compile check** — `go build ./...` must pass with zero errors after all signature updates.

4. **Race detector** — `go test -race ./...` to confirm the package-level client is safe for concurrent use (it is by design, but the test validates it).
