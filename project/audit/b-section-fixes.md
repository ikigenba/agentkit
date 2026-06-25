# B-Section Fix Proposal — multi-clause claims where only some clauses are exercised

Research + proposed remediation for the 8 findings in section **B** of
`findings.md`. Each finding was traced from the design Verification claim → the
id-carrying test → the production code it exercises.

The 8 findings split cleanly into two kinds, and the fixes are different in
nature:

- **Group 1 — real test gaps (5).** The uncovered clause *is* observable at the
  current seam; the test simply never asserts it. Fix = strengthen/extend the
  test. No production or design change.
- **Group 2 — structurally unfalsifiable at this seam (3).** The "no-retry after
  the first byte is delivered" family. The architecture buffers each round-trip
  whole (D2: message-granular delivery, no delta surface), so an errored
  round-trip *never delivers a byte*. The dangerous state the clause guards
  against is unrepresentable, so there is nothing to falsify. Fix = **design-doc
  correction**, not a code patch — re-state the claim as the structurally-true,
  falsifiable property. This matches cross-cutting theme 1.

---

## Group 1 — test gaps to close

### R-CBA7-N2NL (D2) — `orchestration_test.go:226`

**Claim.** Each completed assistant message is emitted as a `MessageDone`
carrying the fully assembled `Message` (visible text, **tool_use blocks**, and
any **reasoning summary**) — and that same message is what landed in `History`.

**Gap.** `reflect.DeepEqual(done.Message, conv.History[1])` is proven only for a
single-`TextBlock` message. The tool_use-block and reasoning-summary message
shapes are never asserted emitted-and-equal-to-`History`.

**Production reality.** `runTurn` (orchestration.go:333-335) builds the event
from the same `message` it appends to `*history`, both via `cloneMessage`, so
the equality holds for every block type. It is genuinely testable for richer
shapes — there is just no assertion.

**Fix.** Add one focused test that drives a tool_use + reasoning-summary message
through the loop and asserts the emitted `MessageDone` `DeepEqual`s the History
entry. The helpers (`assistant`, `newRoundTrip`, `drain`) and a reasoning-bearing
round-trip already exist (cf. `TestReasoningBlockIsReplayedOnToolLoopRequest`,
line 374).

```go
func TestMessageDoneMirrorsHistoryForReasoningAndToolUse(t *testing.T) {
	// R-CBA7-N2NL
	tool := agentkit.RawTool("ok", "ok", json.RawMessage(`{"type":"object"}`),
		func(context.Context, json.RawMessage) (string, error) { return "ok", nil })
	reasoning := agentkit.ReasoningBlock{Opaque: json.RawMessage(`{"signature":"s"}`), Summary: "thinking"}
	first := assistant(reasoning,
		agentkit.TextBlock{Text: "let me check"},
		agentkit.ToolUseBlock{ID: testToolUseID, Name: "ok", Input: json.RawMessage(`{}`)})
	provider := newFakeProvider(
		newRoundTrip(first, agentkit.FinishToolUse, agentkit.Usage{}, nil),
		textRoundTrip("done"),
	)
	conv := &agentkit.Conversation{Provider: provider, Model: testModel, Tools: []agentkit.Tool{tool}}
	events := drain(conv.Send(context.Background(), "go"))

	// The first MessageDone carries the reasoning+text+tool_use shape and equals History[1].
	var firstDone agentkit.MessageDone
	for _, ev := range events {
		if d, ok := ev.(agentkit.MessageDone); ok {
			firstDone = d
			break
		}
	}
	if !reflect.DeepEqual(firstDone.Message, conv.History[1]) {
		t.Fatalf("MessageDone = %#v, want History assistant %#v", firstDone.Message, conv.History[1])
	}
	if len(conv.History[1].Blocks) != 3 {
		t.Fatalf("History assistant blocks = %d, want reasoning+text+tool_use", len(conv.History[1].Blocks))
	}
}
```

This asserts the two previously-uncovered block shapes (tool_use, reasoning
summary) are emitted-and-equal-to-History, closing the clause.

---

### R-IPGC-I69W (D3) — `google/google_test.go:165`

**Claim.** A `ReasoningBlock` produced alongside **parallel** tool calls carries
the `BoundToID` of the *specific* `ToolUseBlock` it must bind to (Gemini
positional rule).

**Gap.** The fixture has a **single** function call, so
`reasoning.BoundToID == use.ID` holds trivially — any binding logic passes.
The positional disambiguation that is the heart of the claim is never exercised.

**Production reality (verified).** `parseParts` (google.go:658-712) accumulates
`thoughtSignature` parts into a `pending` slice and `flushPending(id)` binds them
to the **next** `FunctionCall`'s freshly minted id. So for parallel calls each
reasoning block must bind to the call that follows it positionally. This is
exactly disambiguation logic — and exactly what a single-call fixture cannot
test.

**Fix.** Add a Gemini-adapter test whose `parts` stream is
`[thoughtSig_A, functionCall_A, thoughtSig_B, functionCall_B]` and assert
cross-binding: reasoning A → call A, reasoning B → call B, with
`callA.ID != callB.ID` *and* an explicit negative (`reasoningA.BoundToID != callB.ID`).
The negative is what makes it falsifiable — a naive "bind all reasoning to the
last call" implementation would fail it.

Sketch (mirroring the existing google reasoning+tool test harness; the SSE/JSON
fixture body returns two thought-signature + functionCall pairs):

```go
// R-IPGC-I69W
first := conv.History[1]
var reasonings []agentkit.ReasoningBlock
var uses []agentkit.ToolUseBlock
for _, b := range first.Blocks {
	switch b := b.(type) {
	case agentkit.ReasoningBlock:
		reasonings = append(reasonings, b)
	case agentkit.ToolUseBlock:
		uses = append(uses, b)
	}
}
if len(reasonings) != 2 || len(uses) != 2 {
	t.Fatalf("want two parallel calls each with reasoning, got %d/%d", len(reasonings), len(uses))
}
if uses[0].ID == uses[1].ID {
	t.Fatalf("parallel tool calls must have distinct minted IDs")
}
if reasonings[0].BoundToID != uses[0].ID || reasonings[1].BoundToID != uses[1].ID {
	t.Fatalf("positional binding wrong: r0->%s r1->%s, want %s/%s",
		reasonings[0].BoundToID, reasonings[1].BoundToID, uses[0].ID, uses[1].ID)
}
if reasonings[0].BoundToID == uses[1].ID {
	t.Fatalf("reasoning A must not bind to call B (positional disambiguation)")
}
```

This is the only Group-1 fix that needs a new wire fixture (two parallel
`functionCall` parts, each preceded by its own `thoughtSignature`); the rest are
pure assertions on existing harnesses.

---

### R-6W63-I4FW (D10) — `mcp_integration_test.go:399`

**Claim.** MCP-discovered tools are merged with custom tools in one deterministic
name-sorted order, **stable across turns while the attached set is unchanged**,
**and re-ordered deterministically when a server is attached/detached**.

**Gap.** The stable-across-turns half is asserted (two turns, same order). The
attach/detach **re-ordering** half is not — `R-6SIE-CT7T`'s detach test only
checks tool *removal* (list goes to empty), never a re-sort among multiple
remaining tools.

**Production reality.** `resolveMCPTools` keys the tool cache on the server set
(`mcpServerSetKey`); a set change re-discovers, and `validateAndSortTools`
name-sorts the merged `custom + mcp` list every time. So attaching a server
deterministically re-sorts the full list. Observable, untested.

**Fix.** Extend `TestMCPToolsJoinDeterministicOrderAndSchemaWarnings`: after the
two same-set turns (order `[custom_mid, srvA_alpha, srvZ_zeta]`), attach a third
list-only server whose tool sorts into the middle, run a third `Send`, and assert
the new call's tool order is the deterministically re-sorted merged list.

```go
// R-6W63-I4FW (attach re-orders deterministically)
serverM := newMCPListOnlyServer(t, "mid", `{"type":"object"}`)
defer serverM.Close()
conv.MCPServers = append(conv.MCPServers, MCPServer{Name: "srvM", URL: serverM.URL})
drainMCP(conv.Send(context.Background(), "three"))
last := provider.calls[len(provider.calls)-1]
wantReordered := []string{"custom_mid", "srvA_alpha", "srvM_mid", "srvZ_zeta"}
if got := toolNames(last.Tools); !reflect.DeepEqual(got, wantReordered) {
	t.Fatalf("after attach tools = %v, want deterministically re-sorted %v", got, wantReordered)
}
```

(`srvM_mid` sorts between `srvA_alpha` and `srvZ_zeta`, proving a *re-order*, not
just an append.)

---

### R-6XDZ-VW6L (D11) — `mcp_integration_test.go:301`

**Claim.** MCP discovery (`initialize`/`tools/list`) retries transient transport
failures under the retry policy **but fails fast on `401`/`403`/`400` and
non-MCP `4xx`**.

**Gap.** The retry-on-`500` half is covered (listCalls==2, one sleep). The
fail-fast half is not asserted under this id — the existing `401`/`403` tests
(`R-6Q2L-L9QF`) check only the error category, never *no retry / zero sleeps*.

**Production reality.** Discovery retries via `discoverMCPTools` →
`internalretry.Do` with `retryDecision`. `mcpHTTPCategory` maps `401→ErrAuthentication`,
`403→ErrPermission`, `400→ErrInvalidRequest` — all outside the retryable set, so
`Do` makes exactly one attempt. Fail-fast already works; it is simply unasserted
with a counting clock. (Use `400` for "non-MCP 4xx"; avoid `404`, which the
client treats specially as session-expiry re-init.)

**Fix.** Add a table sub-test in `TestMCPDiscoveryRetriesButToolCallDoesNot` that
returns each fail-fast status on `tools/list` with an injected `fakeMCPClock` and
`MaxAttempts > 1`, asserting one list call and zero sleeps.

```go
// R-6XDZ-VW6L (fail-fast, no retry)
for _, status := range []int{http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden} {
	var listCalls int
	srv := newMCPTestServer(t, func(w http.ResponseWriter, _ *http.Request, req mcpTestRequest) {
		switch req.Method {
		case "initialize":
			writeMCPResult(w, req.ID, `{"protocolVersion":"2025-11-25"}`)
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		case "tools/list":
			listCalls++
			http.Error(w, "nope", status)
		}
	})
	clock := &fakeMCPClock{}
	conv := &Conversation{
		Provider:   &mcpTestProvider{},
		Model:      "mcp-model",
		Retry:      RetryPolicy{MaxAttempts: 3, BaseDelay: time.Millisecond, MaxDelay: time.Millisecond},
		MCPServers: []MCPServer{{Name: "srv", URL: srv.URL}},
		retryClock: clock,
	}
	drainMCP(conv.Send(context.Background(), "hello"))
	if listCalls != 1 || len(clock.sleeps) != 0 {
		t.Fatalf("status %d: listCalls/sleeps = %d/%v, want fail-fast 1/none", status, listCalls, clock.sleeps)
	}
	srv.Close()
}
```

---

### R-6L70-26RN (D17) — `mcp_integration_test.go:178`

**Claim.** A server unreachable at the `Send` boundary (or whose
handshake/discovery fails) surfaces a uniform classifiable error before any
provider call, `History` unchanged; **one failing server is isolated to its own
attribution and does not corrupt other servers' tools**.

**Gap.** The single-server RPC-error case is covered. The **unreachable-server**
case and the **cross-server isolation** clause are never exercised — every
failure test uses one server.

**Production reality.** `resolveMCPTools` (mcp.go:116-152) iterates servers; on
the first discovery failure it `closeMCP`s and returns the per-server attributed
error (`mcpError(serverName, …)`), before any provider call. An unreachable host
yields `ErrNetwork` attributed to that server. Isolation = the error names only
the failing server, and the healthy server remains independently usable.

**Fix.** Two additions:

1. **Unreachable server** — create an `httptest` server, capture its URL, `Close`
   it, then `Send`. Use `MaxAttempts: 1` (and a `fakeMCPClock`) so the retryable
   `ErrNetwork` does not back off with the real clock.

```go
// R-6L70-26RN (unreachable)
dead := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
url := dead.URL
dead.Close()
provider := &mcpTestProvider{}
conv := &Conversation{
	Provider: provider, Model: "mcp-model",
	Retry:      RetryPolicy{MaxAttempts: 1},
	retryClock: &fakeMCPClock{},
	MCPServers: []MCPServer{{Name: "gone", URL: url}},
}
stream := conv.Send(context.Background(), "hello")
drainMCP(stream)
var akErr *Error
if !errors.As(stream.Err(), &akErr) || akErr.MCPServer != "gone" {
	t.Fatalf("Err() = %v, want network error attributed to 'gone'", stream.Err())
}
if len(conv.History) != 0 || len(provider.calls) != 0 {
	t.Fatalf("history/provider calls = %d/%d, want unchanged/no provider call", len(conv.History), len(provider.calls))
}
```

2. **Cross-server isolation** — attach `[good, bad]`; assert the failure is
   attributed to `bad` and the provider was never called; then drop `bad` and
   assert `good`'s tool is discovered and callable — proving `good` was not
   corrupted by `bad`'s failure.

```go
// R-6L70-26RN (isolation)
good := newMCPListOnlyServer(t, "echo", `{"type":"object"}`)
defer good.Close()
badURL := func() string { s := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})); u := s.URL; s.Close(); return u }()
provider := &mcpTestProvider{}
conv := &Conversation{
	Provider: provider, Model: "mcp-model",
	Retry: RetryPolicy{MaxAttempts: 1}, retryClock: &fakeMCPClock{},
	MCPServers: []MCPServer{{Name: "good", URL: good.URL}, {Name: "bad", URL: badURL}},
}
stream := conv.Send(context.Background(), "hello")
drainMCP(stream)
var akErr *Error
if !errors.As(stream.Err(), &akErr) || akErr.MCPServer != "bad" {
	t.Fatalf("Err() = %v, want failure attributed to 'bad' only", stream.Err())
}
if len(provider.calls) != 0 {
	t.Fatalf("provider called despite discovery failure")
}
// 'good' is uncorrupted: detach 'bad' and it discovers/serves normally.
conv.MCPServers = []MCPServer{{Name: "good", URL: good.URL}}
drainMCP(conv.Send(context.Background(), "again"))
if got := toolNames(provider.calls[0].Tools); !reflect.DeepEqual(got, []string{"good_echo"}) {
	t.Fatalf("good server tools = %v, want intact [good_echo]", got)
}
```

---

## Group 2 — structurally unfalsifiable; design-doc correction (cross-cutting theme 1)

These three claims share one root cause. The retry executor (`roundTripWithRetry`
→ `internalretry.Do`) decides retry **purely on `rt.Err()`**, and the orchestrator
`yield`s events only *after* `Do` returns an error-free round-trip
(orchestration.go:306-335). `provider.RoundTrip` returns the whole assembled
message at once (D2: message-granular, no delta surface). Therefore **an errored
round-trip delivers zero events** — already pinned by
`TestFailedTurnsSurfaceErrAndRollback` (R-CDQ0-EM4Z, orchestration_test.go:414:
"events before failed Err() = …, want none"). The MCP path is the same:
`CallTool` reads the full JSON-RPC response (json *or* SSE) before returning, so a
tool result is never partially delivered either.

Consequence: the state "a byte was delivered, then the round-trip failed and we
must decide whether to retry" **cannot occur**. The no-retry-after-first-byte
rule is satisfied *by construction*, not by a runtime guard, so there is no
falsifiable runtime behavior to assert. The three tests below either assert the
inverse or leave the clause empty because the clause is unrepresentable.

**Recommended fix: correct the design Verification text** (not the code, not a new
test). Re-state each claim as the structurally-true, falsifiable property the
architecture actually provides. Do **not** re-introduce an incremental-delivery
seam to make the original wording testable — D2 explicitly *rejected* token/delta
delivery, and adding a partial-delivery state purely to falsify a test would undo
a settled decision and re-open the goroutine/early-break leak surface D2 closed.

### R-P61J-IHKB (D11) — `retry_test.go:145`

- **Now reads:** "a failure after the first event has been delivered to the
  consumer is never retried, regardless of category." Untestable here; the test
  actually asserts a partial-message-bundled-with-error round-trip **is** retried
  (the inverse).
- **Propose:** retitle the claim to the real guarantee the test proves —
  *"a round-trip that fails is the retry unit: its partial assembled output is
  discarded and the entire round-trip re-issued (same idempotent `Request`) and
  re-streamed from scratch, so the consumer never sees a double-delivered or
  partial message."* The existing test already verifies exactly this
  (`provider.calls == 2`, final text `"retried"`, one backoff), so no test change
  is needed — only the claim wording, plus a one-line note that delivery is
  buffered per round-trip (D2) so "after first byte" is unreachable.

### R-Y878-6UDJ clause 2 (D11) — `retry_test.go:176`

- Clause 1 (per-round-trip budget reset across a multi-round-trip turn) is
  genuinely covered (`provider.calls == 4`, two independent backoffs). Keep it.
- Clause 2 ("a failure after any round-trip has delivered an event is not
  retried") is the structural one.
- **Propose:** drop clause 2 from this id's wording and replace it with the
  observable invariant the buffering gives: *"each round-trip's retry budget is
  independent (clause 1), and because each round-trip is delivered whole, a failed
  round-trip contributes no events — eligibility depends only on category and the
  per-round-trip budget, never on what earlier round-trips delivered."* This is
  fully covered by the existing assertions plus R-CDQ0-EM4Z.

### R-6YLW-9NXA clause 2 (D11) — `mcp_integration_test.go:337`

- Clause 1 (`tools/call` transport failure surfaced without automatic retry) is
  covered (`toolCalls == 1`, zero sleeps). Keep it.
- Clause 2 ("once a byte of a tool-result SSE stream is delivered it is never
  retried") is structural: `CallTool` assembles the whole result before returning,
  and `mcpTool.Call` is *never* wrapped in the retry executor (only discovery is),
  so a tool result is never partially delivered and never auto-retried.
- **Propose:** restate clause 2 as *"a `tools/call` is the unit: its result is
  assembled whole before being fed back, and the call is never auto-retried, so no
  partial tool-result can reach the loop."* Already covered by clause 1's
  assertions; no test change.

---

## Summary of recommended actions

| id | Decision | Kind | Action |
|---|---|---|---|
| R-CBA7-N2NL | D2 | test gap | New test: MessageDone == History for reasoning+text+tool_use |
| R-IPGC-I69W | D3 | test gap | New google fixture: two parallel calls, cross-binding asserted |
| R-6W63-I4FW | D10 | test gap | Extend order test: attach a 3rd server, assert deterministic re-sort |
| R-6XDZ-VW6L | D11 | test gap | Add fail-fast sub-test: 400/401/403 → 1 list call, 0 sleeps |
| R-6L70-26RN | D17 | test gap | Add unreachable-server + cross-server isolation assertions |
| R-P61J-IHKB | D11 | design text | Reword claim to "round-trip re-issued whole, no double-delivery"; no test change |
| R-Y878-6UDJ (cl. 2) | D11 | design text | Drop "after delivery" half; keep per-round-trip-budget (clause 1, covered) |
| R-6YLW-9NXA (cl. 2) | D11 | design text | Restate as "tools/call assembled whole, never auto-retried" (clause 1, covered) |

5 test changes (3 files: `orchestration_test.go`, `google/google_test.go`,
`mcp_integration_test.go`), only one of which (R-IPGC-I69W) needs a new wire
fixture. 3 design-doc rewordings in `D11.md`, none requiring code or test
changes — they reconcile the claim text with a guarantee the architecture
provides by construction.
