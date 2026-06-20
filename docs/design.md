# AgentKit — Design

**Authority: shape and its proof.** This document owns *how* AgentKit is built — its seams, public interfaces, naming, struct/type definitions, data model — and *how each behavior is proven* by tests. `docs/product.md` owns the *why*, the users, the scope, and the user-facing promises; this document never re-declares the why. Design *uses* the product's contractual constants by value (module path `github.com/ikigenba/agentkit`, starting version `v0.1.0`, minimum Go 1.26) but does not own them. This is the single, current statement of the architecture: when a decision changes, this doc is rewritten in place to stay true — stale decisions are removed, not stacked. The history of how the design got here lives in the plan.

## Requirement ids

- Each Decision ends with a **Verification** list: the concrete behaviors that decision requires.
- Every Verification item carries a **minted id** of the form `R-XXXX-XXXX`, minted with `idgen -p R` — never hand-written or reused.
- One id, one behavior, in exactly one place. The ids live inline in these Verification lists and nowhere else — there is no separate requirements document.
- When the design is rewritten in place, existing ids are never renumbered; a newly added behavior gets a fresh id, and a removed behavior takes its id with it.

## Conventions

- **Language/module.** Go 1.26; module `github.com/ikigenba/agentkit`; package `agentkit` at the module root. Public symbols are named so their purpose is clear from the name alone, with no package-name stutter (`agentkit.Conversation`, not `agentkit.AgentKitState`).
- **Concurrency stance.** A `*Conversation` is one conversation owned by one goroutine; it is not safe for concurrent use and does no internal locking (cf. `sql.Rows`). Documented, not enforced.
- **Credentials.** Always supplied explicitly by the consumer; AgentKit never reads environment variables, files, or any credential store on its own.

## Decision 1 — The consumer surface: the conversation object and the turn verb

**Decision.** The consumer holds **one** object — a `Conversation` — and calls **one** verb — `Send` — that returns **one** streaming type — `*Stream`. Reasoning, caching, the tool loop, retries, and provider choice are never their own top-level APIs; they are fields/registrations on the `Conversation` and event variants on the `Stream`. This single through-line is what makes the differing provider implementations present an identical outside surface: there is only one shape to learn.

```go
package agentkit

// Conversation is one multi-turn, tool-using text conversation with an LLM.
// It bundles configuration (provider, model, generation settings, system
// prompt, registered tools) with the running history, and is threaded
// explicitly into each turn. Not safe for concurrent use: one Conversation is
// owned by one goroutine (cf. sql.Rows).
type Conversation struct {
    Provider Provider    // swappable between turns; carries its own credentials (Decision 5)
    Model    string      // swappable between turns
    System   string      // system prompt — a field, not a message
    Gen      GenSettings // temperature, max tokens, native reasoning value, …
    Tools    []Tool      // registered tools
    History  []Message   // accumulates append-only across turns
}

// Send appends userText as a user turn, runs the turn to completion —
// including the automatic tool loop — and returns a Stream delivering the
// reply incrementally. Configuration is validated here, at the boundary; an
// unusable configuration — or an empty userText — surfaces through the
// Stream's terminal error.
func (c *Conversation) Send(ctx context.Context, userText string) *Stream

// Boundary-validation sentinels, surfaced via Stream.Err() before any provider
// call, matchable with errors.Is. Returned bare (not *Error — they carry no
// provider detail). Part of the unified error model (Decision 7).
var ErrInvalidConfig = errors.New("agentkit: invalid configuration") // Conversation/Tools setup
var ErrInvalidInput  = errors.New("agentkit: invalid input")         // bad Send argument (e.g. empty userText)
```

Construction is a plain struct literal with exported fields — no constructor, no functional options:

```go
conv := &agentkit.Conversation{
    Provider: anthropic.New(apiKey), // handle carries credentials (Decision 5)
    Model:    "claude-opus-4-8",
    System:   "You are concise.",
}
stream := conv.Send(ctx, "Hello")
```

Mid-conversation switching is field mutation between turns (`conv.Provider = google.New(key); conv.Model = "gemini-2.5-flash"`), with `History` carried over untouched.

Settled choices:
- **Name `Conversation`** — the name states what it is; research's `State` is vague as a public symbol.
- **Verb `Send`**, pointer receiver, mutates `History`.
- **Streaming is the only delivery surface** — one method returning `*Stream`; no blocking variant. A final-text convenience may layer on top later without changing this.
- **Validation at the boundary** (`Send`), not at construction — which is what permits the bare struct literal and free field mutation. A misconfigured `Conversation`/`Tools` set surfaces as the bare sentinel `ErrInvalidConfig`; a bad call argument — an empty `userText` (`""`) — surfaces as `ErrInvalidInput`. Both are surfaced as the `Stream`'s terminal error with nothing appended to `History` and no provider call issued (fail loudly), matched with `errors.Is`, and are part of the unified error model (D7). The two are split because they differ in consumer response: `ErrInvalidConfig` is a setup bug to fix in code, while `ErrInvalidInput` is often runtime-recoverable (e.g. reprompt on empty user input).
- **`Send` is atomic on `History` and non-re-entrant.** A turn commits to `History` only on successful completion; a failed, cancelled, or early-abandoned turn rolls back, so `History` is always a sequence of complete turns. A `Send` while the prior `Stream` is still undrained returns `ErrStreamPending` and changes nothing. Both behaviors live in the orchestrator (Decision 10).

**Rejected.**
- *`New(...)` constructor + functional options* — would force setter methods for mid-conversation switching (or expose the fields anyway), adding surface for no gain.
- *Name `State`* — fails the read-the-name test.
- *Channel- or callback-based return* — settled here that `Send` returns a single `*Stream` value, not a `chan Event` or a callback parameter (the event taxonomy itself is a later decision).
- *Separate blocking + streaming methods* — violates the single-surface principle.

**Verification.**
- R-ZWV0-CY54 — `Send` on a `Conversation` missing a required config field (provider or model) surfaces `ErrInvalidConfig` (matchable via `errors.Is`) through the returned `Stream` rather than panicking or issuing a malformed provider call. (Credential validity is the provider constructor's concern — Decision 5.)
- R-ZELD-OQNG — `Send` with an empty `userText` (`""`) surfaces `ErrInvalidInput` (matchable via `errors.Is`) through the returned `Stream`, leaves `History` unchanged, and issues no provider call.
- R-ZZAT-4HMI — after a successful turn, both the user message and the complete assistant reply are present in `History`, append-only, observable to the consumer.
- R-00IP-I9D7 — changing `Provider`/`Model` between two `Send` calls runs the second turn against the newly selected provider/model with the prior `History` intact, and the conversation continues coherently.

## Decision 2 — The streaming consumption surface: `Stream` and the `Event` taxonomy

**Decision.** `Send` returns a `*Stream` the consumer drains exactly once. Every observable thing in a turn — visible text, reasoning summary, a tool call the model requested, the tool result AgentKit fed back, the completed message, the final usage — arrives as a variant in one ordered event stream. The consumer learns one loop and one type switch, regardless of provider. This is where the "full transparency" promise lands and where reasoning and tool use are made to look uniform: they are simply event variants.

`Stream` follows the `sql.Rows`/`bufio.Scanner` shape on Go 1.23+ range-over-func:

```go
// Stream is the incremental result of one Send. Drained exactly once, by one
// goroutine. Iterating to completion (or breaking early) releases resources.
type Stream struct { /* unexported */ }

// Events yields each event of the turn in order until the turn completes or
// fails. Breaking early is safe: it runs cleanup (closes the HTTP body).
func (s *Stream) Events() iter.Seq[Event]

// Err returns the terminal error of the turn, or nil. Valid only after Events
// has been fully consumed (cf. bufio.Scanner.Err).
func (s *Stream) Err() error

// Usage returns the token usage for the turn. Valid only after Events has been
// fully consumed.
func (s *Stream) Usage() Usage

// Warnings returns any settings the provider could not honor as asked and
// degraded (Decision 6). Valid after Events is fully consumed; empty when
// everything was honored.
func (s *Stream) Warnings() []Warning
```

Consumer loop:

```go
stream := conv.Send(ctx, "What time is it in Tokyo?")
for ev := range stream.Events() {
    switch ev := ev.(type) {
    case agentkit.TextDelta:      fmt.Print(ev.Text)
    case agentkit.ReasoningDelta: // model's thinking summary
    case agentkit.ToolUse:        // model asked to use a tool
    case agentkit.ToolResult:     // AgentKit ran it, fed the result back
    case agentkit.MessageDone:    // one assistant message completed
    }
}
if err := stream.Err(); err != nil { /* typed, classifiable */ }
usage := stream.Usage()
```

The `Event` taxonomy is a sealed interface (idiomatic Go tagged union; type-switch dispatch):

```go
type Event interface{ isEvent() }

// Incremental deltas as the model generates:
type TextDelta      struct{ Text string } // visible answer fragment
type ReasoningDelta struct{ Text string } // thinking-summary fragment

// Whole, semantically-complete events:
type ToolUse    struct{ ID, Name string; Input json.RawMessage }       // model requested a tool
type ToolResult struct{ ID, Name string; Output string; IsError bool } // AgentKit's loop fed this back

// Boundary marker — one assistant message in the exchange has completed:
type MessageDone struct{ Message Message } // the full assembled message (incl. tool_use blocks)
```

Settled choices:
- **`iter.Seq[Event]` + terminal `Err()`/`Usage()`**, not `iter.Seq2[Event, error]` and not a channel: one stream error is terminal (per-event errors are awkward), `Seq2` can't carry setup/teardown errors, channels leak goroutines on early `break`. Range-over-func gives clean early-exit cleanup.
- **Delta events and whole events coexist.** Text and reasoning summary stream as fragments (`*Delta`); tool calls/results arrive whole. The provider asymmetry (Anthropic/OpenAI stream partial tool-JSON, Gemini sends it whole) is absorbed *below* this surface — the consumer always sees a complete `ToolUse`, never partial tool-JSON.
- **`ToolResult` is an event** even though AgentKit (not the model) generated it — the consumer watches the auto-loop without driving it.
- **`MessageDone` carries the assembled `Message`** — the seam between the live stream and `History`: each completed assistant message is both emitted here and appended to `History`.
- **`ctx` is the one from `Send`**, checked inside iteration; cancelling it ends the stream with a context error in `Err()`.
- **Breaking out of `Events()` early abandons the turn.** Early `break` releases resources (R-CCI4-0UEA) and, because the turn did not complete, rolls back any `History` mutations (Decision 10 atomicity) and clears the `Conversation`'s "stream live" flag — so `History` reflects only completed turns and the next `Send` is unblocked.

**Rejected.**
- *`iter.Seq2[Event, error]`* — per-event error that's really terminal; can't express setup/teardown failures.
- *`chan Event`* — goroutine leak on early break, `select` plumbing, no clean teardown hook.
- *Callbacks* — hide tool-call assembly, lose composability and early-exit.
- *A single `Delta` event with a `Kind` field* — invites stringly-typed branching; distinct sealed types make the type switch exhaustive and self-documenting.
- *Surfacing the raw reasoning replay payload as an event* — the opaque signature/encrypted blob is preserve-and-replay-only; it rides in the message model, never the consumer event stream. Only the human-readable reasoning summary streams, as `ReasoningDelta`.

**Verification.**
- R-C7MI-HRFI — draining a text-only turn yields `TextDelta` events whose concatenation equals the assembled final text, then `Err()==nil`, then a populated `Usage()`.
- R-C8UE-VJ67 — a turn where the model requests a tool surfaces, in order, a `ToolUse` with assembled complete `Input` (never partial JSON) followed by the `ToolResult` AgentKit fed back, on every provider — including Gemini (whole-JSON) and Anthropic/OpenAI (fragmented-JSON).
- R-CBA7-N2NL — each completed assistant message is emitted as a `MessageDone` carrying the fully assembled `Message`, and that same message is what landed in `History`.
- R-CCI4-0UEA — breaking out of the `Events()` loop early releases the turn's resources (HTTP body closed) without leaking a goroutine.
- R-CDQ0-EM4Z — a failed turn surfaces its error only through `Err()` after iteration ends (events stop, no panic); `Err()` is nil on success.

## Decision 3 — The canonical message & block data model

**Decision.** History is a provider-agnostic `[]Message`; each `Message` is a `Role` plus an ordered `[]Block`; `Block` is a sealed tagged union of four concrete types. This is the representation that lives in `Conversation.History`, rides in `MessageDone`, and makes mid-conversation switching possible. Canonical shape is the Anthropic superset; adapters translate to/from each wire format.

```go
// Role is the author of a message in canonical form.
type Role string
const (
    RoleUser      Role = "user"
    RoleAssistant Role = "assistant"
)

// Message is one turn: an author plus an ordered list of content blocks.
type Message struct {
    Role   Role
    Blocks []Block
}

// Block is one piece of a message. Sealed: the only implementations are the
// four concrete types below (idiomatic Go tagged union, type-switch dispatch).
type Block interface{ isBlock() }

type TextBlock struct{ Text string }

// ToolUseBlock is the model asking to run a tool. ID is AgentKit-minted in
// Anthropic's strict charset; Name is carried alongside so history stays
// portable across a provider switch (some providers match results by name).
type ToolUseBlock struct {
    ID    string
    Name  string
    Input json.RawMessage // structured tool input, never a string
}

// ToolResultBlock is the result AgentKit fed back for a ToolUseBlock. Keyed by
// the same minted ID (and Name) so every adapter can address it its own way.
type ToolResultBlock struct {
    ToolUseID string
    Name      string
    Content   string
    IsError   bool
}

// ReasoningBlock preserves a model's reasoning output for verbatim replay on
// the next tool-loop turn. Provider-and-model-bound: dropped on a provider
// switch. The opaque payload is replayed exactly — never synthesized,
// mutated, or reordered. Summary is the human-readable text (may be empty).
type ReasoningBlock struct {
    Opaque    json.RawMessage // signature / encrypted_content / thoughtSignature / raw reasoning_content
    Summary   string          // human-readable summary, if the provider gave one
    BoundToID string          // the ToolUseBlock.ID this reasoning binds to (Gemini positional rule); "" if none
}
```

Settled choices:
- **Four block types** (text, tool-use, tool-result, reasoning), sealed via unexported `isBlock()` — a closed set, so type switches are exhaustive and a fifth type is a deliberate design change.
- **Tool-call identity: AgentKit mints its own neutral IDs in Anthropic's strict charset `^[a-zA-Z0-9_-]+$`, and carries the function `Name` on both the use and result block.** At send time each adapter uses what it needs — ID for Anthropic/OpenAI, name (or echoed ID) for Gemini — and normalizes OpenAI's `tool_call_id` (Chat Completions) vs `call_id` (Responses). Raw provider IDs never cross the neutral boundary. This makes history portable under a mid-conversation switch regardless of current Gemini id behavior (research §5 conflict).
- **`Input` is `json.RawMessage`** (structured); **`ToolResultBlock.Content` is a string** (text-only scope) — tool input is structured JSON, tool output is text fed back to the model.
- **`ReasoningBlock` is first-class and preserve-and-replay-verbatim.** `Opaque` holds whatever the provider echoes; `Summary` is human-readable; `BoundToID` captures Gemini's positional binding to a specific tool call. Provider-and-model-bound: on a provider switch, reasoning blocks are dropped (only the producing model needs them). `Opaque` is `json.RawMessage` for uniformity even when a provider's payload is plain text (Z.ai) — the adapter wraps/unwraps.
- **System prompt is not a message** — it is `Conversation.System` (Decision 1), injected by each adapter as the wire format requires.

**Rejected.**
- *Flat-string message content* — recurring prior-art anti-pattern; forces XML-wrapping and regex-parsing. Ordered typed blocks instead.
- *Reusing raw provider tool-call IDs* — OpenAI-style ids corrupt an Anthropic session and break portability; we mint our own.
- *Name-only tool matching* — brittle against the §5 conflict; we carry both ID and name.
- *A `map[string]any` extension bag* — prior-art anti-pattern that metastasizes; provider-specifics live in adapters.
- *Folding reasoning into `TextBlock` with a flag* — the opaque replay payload must never be confused with visible text or mutated; a distinct block keeps "replay verbatim" enforceable.
- *Baking a provider response envelope (`Choices[]`) into `Message`* — one `Message` + a typed finish reason instead (handled in later decisions).

**Verification.**
- R-IKKQ-Z3B4 — a `ToolUseBlock`'s `ID` is AgentKit-minted and matches `^[a-zA-Z0-9_-]+$`, and the paired `ToolResultBlock.ToolUseID` equals it.
- R-ILSN-CV1T — a `History` containing tool-use/tool-result blocks built under one provider serializes correctly to each other provider's wire format on a switch (ID for Anthropic/OpenAI, name available for Gemini) with no charset corruption.
- R-IN0J-QMSI — `ReasoningBlock.Opaque` round-trips byte-for-byte through a tool-loop turn for the same provider/model (sent back unmodified and unreordered).
- R-IO8G-4EJ7 — on a mid-conversation provider switch, `ReasoningBlock`s are dropped from what is sent to the new provider while text/tool blocks carry over intact.
- R-IPGC-I69W — a `ReasoningBlock` produced alongside parallel tool calls carries the `BoundToID` of the specific `ToolUseBlock` it must bind to (Gemini positional rule).
- R-XW08-D4YL — for each reasoning provider, a reasoning+tool round-trip produces a **non-empty** `ReasoningBlock.Opaque` (Anthropic `signature`, OpenAI `encrypted_content`, Gemini `thoughtSignature`, Z.ai `reasoning_content`), so the byte-for-byte replay assertion (R-IN0J-QMSI) cannot pass on empty data.

## Decision 4 — The tool definition & registration surface

**Decision.** A tool is defined with a generic constructor at the edge that erases to a non-generic `Tool` interface the orchestration loop holds. The product promise — name, description, input shape, code — maps to `NewTool[In]`; a `RawTool` escape hatch covers inputs that don't fit a Go struct.

```go
// Tool is a registered capability the model may invoke. The Conversation holds
// []Tool; the auto-loop calls them. Non-generic so a heterogeneous set lives in
// one slice. Consumers normally build one with NewTool, not by hand.
type Tool interface {
    Name() string
    Description() string
    JSONSchema() json.RawMessage // input schema, JSON Schema
    Call(ctx context.Context, input json.RawMessage) (string, error)
    isTool() // sealed: only NewTool / RawTool produce one
}

// NewTool builds a Tool from a typed input struct. The JSON Schema is derived
// once from In (via invopop/jsonschema) and cached. fn receives the decoded
// input; its string return is the tool result fed back to the model.
func NewTool[In any](name, description string, fn func(ctx context.Context, in In) (string, error)) Tool

// RawTool is the escape hatch: supply a hand-written JSON Schema and operate on
// raw bytes, for inputs that don't map to a Go struct.
func RawTool(name, description string, schema json.RawMessage,
    fn func(ctx context.Context, input json.RawMessage) (string, error)) Tool
```

Consumer usage:

```go
type weatherIn struct {
    City string `json:"city" jsonschema:"required,description=City name"`
}
tool := agentkit.NewTool("get_weather", "Look up current weather",
    func(ctx context.Context, in weatherIn) (string, error) {
        return lookup(in.City), nil
    })
conv.Tools = append(conv.Tools, tool)
```

Settled choices:
- **Generics only at the registration edge, erased into the non-generic `Tool`** — typed `fn(ctx, In)` for the consumer (no manual decode, no manual schema), plain `Tool` for the loop, so `[]Tool` holds a heterogeneous set with no generics in orchestration.
- **Canonical schema = JSON Schema as `json.RawMessage`**, derived once from the input struct via `github.com/invopop/jsonschema` and cached. Per-provider conversion is at the adapter boundary — the lossy `JSON Schema → *genai.Schema` translation for Google isolated in one place.
- **`Call` returns `(string, error)`** — text-only result. A returned `error` becomes a `ToolResultBlock{IsError: true}` fed back to the model (in-band signal the model can recover from), not a turn-ending transport failure.
- **`Tool` is sealed (`isTool()`)** — only `NewTool`/`RawTool` produce one, so the loop holds a schema cached at construction. For `NewTool` that schema is *derived* from `In` (always well-formed). `RawTool`'s is *hand-written*, so its well-formedness (parseable, valid JSON Schema) is checked at the **`Send` boundary** — the constructor returns a bare `Tool` and cannot report a bad schema. An invalid `RawTool` schema surfaces as the `Stream`'s terminal error with no provider call issued.
- **Tool names are unique within the set; duplicates are rejected at the `Send` boundary.** `Tools` is a bare exported slice the consumer appends to (D1 — no registration method, no constructor), so AgentKit first observes the assembled set at `Send`; that is the only chokepoint where uniqueness can be enforced. Two tools sharing a `Name()` surface as the `Stream`'s terminal error, `History` unchanged and no provider call (fail loudly: providers reject duplicate tool names, and the loop's name→tool dispatch would otherwise be ambiguous).
- **Deterministic tool ordering for cache stability** (research §8) is the adapter's job at send time (sort by name, deterministic JSON), owned by the provider-request decision — not the consumer's concern.
- **MCP tools reuse this exact abstraction** (Decision 17): each is wrapped as an ordinary `Tool` whose `JSONSchema()` is the server's third-party `inputSchema` (arbitrary draft-2020-12, with `$ref`/`$defs`/`oneOf`/`additionalProperties` AgentKit does not control). The lossy Gemini conversion (R-X3VB-65U3) runs **best-effort at the Google boundary only** and **emits a non-fatal `Warning`** (Decision 6) naming the server+tool and the dropped keywords — not silent degradation. Under Anthropic/OpenAI (which take JSON Schema near-verbatim) the schema passes through and registration never fails on lossiness; the warning surfaces only if/when the conversation switches to Gemini (research line 413).

**Rejected.**
- *A generic `Tool[In]` interface* — can't live in one `[]Tool`; forces `[]any`/reflection in the loop. Erase at construction.
- *`Call` returning `(any, error)`* — text-only scope makes the result always text; `string` is honest and avoids a marshal step the consumer can't inspect.
- *Consumer hand-writes JSON Schema as the primary path* — tedious and error-prone; typed struct is primary, `RawTool` is the escape hatch.
- *Method-based registration (`conv.AddTool`)* — unnecessary; `Tools` is an exported field and append is idiomatic.
- *Reflection-based auto-discovery of tool methods* — magic; violates explicit-over-implicit.

**Verification.**
- R-WYZP-N2VB — a `Tool` from `NewTool` produces a `JSONSchema()` that reflects the struct's fields (required/description honored) and is byte-stable across calls (derived once, cached).
- R-X07M-0UM0 — when the model emits a tool call, the auto-loop decodes the input into the tool's `In` type and invokes `fn` with the decoded value; the string return becomes the `ToolResultBlock.Content`.
- R-X1FI-EMCP — a `fn` returning an error yields a `ToolResultBlock{IsError: true}` fed back to the model and the turn continues (not a terminal stream error).
- R-X2NE-SE3E — `RawTool` with a hand-written schema invokes its `fn` with the raw input bytes and feeds back the string result identically.
- R-SX1B-XRK2 — a `RawTool` whose hand-written schema is not parseable/valid JSON Schema surfaces `ErrInvalidConfig` (matchable via `errors.Is`) through the returned `Stream` at the `Send` boundary, with no provider call issued; a valid one passes the gate.
- R-SZH4-PB1G — a `Send` whose `Tools` contains two tools sharing a `Name()` surfaces `ErrInvalidConfig` (matchable via `errors.Is`) through the returned `Stream`, leaves `History` unchanged, and issues no provider call.
- R-X3VB-65U3 — the Google adapter converts a tool's JSON Schema to `*genai.Schema`, dropping unsupported constructs (`$ref`/`additionalProperties`/`oneOf`) without erroring.
- R-6ZTS-NFNZ — an MCP tool whose third-party `inputSchema` contains Gemini-unsupported constructs converts best-effort at the Google boundary and emits a `Warning{Setting:"tool_schema", Code:WarnToolSchemaLossy}` (via `Stream.Warnings()`) whose `Detail` names the server+tool and dropped keywords; under Anthropic/OpenAI the same tool registers and runs with no warning.

## Decision 5 — Provider packaging, selection, and credential placement

**Decision.** One sub-package per provider family; each `New` returns the opaque `agentkit.Provider` handle the consumer assigns to `Conversation.Provider`. Credentials are constructor arguments baked into the handle — so there is no separate `Creds` field on `Conversation`.

```go
// package agentkit
//
// Provider is a handle to a configured provider backend. Consumers obtain one
// from a provider sub-package's New and assign it to Conversation.Provider;
// they do not call its methods directly. Its method set is the exported SPI
// defined in Decision 9 (exported because sub-package adapters must implement
// it across a package boundary).
type Provider interface{ /* SPI — see Decision 9 */ }
```

```go
import (
    "github.com/ikigenba/agentkit"
    "github.com/ikigenba/agentkit/anthropic"
    "github.com/ikigenba/agentkit/google"
    "github.com/ikigenba/agentkit/openai"
    "github.com/ikigenba/agentkit/zai"
)

conv := &agentkit.Conversation{Provider: anthropic.New(apiKey), Model: "claude-opus-4-8"}
conv.Provider = google.New(geminiKey); conv.Model = "gemini-2.5-flash"          // switch
conv.Provider = zai.New(zaiKey); conv.Model = "glm-5.2"                          // base URL internal
```

Constructor shapes (each returns `agentkit.Provider`):

```go
func anthropic.New(apiKey string, opts ...Option) agentkit.Provider
func google.New(apiKey string, opts ...Option) agentkit.Provider
func openai.New(apiKey string, opts ...Option) agentkit.Provider                 // Responses API
func zai.New(apiKey string, opts ...Option) agentkit.Provider                     // OpenAI-compatible wire, base URL internal
```

Settled choices:
- **Four sub-packages: `anthropic`, `google`, `openai`, `zai` — four first-class, named providers.** `zai` happens to speak Z.ai's OpenAI-compatible Chat-Completions wire, but that is an implementation detail: `zai.New(apiKey)` bakes in Z.ai's base URL (`https://api.z.ai/api/paas/v4/`) and constructs the shared `internal/openaicompat` adapter (research §2.4, §10), which is never part of the public surface. **Provider first-classness is independent of implementation strategy** — a provider reached via API-compatibility still gets its own package, its own `New(apiKey)`, its own model registry/pricing, and its own `Error.Provider` label (`"zai"`, never `"openaicompat"`); the consumer sees four peers and need never supply a base URL to select one — `zai.New(apiKey)` alone suffices (a base URL is *overridable* via the `WithBaseURL` option below, but never *required* to pick or use a provider). Any future OpenAI-compatible provider follows the same pattern (its own named package over the shared internal adapter). There is no public `openaicompat` package.
- **Credentials are constructor arguments, baked into the handle — `Conversation` has no `Creds` field.** Cleaner, keeps "explicit at construction," and makes a provider switch a single assignment carrying its own auth. (Revised Decision 1 accordingly.)
- **`Model` stays a `Conversation` string field**, separate from the handle — switching model within a provider is a cheap string change. **Validity is the model registry (D16): at the `Send` boundary the adapter rejects any id the provider's registry doesn't know, with a typed error, before issuing a call.** That registry is the single source of truth shared with pricing, so the supported set and the priced set are identical by construction (closed curated set, per product).
- **Per-provider packaging isolates dependencies** — importing only `anthropic` pulls no Google/OpenAI deps; decisive if SDKs are wrapped (research §10) and good hygiene regardless.
- **Functional options per constructor are a supported public surface, not a test-only seam.** `WithBaseURL(string)` and `WithHTTPClient(*http.Client)` are legitimate consumer construction overrides — point any provider at an alternate, self-hosted, proxied, or API-compatible endpoint, or supply a custom HTTP client (timeouts, transport, instrumentation). They are the one place options fit — backend-local construction details — unlike the freely-mutated `Conversation` fields. The default is always correct without them: each provider bakes in its canonical base URL and `WithBaseURL` only overrides it, never a requirement. The motivating consumer case is Z.ai's coding-plan endpoint (`https://api.z.ai/api/coding/paas/v4`) in place of the default (`…/paas/v4`). This is the *same* option the D13 test harness points at an `httptest.Server`, so consumer override and testability share one seam; its routing behavior is proven by R-WKTI-LIIE.
- **Model-name constants** exported per sub-package for discoverability (`anthropic.ModelOpus48 = "claude-opus-4-8"`, etc.). The field is a plain `string` so any value *compiles*, but only registry-known ids pass `Send` validation — the exported constants enumerate exactly the supported (and priced) set; an unknown id is rejected, not silently attempted.

**Rejected.**
- *Single package with `agentkit.NewAnthropic(...)`* — forces every consumer to compile in all four providers' deps; loses isolation.
- *Separate `Creds` field on `Conversation`* — redundant once the handle carries auth; two fields to coordinate and ambiguous on switch.
- *`Credentials` passed at `Send`* — pushes auth into the call path; construction-time is the right boundary.
- *Provider as a string enum* — stringly-typed; can't carry construction options or isolate deps.

**Verification.**
- R-H3PK-QFG3 — each provider sub-package's `New` returns a value assignable to `Conversation.Provider`, and a `Send` against it issues a correctly-authenticated request to that provider's endpoint.
- R-H4XH-476S — `zai.New` issues correctly bearer-authenticated requests to Z.ai's baked-in base URL via the shared internal OpenAI-compatible adapter; the consumer supplies only the API key (no base URL), and that adapter is not exposed as a public package.
- R-H65D-HYXH — assigning a new `Provider` (and `Model`) between turns routes the next `Send` to the new backend with `History` intact.
- R-7GGH-BPYN — `Send` with a `Model` id the selected provider's registry does not know surfaces `ErrInvalidConfig` (matchable via `errors.Is`) through the returned `Stream` (validity gate, D16) and issues no provider call; every exported model constant passes this gate.

## Decision 6 — Generation settings and the native reasoning value

**Decision.** `Conversation.Gen` holds uniform sampling controls plus a **native-per-model** reasoning value. Reasoning is expressed in the *selected model's own* native term and values — there is **no** cross-model vocabulary and **no** translation. A value the model natively understands is honored exactly; anything it does not (unknown term, invalid level, out-of-range budget, a disable request a model can't honor, or a value carried over from a previously-selected model) is **warned and falls back to the model's default** — never silent, never turn-breaking. The per-model spec that defines "natively understood," and the introspection that exposes it, live in Decision 16; the warn surface is `Stream.Warnings()` (Decision 2).

The consumer sets reasoning with a tagged `ReasoningValue` carrying exactly one native form, so the value flows to the adapter untranslated:

```go
// GenSettings holds uniform generation controls. The zero value is "use each
// provider's defaults": nil/0/unset fields are omitted from the request.
type GenSettings struct {
    Temperature *float64       // nil → provider default (pointer so 0.0 is distinguishable)
    TopP        *float64       // nil → provider default
    MaxTokens   int            // 0 → adapter-supplied default (Anthropic requires a value)
    Reasoning   ReasoningValue // zero value = unset → model default, no warning
}

// ReasoningValue carries exactly one native reasoning form for the selected
// model, flowing to the adapter untranslated. The four states — unset, a native
// level string, a native token budget, explicit-disabled — are mutually
// exclusive by construction (unexported fields; built only via the constructors
// below). The zero value is "unset": use the model's default, emit no warning.
type ReasoningValue struct { /* unexported: tag + level string + budget int */ }

func Level(s string) ReasoningValue    // a native level string: Level("high"), Level("xhigh"), Level("minimal")
func Budget(n int) ReasoningValue      // a native token budget: Budget(8000)
func DisableReasoning() ReasoningValue // explicit off; the adapter lowers it to the model's native off-form
```

```go
conv.Gen.Reasoning = agentkit.Level("xhigh")   // valid on opus, warns+defaults on sonnet (no xhigh)
conv.Gen.Reasoning = agentkit.Budget(8000)     // valid on gemini-2.5/haiku, warns on an enum model
conv.Gen.Reasoning = agentkit.DisableReasoning() // off where the model allows it; warns on a can't-disable model
```

**Warn-and-default mechanism.** Validation is a single choke point at **request-build time, in the adapter**, against the request's selected model: `spec := <registry>.ReasoningSpec(req.Model); if !spec.accepts(req.Gen.Reasoning) { apply spec.Default; emit one Warning }`. A natively-understood value (or unset) emits nothing and is sent as-is. All the non-native cases — wrong value kind (a budget on an enum model or a level on a range model), a level string outside `spec.Levels`, a budget outside `[Min,Max]` and not a sentinel, a disable a model can't honor, and a value **carried over from a previously-selected model** — funnel through this one check; the carried-over case is *why* validation must be at build time against `req.Model`, not at set time. The fallback never breaks the turn: the request is still issued, with the model's default reasoning.

The `Warning` is one honest flat struct, shared by every degrade-and-warn case (reasoning, Z.ai `tool_choice`, MCP/Gemini schema lossiness), surfaced via `Stream.Warnings()` (D2):

```go
// Warning records a requested setting the adapter could not honor as asked and
// degraded. Every field is meaningful for every warning kind — no kind-specific
// fields left zero.
type Warning struct {
    Setting string      // "reasoning" | "tool_choice" | "tool_schema"
    Code    WarningCode // classifiable reason
    Detail  string      // human-readable: what was asked, what was applied instead
}

type WarningCode int
const (
    WarnReasoningUnsupported  WarningCode = iota // value not accepted by the model (wrong kind, bad level, out-of-range budget, or carried over from a prior model)
    WarnReasoningCannotDisable                   // DisableReasoning() against a model that cannot disable
    WarnToolChoiceForced                         // forced tool_choice degraded to auto (Z.ai)
    WarnToolSchemaLossy                          // MCP/tool JSON-Schema keywords dropped at the Gemini boundary (D4)
)
```

Settled choices:
- **`ReasoningValue` is a tagged carrier with three constructors** (`Level`/`Budget`/`DisableReasoning`) and an unset zero value — no exported fields, so the four states cannot be combined or half-set. This mirrors the sealed-union idiom already used for `Block` (D3) and `Event` (D2).
- **`Level` takes a raw native string, not a typed enum.** The accepted set differs per model and there is no honest cross-model enum (the reason the former `ReasoningEffort` ordinal was removed); validity is judged later against the model's spec (D16), not by the type system here.
- **`DisableReasoning()` is first-class**, not `Level("none")`/`Budget(0)` — *which* value means "off" is model-specific (`thinking:{type:"disabled"}` vs `reasoning.effort:"none"` vs `thinkingBudget:0`); the consumer expresses intent and the adapter lowers it to that model's native off-form (or warns+defaults if the model cannot disable).
- **Unset (zero value) leaves the model default** — an untouched `GenSettings` sends no reasoning parameter and emits no warning, so a consumer who ignores reasoning is unaffected.
- **No cross-model translation in the type.** A `ReasoningValue` reaches the provider adapter as the exact native form it was constructed with; the per-adapter lowering to wire fields is Decision 9, the warn-and-default contract is below.
- **Degrade-with-warning, never silent, never turn-breaking.** A reasoning value the selected model does not natively understand falls back to the model's default and records a `Warning` (surfaced via `Stream.Warnings()`); the turn still succeeds. The same degrade-and-warn rule covers Z.ai's `tool_choice=auto`-only constraint.
- **Validation is one choke point, in the adapter, at request-build time.** All five non-native cases (wrong kind, bad level, out-of-range budget, can't-disable, carried-over) reduce to `spec.accepts(req.Gen.Reasoning)` against `req.Model`. It lives in the adapter, not the orchestrator: the adapter already produces `Warning`s (`RoundTrip.Warnings()`, D9), reads its own registry directly, and the carried-over case is only detectable once the new model is the selected one — i.e. at build time.
- **`Warning` is one honest flat struct, not a god-struct and not a union.** `{Setting, Code, Detail}` — every field meaningful for every warning kind. `Code` classifies (so a consumer can tell programmatically that reasoning was not honored); `Detail` is the human string. The five reasoning failure modes collapse to two codes (`WarnReasoningUnsupported`, `WarnReasoningCannotDisable`) because they share one detection point and one remedy; "carried-over" is a *cause* noted in `Detail`, not its own code.
- **Pointers for `Temperature`/`TopP`** distinguish "unset" (omit) from a deliberate `0.0`. `MaxTokens int` with `0` → adapter-default (supplies Anthropic's required value).
- **Removing `ReasoningEffort` is a breaking change to the reasoning surface**, acceptable under the pre-stable `v0.x` line (product contractual constant: starting `v0.1.0`, `v0` signals a public-but-evolving surface). The `ReasoningEffort` enum and the `EffortDefault…` constants are gone; consumers move to `ReasoningValue` + the `ReasoningInspector` introspection (D16). No compatibility shim is kept (principles: delete over maintain).

**Rejected.**
- *A single neutral `ReasoningEffort` ordinal mapped per provider* — the removed design; "nearest" is undefinable across a discrete enum and an integer token budget, and per-model value sets differ even within effort-enum providers (research §7.1). Native-first is strictly more truthful for a verification harness.
- *`Level`/`Budget`/disabled as separate exported fields on `GenSettings`* — lets a consumer set more than one (ambiguous) and leaks the tag; the opaque carrier forbids it.
- *`Reasoning *ReasoningValue` (pointer for unset)* — the zero value already means unset cleanly; a pointer only adds nil-handling.
- *Overloading `Level("none")`/`Budget(0)` for disable* — forces the consumer to know each model's native off-token, breaking the thin-consumer promise; `DisableReasoning()` carries the intent.
- *Plain `float64` temperature with a sentinel* — `0.0` is legal; pointer is unambiguous.
- *An enriched `Warning` with `Model`/`Given`/`Applied ReasoningValue` fields* (research §7.1 sketch) — those fields are meaningless for `tool_choice`/`tool_schema` warnings, so the struct would lie for two of three kinds (exactly D7's rejected god-struct); `Detail` carries "requested X, applied Y" without the lie.
- *A sealed `Warning` interface with per-kind variants* — the most strongly-typed option and it matches the `Block`/`Event` idiom, but consumers only display warnings, not branch on their payloads; D7 already set the precedent of one honest struct (not a union) for this exact "several failure shapes, one type" situation.

**Verification.**
- R-P5U3-5CFZ — a `GenSettings` with `Temperature`/`TopP`/`MaxTokens` set reaches each provider's request with those values; leaving them nil/0 omits the parameter so the provider default applies.
- R-B7YX-J342 — a reasoning value the selected model does not natively understand (wrong kind, a level outside `spec.Levels`, or a budget outside `[Min,Max]`) is replaced by `spec.Default` in the built request and records exactly one `Warning{Setting:"reasoning", Code:WarnReasoningUnsupported}`; the request is still issued and the turn succeeds.
- R-B96T-WUUR — validation runs at request-build against `req.Model`: a `Reasoning` value native to a *previously*-selected model but invalid for the now-selected model warns (`WarnReasoningUnsupported`) and applies the new model's default — proving the carried-over case is caught at the build-time choke point, not at set time.
- R-T40A-VZQ7 — a `Level`/`Budget`/`DisableReasoning` value reaches the provider adapter as the exact native form it was constructed with, with no cross-model mapping applied between the consumer call and the adapter seam.
- R-T587-9RGW — an unset (zero-value) `Reasoning` omits all reasoning parameters from the built request and produces no `Warning`.
- R-T6G3-NJ7L — the four `ReasoningValue` states (unset, level, budget, disabled) are mutually exclusive: a value built by one constructor never presents as another at the adapter seam (e.g. a `Budget` value is not readable as a level).
- R-P89V-WVXD — a `DisableReasoning()` against a model that cannot disable reasoning (gemini-2.5-pro, gpt-5.5-pro) falls back to the model's default and records a `Warning{Code:WarnReasoningCannotDisable}` via `Stream.Warnings()`, and the turn still succeeds.
- R-P9HS-ANO2 — a forced `tool_choice` against the `zai` provider (Z.ai, `auto`-only) degrades to `auto` and records a `Warning{Code:WarnToolChoiceForced}` rather than failing.
- R-PBXL-275G — when every requested setting is honored, `Stream.Warnings()` is empty.

## Decision 7 — The error model

**Decision.** Every failure surfaces through `Stream.Err()` and is classified with **one idiom — `errors.Is(err, ErrX)`** — across three families:

1. **Provider failures** additionally carry a rich typed `*Error` (the sentinel `Category`, plus `Provider`, `StatusCode`, verbatim `Raw` body, server-advised `RetryAfter`), reachable with `errors.As`. No provider-specific knowledge is needed; no message string-matching.
2. **Orchestration conditions** — `ErrToolLoopLimit` (D10), `ErrStreamPending` (D10), `ErrClosed` (D15) — are **bare** sentinels (no payload).
3. **Boundary-validation errors** — `ErrInvalidConfig`, `ErrInvalidInput` (D1), raised at `Send` before any provider call — are **bare** sentinels.

`errors.Is` is the single branching contract over all three; `*Error` is *additive* detail that **only provider failures attach** — its fields (`Provider`/`StatusCode`/`Raw`/`RetryAfter`) are provider-HTTP concepts, so a local condition that has none of them is a bare sentinel rather than a mostly-nil struct. A consumer that uses `errors.As(err, &agentkit.Error{})` as its *sole* branch sees provider failures but misses every bare sentinel — branch with `errors.Is`. This split mirrors the standard library (`fs.ErrNotExist` bare sentinel vs `*fs.PathError` rich type that also satisfies `errors.Is`).

```go
// Sentinel categories — branch with errors.Is, never by string-matching Message.
var (
    ErrAuthentication = errors.New("agentkit: authentication")
    ErrPermission     = errors.New("agentkit: permission")
    ErrInvalidRequest = errors.New("agentkit: invalid request")
    ErrNotFound       = errors.New("agentkit: not found")
    ErrRateLimited    = errors.New("agentkit: rate limited")
    ErrOverloaded     = errors.New("agentkit: overloaded")
    ErrServerError    = errors.New("agentkit: server error")
    ErrTimeout        = errors.New("agentkit: timeout")
    ErrNetwork        = errors.New("agentkit: network")
    ErrContextLength  = errors.New("agentkit: context length exceeded")
    ErrContentFilter  = errors.New("agentkit: content filtered")
    ErrBilling        = errors.New("agentkit: billing")
    ErrUnknown        = errors.New("agentkit: unknown")
)

// Error is the uniform provider error. Branch on Category via errors.Is;
// inspect the raw body via errors.As. Never string-match Message.
type Error struct {
    Category   error           // one of the sentinels above
    Provider   string          // "anthropic" | "google" | "openai" | "zai"; "" for MCP failures
    MCPServer  string          // attached server name for an MCP failure; "" for LLM-provider failures
    StatusCode int             // HTTP status; 0 if transport-level
    Type       string          // provider error-type string or numeric code
    Message    string          // provider human-readable message
    RequestID  string          // provider request id, if present
    RetryAfter time.Duration   // server-advised delay; 0 if none
    Raw        json.RawMessage // verbatim provider error body — never re-marshaled
    Err        error           // wrapped transport error, if any
}

func (e *Error) Error() string
func (e *Error) Is(target error) bool { return target == e.Category }
func (e *Error) Unwrap() error        { return e.Err }
```

Settled choices:
- **Thirteen sentinel categories** (research §6.1), each a package-level `error` var; `(*Error).Is` matches the carried `Category`, so `errors.Is(err, ErrX)` is the branching idiom. `ErrUnknown` is the catch-all so classification never silently drops.
- **One `Error` struct** carries provider, status, type, message, request id, server-advised `RetryAfter`, the **verbatim raw body** (`json.RawMessage`, never re-marshaled), and a wrapped transport `Err` for `errors.Unwrap`/`errors.As`.
- **Classification is the adapter's job: HTTP status first, refined by provider error-type string** — except **Z.ai, which classifies on its string-numeric `code`** (no `type` in its envelope). Each adapter owns its mapping per the §6.1 matrix.
- **Context-length and content-filter** are normalized to `ErrContextLength`/`ErrContentFilter` even when the provider signals them via a finish/block reason rather than an HTTP error.
- **`Provider` is a plain string** on the error (a diagnostic label), decoupled from the `Provider` handle type.
- **MCP failures reuse this taxonomy — no new sentinel** (Decision 17, research line 419). Attribution is a dedicated `MCPServer` field (the failing server's name) rather than a `Provider = "mcp:<name>"` convention, keeping `Provider` strictly LLM-valued. Mapping: handshake/discovery transport failure → `ErrNetwork`/`ErrTimeout`; `401` → `ErrAuthentication` (with `WWW-Authenticate` preserved in `Raw`/`Message`); `403` → `ErrPermission`; wrong-URL / non-MCP `4xx` → `ErrNotFound`/`ErrInvalidRequest`; `5xx` → `ErrServerError`; JSON-RPC `-32601`/`-32602`/`-32600` → `ErrInvalidRequest`; JSON-RPC `-32603` / server `-32000..-32099` / `-32700` → `ErrServerError`. `Raw` carries the verbatim JSON-RPC `error` object (or HTTP error body); the JSON-RPC `code` maps into `Error.Type`. A `405` on the GET stream / on `DELETE` is benign, not an error.
- **`*Error` is the payload of provider failures only; every other condition is a bare sentinel.** The orchestration sentinels (`ErrToolLoopLimit`, `ErrStreamPending`, `ErrClosed`) and the boundary-validation sentinels (`ErrInvalidConfig`, `ErrInvalidInput`) carry no `Category`/`Raw` and are matched solely with `errors.Is`. Each sentinel is declared **where its condition arises** — the provider categories here, orchestration in D10/D15, boundary in D1 — and D7 is the model that unifies them under the one `errors.Is` contract; `errors.As(err, &agentkit.Error{})` selects provider failures specifically. (We reject a single god-`*Error` for all failures: its provider-shaped fields would be nil for local conditions, a type whose shape lies about its contents — the stdlib `errors.Is`/`errors.As` split is the idiomatic Go model.)

**Rejected.**
- *`Category` as an `int` enum* — sentinel `error` values compose directly with `errors.Is`; an int enum needs a translation layer.
- *Per-provider error types* — defeats the uniform-surface promise; one `*agentkit.Error` with a `Provider` field.
- *String-matching provider messages to branch* — brittle; branch on `Category`.
- *Re-marshaling the provider body* — lossy; `Raw` is the bytes as received.

**Verification.**
- R-BUR1-XAK8 — each provider's documented error responses (the §6.1 matrix) map to the correct sentinel `Category`, asserted table-driven against recorded fixtures.
- R-BVYY-B2AX — `errors.Is(err, ErrRateLimited)` (and each other sentinel) returns true for a matching error and false for a non-matching one.
- R-BX6U-OU1M — `(*Error).Raw` equals the provider's error body byte-for-byte (not re-marshaled), and `errors.As` extracts `Provider`/`StatusCode`/`RequestID`.
- R-BYER-2LSB — `RetryAfter` is populated from the server signal where present (Anthropic/OpenAI `Retry-After` header, Gemini body `RetryInfo.retryDelay`) and is 0 when absent.
- R-BZMN-GDJ0 — a Z.ai error is classified by numeric `code` (e.g. `1302`→`ErrRateLimited`, `1110`→`ErrBilling`), not by an OpenAI-style `error.type`.
- R-I5VJ-CTXE — a provider failure from `Stream.Err()` satisfies `errors.As(err, &agentkit.Error{})` and carries a non-nil `Category`, whereas each orchestration sentinel (`ErrToolLoopLimit`, `ErrStreamPending`, `ErrClosed`) is matched by `errors.Is` but does **not** satisfy `errors.As(&agentkit.Error{})` — so the consumer can distinguish the two kinds.
- R-7CYE-KS40 — each boundary-validation sentinel (`ErrInvalidConfig`, `ErrInvalidInput`) is matched by `errors.Is` but does **not** satisfy `errors.As(&agentkit.Error{})`, confirming the one-idiom model: `errors.Is` classifies all three failure families while `*Error` is attached to provider failures alone.
- R-6TQA-QKYI — an MCP failure produces an `*Error` whose `MCPServer` is the failing server's name and whose `Provider` is empty, classified to the correct sentinel per the MCP mapping (e.g. JSON-RPC `-32601` → `ErrInvalidRequest`, `5xx`/`-32603` → `ErrServerError`), with `Raw` carrying the verbatim JSON-RPC `error`/HTTP body and the JSON-RPC `code` in `Type`.
- R-6UY7-4CP7 — an MCP `401` carrying `WWW-Authenticate` maps to `ErrAuthentication` with the header value preserved in `Raw`/`Message`, and a `403` maps to `ErrPermission`; no new MCP-specific sentinel exists.

## Decision 8 — The uniform `Usage` struct (disjoint token buckets)

**Decision.** `Stream.Usage()` returns disjoint token buckets a consumer can price as `Σ bucket × rate[bucket]`. Adapters subtract per provider so the buckets are genuinely disjoint. Usage is the summed turn total across the tool loop's round-trips.

```go
// Usage reports token consumption for a turn in disjoint buckets.
//
// The summing buckets are InputUncached, CacheReadInput, CacheWriteInput,
// Output, and ReasoningOutput; they sum to Total. CacheWrite5m and CacheWrite1h
// are an informational sub-split of CacheWriteInput (Anthropic only) and are
// NOT added again. Any field a provider cannot report stays 0.
type Usage struct {
    InputUncached   int64 // fresh input, never cached
    CacheReadInput  int64 // input served from cache (discounted)
    CacheWriteInput int64 // input written to cache (Anthropic only; else 0)
    CacheWrite5m    int64 // subset of CacheWriteInput, 5m tier (Anthropic only)
    CacheWrite1h    int64 // subset of CacheWriteInput, 1h tier (Anthropic only)
    Output          int64 // visible output, excluding reasoning where separable
    ReasoningOutput int64 // thinking/reasoning tokens (0 where not separable)
    Total           int64 // InputUncached+CacheReadInput+CacheWriteInput+Output+ReasoningOutput
}
```

Settled choices:
- **Disjoint summing buckets** = `{InputUncached, CacheReadInput, CacheWriteInput, Output, ReasoningOutput}` → `Total`. `CacheWrite5m`/`CacheWrite1h` are a sub-split of `CacheWriteInput` (not part of the sum) — tightened from research's struct comment, which ambiguously called all fields disjoint; double-counting them would break the sum.
- **Per-provider mapping with subtraction** (research §6.3 table), owned by each adapter: OpenAI/Gemini/Z.ai prompt count *includes* cached, so `InputUncached = prompt − cached`; OpenAI separates reasoning, so `Output = output − reasoning`; Gemini reports thoughts separately already; Anthropic & Z.ai can't separate reasoning, so `ReasoningOutput = 0` and reasoning stays inside `Output`.
- **`Total`: Anthropic derived** (no native total — sum the buckets); the other three carry a native total the adapter **asserts equals the bucket sum** as a regression canary.
- **`int64`** counts (defensive on large-context turns).
- **Turn-total semantics** — `Usage()` sums the multiple provider round-trips of one `Send` into a single turn total; per-round-trip detail is not exposed.

**Rejected.**
- *A flat `{Input, Output, Total}` triple* — can't express cached/uncached/cache-write; consumer can't price correctly.
- *Reasoning folded into `Output` everywhere* — loses the breakdown OpenAI/Gemini provide.
- *A `map[string]int64` of native fields* — pushes provider parsing onto the consumer.
- *Counting `CacheWrite5m`/`1h` as independent summing buckets* — double-counts against `CacheWriteInput`.

**Verification.**
- R-Y810-TECF — for each provider, a recorded usage response maps to the buckets per the §6.3 table, with the documented subtractions applied so the summing buckets are disjoint.
- R-Y98X-7634 — the summing buckets sum to `Total`; for OpenAI/Gemini/Z.ai the provider's native total equals that sum (asserted), and Anthropic's `Total` is the derived sum.
- R-YAGT-KXTT — `CacheWrite5m + CacheWrite1h == CacheWriteInput` for Anthropic, and all three cache-write fields are 0 for the other providers.
- R-YBOP-YPKI — `ReasoningOutput` is populated for OpenAI and Gemini and 0 for Anthropic and Z.ai (reasoning remaining inside `Output` for those two).
- R-YCWM-CHB7 — for OpenAI/Gemini/Z.ai cached tokens are subtracted out of `InputUncached` (cached ⊂ prompt), while Anthropic's `InputUncached` comes straight from `input_tokens` (already cache-exclusive).

## Decision 9 — Package architecture & the provider adapter seam (SPI)

**Decision.** The root `agentkit` package owns the consumer types, the orchestration, and the `Provider` SPI; each provider lives in a sub-package that imports root and implements the SPI. Because a sub-package cannot implement another package's unexported methods, **the SPI is exported** (correcting Decision 5's "unexported methods" sketch). The dependency graph has no cycles: root never imports a sub-package.

```
agentkit (root)  ── consumer types + orchestration + Provider SPI
   ▲   ▲   ▲   ▲
   └───┴───┴───┴── agentkit/{anthropic,google,openai,zai} import root, implement Provider
```

A consumer imports root + whichever sub-package(s) they use — the `database/sql` + `database/sql/driver` pattern, minus string-registration (explicit construction).

```go
// Provider is implemented by provider sub-packages. Consumers obtain one from a
// sub-package's New and assign it to Conversation.Provider; they do not call
// RoundTrip directly. This is the SPI for adding a provider.
type Provider interface {
    // RoundTrip performs ONE model call and returns a low-level stream the
    // orchestrator drains. The auto-tool-loop, history, and transparency live
    // in the orchestrator above this — not in the adapter.
    RoundTrip(ctx context.Context, req *Request) *RoundTrip
    Name() string // labels Error.Provider
    Pricing(model string) (Pricing, bool) // registry lookup; false = provider doesn't know this model id (validity gate, Decision 16)
}

// Request is one round-trip's input, built by the orchestrator from the
// Conversation. The adapter translates it to its wire format.
type Request struct {
    Model    string
    System   string
    Messages []Message // full history, resent every round-trip (stateless)
    Tools    []Tool
    Gen      GenSettings
}

// RoundTrip is one model call's low-level result. The adapter streams text and
// reasoning-summary deltas, assembles tool-call JSON centrally, and exposes the
// assembled assistant Message plus metadata after Events is drained.
type RoundTrip struct { /* unexported */ }
func (r *RoundTrip) Events() iter.Seq[Event] // TextDelta / ReasoningDelta only
func (r *RoundTrip) Message() Message         // assembled assistant message (text+tool_use+reasoning blocks)
func (r *RoundTrip) Finish() FinishReason
func (r *RoundTrip) Usage() Usage
func (r *RoundTrip) Warnings() []Warning
func (r *RoundTrip) Err() error

// FinishReason is the normalized reason a round-trip ended.
type FinishReason int
const (
    FinishStop          FinishReason = iota // natural end
    FinishToolUse                            // model requested tools
    FinishMaxTokens
    FinishContentFilter
    FinishOther
)
```

Settled choices:
- **The SPI is exported** (`Provider`, `Request`, `RoundTrip`, `FinishReason`) — the consequence of sub-package adapters; documented as "implemented by sub-packages; not called by consumers." (Revised Decision 5.)
- **Adapter does one round-trip; orchestrator owns the loop.** The adapter only translates `Request`→wire, opens the stream, parses, **assembles partial tool-call JSON centrally** (absorbing fragment-vs-whole), and exposes deltas + assembled `Message` + metadata. No tool loop, history, or `ToolResult` in the adapter.
- **Streamed tool-call input is assembled from the argument fragments alone.** For providers that stream tool arguments incrementally (Anthropic `input_json_delta`, the OpenAI-family `arguments` deltas), the opening frame (`content_block_start` / its equivalent) carries only an **empty `{}` placeholder**, not real arguments. That placeholder must **not** be prepended to the fragment buffer: doing so concatenates to invalid JSON such as `{}{"command":"…"}`, which fails the assembled-input parse and surfaces as `ErrInvalidRequest: invalid … tool input JSON` on the *first* tool call of a turn. The assembled `Input` equals exactly the concatenation of the streamed argument fragments; an absent or empty fragment stream assembles to `{}`.
- **Loop continuation keys off the assembled `Message` containing `ToolUseBlock`s**; `FinishReason` is carried for diagnostics and to map `FinishContentFilter`→`ErrContentFilter`.
- **`RoundTrip.Events()` yields only `TextDelta`/`ReasoningDelta`**; `ToolUse`/`ToolResult`/`MessageDone` are emitted by the orchestrator — keeping transparency logic in one place.
- **Reasoning-block drop-on-switch is localized in the adapter**: each adapter emits back only `ReasoningBlock`s whose `Opaque` is its own format and drops foreign ones — a switch sheds prior reasoning with no origin-tag on the block.
- **Each adapter lowers the native `ReasoningValue` to its own wire fields, keyed on the model's `spec.Kind` (D6/D16) — no shared `applyReasoning`, no cross-model normalization.** The wire shapes do not unify, so the lowering is per-provider, and a value whose kind the model doesn't accept warns+defaults at the build-time choke point (D6). The full table:

  | Provider / family | `Level(s)` → | `Budget(n)` → | `DisableReasoning()` → | unset → |
  |---|---|---|---|---|
  | **Anthropic** opus-4-8 / sonnet-4-6 | `output_config.effort=s` **and** enable thinking (`thinking:{type:"adaptive"}`) | wrong kind → warn+default | `thinking:{type:"disabled"}` | omit (default: thinking off) |
  | **Anthropic** haiku-4-5 | wrong kind → warn+default | `thinking:{type:"enabled",budget_tokens:n}` | `thinking:{type:"disabled"}` | omit (default: off) |
  | **OpenAI** gpt-5.x | `reasoning.effort=s` | wrong kind → warn+default | `reasoning.effort:"none"` (gpt-5.5-pro can't → warn) | omit |
  | **Google** 2.5 flash/pro | wrong kind → warn+default | `thinkingConfig.thinkingBudget=n` | `thinkingBudget:0` (pro can't, min 128 → warn) | omit (default: dynamic) |
  | **Google** 3.x flash / flash-lite / pro-preview | `thinkingConfig.thinkingLevel=s` | wrong kind → warn+default | 3.x can't disable → warn | omit |
  | **Z.ai/GLM** 5.x | `thinking:{type:"enabled"}` + `reasoning_effort=s` | wrong kind → warn+default | `thinking:{type:"disabled"}` | omit (default: enabled) |
  | **Z.ai/GLM** 4.6/4.7 (toggle) | no levels → warn+default | wrong kind → warn+default | `thinking:{type:"disabled"}` | omit (default: enabled) |

  Three load-bearing rules: (1) **Anthropic is two-axis — a `Level` also enables thinking** (`type:"adaptive"`); the consumer's single `ReasoningValue` means "think this hard," so an effort level implies thinking-on, and `DisableReasoning()` is the only path to the off axis (this also makes §7.2 thinking-block preservation engage exactly when reasoning is on). (2) **Gemini never sends both `thinkingBudget` and `thinkingLevel`** (400) — `spec.Kind` selects exactly one field per model; the other kind warns. (3) **The off-form is per-model** (`type:"disabled"` / `effort:"none"` / `thinkingBudget:0`), which is why `DisableReasoning()` is a distinct intent and a can't-disable model warns instead.
- **OpenAI Responses reasoning replay is mandatory, fixed adapter behavior.** The `openai` adapter sets `store:false` and injects `include:["reasoning.encrypted_content"]` on **every** request — never a consumer knob. Without `include`, OpenAI returns no `encrypted_content`, so the stateless multi-turn tool loop (history resent every round-trip) loses its reasoning chain ("reasoning item not found" / silent degradation on the v1 reasoning-model targets). The returned `encrypted_content` is what populates `ReasoningBlock.Opaque` for OpenAI; the resend-history model replays it verbatim. `store:false` is inseparable from `include` here — the ZDR path only returns `encrypted_content` when not server-stored — and it also keeps the adapter stateless/symmetric (no `previous_response_id`, per Decision 5). The other three reasoning providers likewise capture a non-empty `Opaque` from their own echo field (Anthropic `signature`, Gemini `thoughtSignature`, Z.ai `reasoning_content`).
- **A replayed OpenAI reasoning item must always carry `summary`, even empty.** OpenAI's Responses API requires every `reasoning` **input** item to include a `summary` array — it may be empty (`[]`) but must be **present**. The adapter does not request summaries (it sends only `reasoning.effort`, never `reasoning.summary:"auto"`), so `ReasoningBlock.Summary` is almost always `""`; the serialized reasoning item must therefore still emit `"summary": []` rather than omit the field. Omitting it — the trap being a struct tag's `omitempty`, which drops a nil/empty slice — makes OpenAI reject the request with `400 Missing required parameter: 'input[N].summary'`. The failure surfaces on the **second** turn of any reasoning conversation (the first turn to replay a prior reasoning item, e.g. the replayed item lands at `input[1]`), not the first, so it is invisible to single-turn tests. The serialization is exactly: `[{"type":"summary_text","text":<Summary>}]` when `Summary` is non-empty, `[]` otherwise — and the empty-`[]` case must survive on the wire. This is independent of any future `summary:"auto"` request knob: even with auto-summaries some reasoning items return empty and must still replay `summary:[]`.
- **A replayed tool call serializes `arguments` as a JSON string, not a nested object (OpenAI Responses).** The Responses `function_call` **input** item's `arguments` field is a **string** carrying the JSON text — exactly the form the model emits on output, where `arguments` accumulates as a string and the adapter's *parse* side already types it `string`. The canonical `ToolUseBlock.Input` is a `json.RawMessage` (a JSON value), so the adapter must emit it as `string(Input)` (`"{\"path\":\"PING\"}"`), never as the raw object (`{"path":"PING"}`). The trap is typing the outbound field `json.RawMessage`, which marshals verbatim as an object; it must be a `string`. Sending the object trips `400 Invalid type for 'input[N].arguments': expected a string, but got an object instead`. Like the `summary` case, this bites only on **replay**: the model's own tool call in the first round-trip succeeds, but the follow-up round-trip of the same turn — which resends the assistant `ToolUseBlock`s as `function_call` items to fetch the final answer — is the first to serialize `arguments` outbound (the replayed call lands at `input[1]`). It was previously masked by the `summary` 400, which failed validation earlier in the same request. **The same object-vs-string defect exists independently in the shared `internal/openaicompat` Chat-Completions path** that backs `zai` (and any future compat provider): there the assistant `tool_calls[].function.arguments` field is likewise typed `json.RawMessage` and set from `ToolUseBlock.Input`, so it serializes as an object while the parse side (`toolFunctionDelta.Arguments`) already types it `string` — the identical outbound/inbound asymmetry. It is masked on Z.ai's default base URL (`api/paas/v4`, which tolerates an object) and only surfaces once `zai.base_url` points at the strict coding endpoint (`api/coding/paas/v4`), which rejects it with `400 Invalid API parameter (type=1210)` on the same replay round-trip. Both adapters need the outbound field retyped `string` and set to `string(Input)`. Anthropic/Gemini take structured tool input directly and are unaffected.
- **A Gemini `thoughtSignature` is part-level replay metadata, not a reasoning-content marker — it must never suppress the visible text or `functionCall` riding on the same part.** Gemini attaches `thoughtSignature` to the part that carries the model's output for a step: a thought part (`thought:true`), a visible-text part, *or* the `functionCall` part itself (and, with parallel calls, each `functionCall` part). The Google response parse must therefore decide "is this reasoning content?" from the **`thought:true` flag alone**, never from the mere presence of `thoughtSignature`. Treating any signature-bearing part as reasoning-only and skipping the rest of it has two failure modes: it **drops the part's `functionCall`** (no `ToolUseBlock` is assembled, `Finish` stays `FinishStop`, the tool loop never runs, and the turn ends with an empty assistant message — only a text-less reasoning block), and it **reclassifies a real text answer as a `ReasoningDelta`** (the answer renders as a thinking summary instead of assistant text and never lands in `History` as a `TextBlock`, so context also degrades each turn). Both were observed via agentrepl on `gemini-2.5-flash`: a clean first text turn, then every tool-eliciting or answer turn produces only token usage and the message-boundary separator, with no visible output and no tool call. End state: a `thoughtSignature` is captured into `ReasoningBlock.Opaque` (bound to the co-resident or following `ToolUseBlock` via `BoundToID`, the positional rule of R-IPGC-I69W) **independently** of — and without consuming — the same part's text or `functionCall`, which are still emitted as `TextDelta`/`TextBlock` and `ToolUseBlock` respectively; `thought:true` parts remain the only source of `ReasoningDelta`/reasoning summary. This is the Gemini analogue of the OpenAI replay-shape traps above — a parse asymmetry invisible to any fixture that places the signature on a standalone thought part rather than on the live `functionCall`/text part.
- **A replayed Gemini `thoughtSignature` is a sibling field of the `Part`, not a field of `functionCall`.** Gemini's wire schema places `thoughtSignature` at the **part** level — alongside `text`/`functionCall`/`functionResponse` — never inside the `functionCall` object (whose only fields are `name`, `args`, and `id`). The adapter's own *parse* side already models this correctly (`thoughtSignature` lives on the `part` struct, sibling of `functionCall`), but the *request build* (`partsFromMessage`) nests the replayed signature **inside** the `functionCall` map (`call["thoughtSignature"] = sig`) rather than on the part wrapper that holds it. Gemini rejects the request with `400 INVALID_ARGUMENT: Invalid JSON payload received. Unknown name "thoughtSignature" at 'contents[N].parts[0].function_call': Cannot find field.` This is the **outbound** sibling of the inbound parse defect above and shares its outbound/inbound asymmetry shape with the OpenAI/`openaicompat` `arguments` traps (Phases 17–18): the parse side is right, the build side is wrong. It is **unmasked by the Phase 19 parse fix** — before that fix the signature on a live `functionCall` part was dropped on parse, so no `ReasoningBlock.Opaque` existed to replay and `partsFromMessage` never added a `thoughtSignature` at all; once Phase 19 began capturing the signature into `Opaque` (bound to its `ToolUseBlock`), the next round-trip became the first to serialize it outbound, in the wrong location. Like the OpenAI replay traps it bites only on **replay**: the model's own first tool-call round-trip succeeds (nothing replayed yet), and the follow-up round-trip that resends the assistant `ToolUseBlock` — carrying its bound signature — is the first to ship a misplaced `thoughtSignature` (observed at `contents[3]`, the second assistant turn, via agentrepl on `gemini-2.5-flash`). End state: the replayed signature is emitted as a **part-level** key (`{"functionCall":{…}, "thoughtSignature":<sig>}`), the `functionCall` object carrying only `name`/`args`/`id`, mirroring the parse-side `part` shape. Anthropic/OpenAI/Z.ai are unaffected (no part-level signature field).
- **A replayed Anthropic thinking block serializes its reasoning text in the `thinking` field, not `text`.** Anthropic's Messages API requires every assistant `thinking` **input** block to carry a `thinking` string — the reasoning text the `signature` signs — shaped `{"type":"thinking","thinking":<text>,"signature":<sig>}`. The `anthropic` adapter's request build (`convertMessage`) lowers a replayed `ReasoningBlock` to `wireBlock{Type:"thinking", Text: b.Summary, Signature: sig}`, placing the reasoning text in the block's `text` field — and `wireBlock` has **no** `thinking` field at all — so the block ships as `{"type":"thinking","text":<text>,"signature":<sig>}`, missing the required `thinking` field. Anthropic rejects the request with `400 messages.N.content.0.thinking.thinking: Field required`. Like the OpenAI/Gemini replay traps above this bites only on **replay**: the first turn has no prior thinking block to resend and succeeds; the **second** turn of any reasoning-enabled Anthropic conversation is the first to serialize a `thinking` input block (the replayed block lands at `messages[1].content[0]`, the reasoning block emitted before the assistant's text), and fails. Because an Anthropic `Level` also enables thinking (`type:"adaptive"`, the two-axis rule above) and a `Budget` enables it directly, the trap fires for every reasoning-on Anthropic model — opus-4-8 / sonnet-4-6 and haiku-4-5 alike. Observed via agentrepl on `claude-haiku-4-5` with `-c thinking_budget=2048`: a clean first reply, then `error › anthropic: messages.1.content.0.thinking.thinking: Field required (status=400 type=invalid_request_error)` on the next turn. The `signature` itself round-trips correctly (captured into `ReasoningBlock.Opaque`, R-IN0J-QMSI / R-XW08-D4YL); the gap is purely the outbound **field name** the reasoning text lands in, which no existing wire-shape test asserts — the byte-for-byte `Opaque` round-trip checks the signature, not the JSON field the summary serializes into. This is the Anthropic sibling of the OpenAI summary/`arguments` and Gemini `thoughtSignature` replay traps: the parse side is unaffected (Anthropic streams the `thinking` summary in via `thinking_delta`, not the `text` field), the build side is wrong. End state: `wireBlock` gains a `thinking` field (`Thinking string \`json:"thinking,omitempty"\``) and the `ReasoningBlock` case sets `Thinking: b.Summary` rather than `Text: b.Summary`, emitting `{"type":"thinking","thinking":<text>,"signature":<sig>}`. OpenAI/Gemini/Z.ai are unaffected (their reasoning replay uses their own input-item shapes).

**Rejected.**
- *Adapters in the root package* — compiles every provider's deps into every consumer; loses Decision 5 isolation.
- *A separate `agentkit/driver` package for neutral types* — would split or duplicate `Message`/`Block`; one shared root type is simpler since both sides use identical neutral types.
- *Adapter returns the consumer `*Stream`* — conflates one round-trip with a whole (multi-round-trip) turn; `*RoundTrip` is the right granularity.
- *Tagging `ReasoningBlock` with its origin provider* — unnecessary; format-recognition achieves the drop without a consumer-visible field.

**Verification.**
- R-01HL-I6TM — a build importing only `agentkit` + `agentkit/anthropic` does not compile in the Google or OpenAI SDK/dependencies (dependency isolation holds).
- R-02PH-VYKB — a `RoundTrip` whose assembled `Message` contains a `ToolUseBlock` drives the orchestrator to continue the loop; one without ends the turn.
- R-OUE3-L8VS — a streamed `tool_use` whose opening frame carries an empty `{}` input placeholder followed by argument-fragment deltas assembles `ToolUseBlock.Input` to exactly the concatenated fragments (valid JSON), never the placeholder-prepended `{}{…}` form — covered at minimum by the Anthropic adapter (`content_block_start` `input:{}` then `input_json_delta`), and no round-trip carrying tool arguments fails with `ErrInvalidRequest` for placeholder-concatenated JSON.
- R-03XE-9QB0 — a round-trip ending in the provider's content-filter signal yields `Finish()==FinishContentFilter`, surfaced by the orchestrator as `ErrContentFilter`.
- R-055A-NI1P — an adapter given a `Request` whose history contains a `ReasoningBlock` in a *foreign* provider's `Opaque` format drops it from the wire request rather than sending or erroring on it.
- R-XR4M-U1ZT — every request the `openai` (Responses) adapter emits carries `store:false` and `include:["reasoning.encrypted_content"]`, and a reasoning+tool turn captures a non-empty `ReasoningBlock.Opaque` from the returned `encrypted_content` and replays it on the next round-trip.
- R-OMKB-AY19 — a replayed OpenAI reasoning input item always serializes the `summary` field: `[]` when `ReasoningBlock.Summary` is empty (the field is present on the wire, never omitted) and `[{"type":"summary_text","text":<Summary>}]` when non-empty. A second `Send` that resends a prior **empty-summary** reasoning item produces a request body whose reasoning item contains `"summary":[]` and does not fail with `400 Missing required parameter: 'input[N].summary'`. Covered by a golden/replay test whose first turn yields a reasoning item with no summary, asserting the empty-`[]` case on the wire (the single-turn `R-XR4M-U1ZT` and the summary-present `R-XW08-D4YL`/replay fixtures do not exercise it).
- R-UJNS-PFLL — a replayed `function_call` input item the `openai` adapter emits serializes `arguments` as a JSON **string** (e.g. `"{\"path\":\"PING\"}"`) equal to the canonical `ToolUseBlock.Input` bytes, not as a nested JSON object, so a follow-up round-trip that resends a prior `ToolUseBlock` does not fail with `400 Invalid type for 'input[N].arguments': expected a string, but got an object instead`. Covered by a replay test exercising a multi-tool turn's follow-up round-trip and asserting the request body's `function_call.arguments` is a JSON string (the round-trip assembly `R-C8UE-VJ67`/`R-OUE3-L8VS` exercise the inbound parse, not the outbound replay shape).
- R-ZCMP-ARG8 — a replayed assistant tool call the shared `internal/openaicompat` Chat-Completions adapter emits (and thus the `zai` adapter over it) serializes `tool_calls[].function.arguments` as a JSON **string** equal to the canonical `ToolUseBlock.Input` bytes, not as a nested JSON object, so a follow-up round-trip that resends a prior `ToolUseBlock` does not fail with `400 Invalid API parameter (type=1210)` against Z.ai's strict coding endpoint. Covered by a replay/wire test asserting the second request body's assistant `function.arguments` is a JSON string (the existing `zai` tool-turn test asserts the *presence* of the replayed tool output and reasoning but not the *type* of `arguments`, which is the gap that let this through).
- R-DNS8-QC6Z — the Google adapter, given a response part that carries **both** a `functionCall` and a `thoughtSignature`, assembles the `ToolUseBlock` and reports `FinishToolUse` so the tool loop continues, rather than swallowing the part as reasoning-only and ending the turn with no tool use. Covered by a golden-SSE replay whose `functionCall` part carries a `thoughtSignature` (the existing R-C8UE-VJ67 / R-IPGC-I69W fixtures place the signature on a *separate* thought part preceding the call, which is the gap that let this through).
- R-DRFX-VNF2 — the Google adapter, given a visible-text part that also carries a `thoughtSignature`, emits that text as a `TextDelta` and a `TextBlock` in the assembled `Message` (assistant-visible, persisted to `History`), not as a `ReasoningDelta`/reasoning-only part; only a `thought:true` part produces a reasoning summary.
- R-DTVQ-N6WG — a `thoughtSignature` riding on a `functionCall` (or visible-text) part is still captured into a non-empty `ReasoningBlock.Opaque` bound to the relevant `ToolUseBlock` via `BoundToID`, so the byte-for-byte replay guarantee (R-IN0J-QMSI / R-XW08-D4YL) holds without the signature consuming the part's tool call or text.
- R-GSIG-PT07 — a request the Google adapter builds for a history whose assistant `ToolUseBlock` carries a bound (Phase-19-captured) `thoughtSignature` serializes that signature as a **part-level** sibling of `functionCall` (`{"functionCall":{"name":…,"args":…,"id":…}, "thoughtSignature":<sig>}`), never nested inside the `functionCall` object, so a follow-up round-trip that replays the prior tool call does not fail with `400 INVALID_ARGUMENT: Unknown name "thoughtSignature" at 'contents[N].parts[0].function_call'`. Covered by a wire/replay test asserting the rebuilt request body places `thoughtSignature` on the part and that `functionCall` carries only `name`/`args`/`id` (the parse-side R-DTVQ-N6WG exercises capture into `Opaque`, not the outbound replay placement).
- R-TQ77-6QLK — a request the `anthropic` adapter builds for a history whose assistant message carries a `ReasoningBlock` serializes that block as `{"type":"thinking","thinking":<Summary>,"signature":<sig>}` — the reasoning text in a `thinking` field (never `text`) with the `signature` present — so a follow-up turn that replays a prior thinking block does not fail with `400 messages.N.content.0.thinking.thinking: Field required`. Covered by a wire/replay test asserting the rebuilt request body's thinking block carries a non-empty `thinking` field and no `text` field for that block (the existing `Opaque` round-trips R-IN0J-QMSI / R-XW08-D4YL assert the signature survives, not the field the summary serializes into, which is the gap that let this through).
- R-ELUQ-VJIQ — each adapter lowers a native `ReasoningValue` to the correct wire field(s) per the lowering table: a `Level` on Anthropic opus/sonnet emits `output_config.effort` **and** enables thinking; a `Budget` on haiku-4-5 / gemini-2.5 emits the token-budget field; a `Level` on gpt-5.x / gemini-3.x / glm-5.x emits the effort/level field; a Gemini request never carries both `thinkingBudget` and `thinkingLevel`; and a value whose kind the model does not accept is replaced by the default and warned (R-B7YX-J342) rather than sent.

## Decision 10 — The orchestration layer: tool loop, history, transparency, reasoning replay, cache-prefix stability

**Decision.** Everything above the SPI lives in the root package behind `Send`. The turn algorithm:

1. Record `len(History)` as the rollback point, then append the user message to `History` (committed only if the turn completes — see atomicity below).
2. Build a `Request` from the `Conversation` (System, Messages=History, Tools, Gen, Model), with **tools serialized in stable deterministic order** (sorted by name, deterministic JSON) for cache stability.
3. Loop:
   - `rt := provider.RoundTrip(ctx, req)`.
   - Forward each `TextDelta`/`ReasoningDelta` from `rt.Events()` to the consumer `Stream` as it arrives.
   - On drain: take `rt.Message()`, emit `MessageDone`, append it to `History`, accumulate `rt.Usage()`, collect `rt.Warnings()`. A round-trip error terminates the turn (surfaced via `Stream.Err()`).
   - If the message has no `ToolUseBlock`s → done.
   - Otherwise, for each `ToolUseBlock`: emit `ToolUse`, run the matching registered tool, build a `ToolResultBlock`, emit `ToolResult`. Collect all results into one user message, append to `History`, rebuild `Request`, loop.

```go
// On Conversation:
MaxToolIterations int // 0 → default 1000 (a runaway backstop, not an interactive limit)

// Orchestration sentinels, surfaced via Stream.Err(), matchable with errors.Is.
// Returned bare (not wrapped in *Error — they are not provider errors).
var ErrToolLoopLimit = errors.New("agentkit: tool-loop iteration limit exceeded")
var ErrStreamPending = errors.New("agentkit: prior stream not yet drained")
```

Settled choices:
- **Auto-tool-loop and history accumulation live here** (research §4.5), above the stateless adapters. History grows append-only: `[…, user, assistant(tool_use), user(tool_results), assistant(final)]`.
- **Full transparency** = `ToolUse`+`ToolResult` events around each tool execution, `MessageDone` per assistant message, plus live deltas — nothing filtered.
- **Tools run sequentially** in call order; results go into one user message in that order. Concurrent execution of parallel calls is deferred (determinism + simpler consumer code).
- **Unknown tool name → `ToolResultBlock{IsError: true}` fed back**, not fatal — consistent with Decision 4.
- **Bounded loop.** `MaxToolIterations` (settable in config; `0` → default **1000**, sized for extended automated workflows while still finite) caps the loop; exceeding it ends the turn with `ErrToolLoopLimit` (fail loudly).
- **Reasoning replay across the loop is automatic**: each assistant `Message` (with its `ReasoningBlock`s) is appended to `History`, and the full `History` is resent every round-trip, so prior reasoning replays verbatim within the turn; the adapter re-serializes its own `Opaque` (Decision 9).
- **MCP-discovered tools join the same registry and the same name-sorted order** (Decision 17). After prefixing/sanitization, MCP tools are merged with custom tools into the one `[]Tool` the loop drives and serialized in the same deterministic name-sorted order (R-VXPR-861V) — they are indistinguishable to the loop and adapters. A tool-set change from attaching/detaching a server is a deliberate cache-invalidation event (the prefix's tool array changes), in the same cost class as a model switch; v1 does not honor mid-conversation `tools/list_changed`, so a stable attached set keeps a stable prefix (research line 444).
- **Cache-prefix stability is an orchestration invariant** (research §8): frozen system (no `now()`/UUID interpolation), deterministic sorted tools never reordered/mutated mid-conversation, append-only history. The **stable prefix** is the precise object this invariant protects: the leading run of request blocks that is byte-identical across successive round-trips — in request order, the frozen `System` block(s), then the name-sorted tool definitions, then the already-committed `History` messages — i.e. everything ahead of the round's newly-appended content. Each round-trip only *appends* (tool-use/tool-result, then the next user turn), so the prefix only grows; it is never reordered or rewritten. The **default Anthropic cache breakpoint** is set by the Anthropic adapter (internal) on the last block of that prefix.
- **A turn is atomic with respect to `History`.** The user message and every message the turn produces are committed only if the turn runs to successful completion (a tool-free assistant message). If the turn fails, is cancelled (`ctx`), hits `MaxToolIterations`, or is abandoned by breaking out of `Events()` early, `History` is truncated back to the step-1 rollback point — left exactly as it was before `Send`. So `History` is always a sequence of complete turns; a dangling user message (or an assistant message with no matching turn) can never occur. Rollback is a cheap length-truncation because `History` is append-only.
- **`Send` is non-re-entrant; re-entrancy is guarded.** A `Send` issued while a prior `Stream` is still live (not yet fully drained or broken-with-cleanup) returns immediately with a terminal-error `Stream` carrying `ErrStreamPending` and mutates nothing. The `Conversation` tracks one "stream live" flag, set when `Send` returns a `Stream` and cleared when that `Stream` is fully consumed or its early-break cleanup runs (Decision 2, R-CCI4-0UEA). This is the sequential-re-entrancy axis; the multi-goroutine axis remains "documented, not enforced" (Conventions).

**Rejected.**
- *Tool loop inside the adapter* — would duplicate it four times and entangle it with wire formats; it belongs once, above the SPI.
- *Unbounded loop* — a stuck model would hang forever; the cap fails loudly.
- *Concurrent tool execution in v1* — non-deterministic ordering and goroutine-safety assumptions on consumer code; deferred.
- *Fatal error on unknown tool* — denies the model a recovery path; in-band error result instead.
- *A consumer-facing caching API* — research §8 defers it; v1 is prefix stability + usage reporting + the internal default Anthropic breakpoint.
- *Reusing `ErrInvalidRequest` for the loop cap* — a distinct, consumer-actionable condition deserves its own sentinel.

**Verification.**
- R-VV9Y-GMKH — a turn with no tool use appends the user message and exactly one assistant message to `History`, emits one `MessageDone`, and runs the round-trip loop once.
- R-VWHU-UEB6 — a turn where the model requests a tool runs the matching registered tool, appends `assistant(tool_use)` then `user(tool_result)` to `History`, emits `ToolUse` then `ToolResult`, and continues until a tool-free assistant message, returning the final result.
- R-VXPR-861V — tools are serialized into the `Request` in name-sorted, deterministic-JSON order, identical across successive turns of a conversation.
- R-VYXN-LXSK — an unknown tool name requested by the model yields a `ToolResultBlock{IsError: true}` fed back and the turn continues (not fatal).
- R-W05J-ZPJ9 — a turn exceeding `MaxToolIterations` ends with `ErrToolLoopLimit` (matchable via `errors.Is`) rather than looping indefinitely; the configured value overrides the default.
- R-W1DG-DH9Y — a `ReasoningBlock` produced in an earlier round-trip of a turn is present in `History` and re-sent on subsequent round-trips of that same turn.
- R-W2LC-R90N — when Anthropic is selected and the stable prefix (`System` → name-sorted tools → committed `History`, as defined in the settled choices) meets the per-model token minimum, the Anthropic adapter sets exactly one default 5m `cache_control` breakpoint on the last block of that prefix.
- R-XZNX-IG6O — a second `Send` issued before the prior `Stream` is drained (or broken-with-cleanup) returns `ErrStreamPending` (matchable via `errors.Is`), leaves `History` unchanged, and issues no provider call.
- R-Y4JJ-1J5G — a turn that errors, is cancelled, hits `MaxToolIterations`, or is abandoned by an early `break` leaves `History` identical to its pre-`Send` state (atomic rollback to the step-1 length); a successfully completed turn commits the full user/assistant/tool message sequence.
- R-6W63-I4FW — MCP-discovered tools are merged with custom tools and serialized into the `Request` in the one deterministic name-sorted order, stable across turns while the attached server set is unchanged, and re-ordered deterministically when a server is attached/detached.

## Decision 11 — Retry & backoff policy

**Decision.** AgentKit owns one cross-provider retry policy, executed in the orchestrator around each `RoundTrip`, configured on `Conversation.Retry`. Wrapped SDKs have their built-in retry disabled so this is the single policy.

```go
// RetryPolicy controls automatic retrying of transient failures. The zero
// value uses defaults. Set on Conversation.Retry. The budget below is per
// round-trip (one model call), not per turn — a turn's many tool round-trips
// each get their own budget; overall turn wall-clock is the consumer's ctx.
type RetryPolicy struct {
    MaxAttempts      int           // total attempts incl. the first; 0 → default 4
    BaseDelay        time.Duration // 0 → default (e.g. 500ms)
    MaxDelay         time.Duration // backoff cap; 0 → default (e.g. 30s)
    MaxElapsed       time.Duration // overall budget across attempts; 0 → no cap
    IgnoreRetryAfter bool          // default false → honor server Retry-After / RetryInfo
}

// On Conversation:
Retry RetryPolicy
```

Settled choices:
- **Retry lives in the orchestrator, around `RoundTrip`** — uniform across all four providers; wrapped SDKs' built-in retry disabled (Google's doesn't retry anyway).
- **Retryable set is fixed** to Decision 7 categories: `ErrRateLimited`, `ErrOverloaded`, `ErrServerError`, `ErrTimeout`, `ErrNetwork`. Everything else is never retried.
- **No-retry-after-first-byte rule, scoped per round-trip** (the streaming-idempotency rule): eligibility is tracked per `RoundTrip`, not per turn. Each round-trip is independently retryable until *it* forwards its own first event to the consumer `Stream`; a failure before the current round-trip's first delivered byte is eligible (subject to category), **regardless of whether earlier round-trips in the same turn already delivered events** — retrying re-issues only the current round-trip's call (same `Request`, idempotent) and re-streams it from scratch, so the consumer sees no double-delivery. Once any byte of the current round-trip reaches the consumer, that round-trip — and thus the turn — is non-retryable.
- **Retry budget is per round-trip.** `MaxAttempts`/`MaxDelay`/`MaxElapsed` bound the retries of a single model call, not the whole turn — a long agentic turn legitimately makes many round-trips, each with its own budget. The overall turn wall-clock is bounded by the consumer's `context.Context` deadline (already respected by backoff waits), not by the retry policy: retry budget and turn timeout stay separate concerns.
- **Server signal first, then backoff.** Honor `Error.RetryAfter` when set (adapter-extracted per D7); otherwise full-jitter exponential backoff capped at `MaxDelay`, bounded overall by `MaxElapsed`. `IgnoreRetryAfter` disables honoring.
- **Context-aware** — backoff waits respect `ctx`; cancellation ends retrying with the context error.
- **Injectable clock** (unexported, package-internal) for tests to assert attempt counts and delays without real sleeps.
- **Configurable vs fixed**: configurable = attempts, base/cap/elapsed delays (per round-trip), honor-Retry-After toggle. Fixed = the full-jitter algorithm, the retryable/non-retryable lists, the per-round-trip scoping of the budget, and the per-round-trip no-retry-after-first-byte rule.
- **MCP discovery is retried; MCP `tools/call` is not** (Decision 17, research line 440). `initialize` and `tools/list` are idempotent/read-only, so they retry under this same policy (network/timeout/5xx/429 → full-jitter backoff) but fail-fast on `401`/`403`/`400` and non-MCP `4xx`. A `tools/call` is treated as a non-idempotent POST: a transport-level failure (`ErrNetwork`/`ErrTimeout`/`ErrServerError`/`429`) is surfaced to the caller **without** automatic retry (the model may re-issue), and the no-retry-after-first-byte rule applies to a tool-result SSE stream too.

**Rejected.**
- *Retry inside each adapter* — four divergent policies; must be uniform and own the first-byte rule centrally.
- *Leaning on SDKs' built-in retry* — three different policies, none aware of the streaming-idempotency rule.
- *Retrying mid-stream by buffering/replaying* — can't un-deliver events already given to the consumer.
- *Equal-/no-jitter backoff* — full jitter is the researched choice.

**Verification.**
- R-P3LQ-QY2X — a retryable-category failure before any event is delivered is retried up to `MaxAttempts`, then surfaces the final error; with an injected clock the attempt count and honored delays are asserted.
- R-P4TN-4PTM — a non-retryable-category failure is never retried (asserted across the non-retryable categories).
- R-P61J-IHKB — a failure after the first event has been delivered to the consumer is never retried, regardless of category (streaming-idempotency rule).
- R-Y878-6UDJ — a retryable-category failure on a *later* round-trip of a multi-round-trip turn, occurring before that round-trip delivers any event, is still retried (per-round-trip scope), while a failure after any round-trip has delivered an event is not; the retry budget applies per round-trip, not per turn.
- R-P79F-W9B0 — when `Error.RetryAfter` is present it is honored as the delay; when absent, full-jitter exponential backoff capped at `MaxDelay` is used; `IgnoreRetryAfter=true` ignores the server delay.
- R-P8HC-A11P — context cancellation during a backoff wait ends retrying promptly with the context error.
- R-6XDZ-VW6L — MCP discovery (`initialize`/`tools/list`) retries transient transport failures under this policy but fails fast on `401`/`403`/`400` and non-MCP `4xx`.
- R-6YLW-9NXA — an MCP `tools/call` transport failure is surfaced without any automatic retry (non-idempotent), and once a byte of a tool-result SSE stream is delivered it is never retried.

## Decision 12 — Raw HTTP, not wrapped SDKs

**Decision.** Every adapter talks to `net/http` directly; no official provider SDK is wrapped. Shared internal helpers absorb the common parts:

```
internal/httpx — request execution, header/body helpers, *http.Client injection
internal/sse   — SSE framing/parsing (shared by Anthropic / OpenAI / Z.ai)
internal/openaicompat — shared OpenAI Chat-Completions adapter; the public `zai` package (and any future compat-backed provider) constructs it with its own baked-in base URL + model registry. Not consumer-importable.
internal/mcp   — raw-HTTP Streamable-HTTP MCP client (Decision 17): the 4 calls, the dual application/json-vs-text/event-stream response path, session/version headers. Reuses internal/sse. Not consumer-importable.
```

(Both importable by the provider sub-packages, since `internal/` under the module root is.) Each adapter owns its request build, SSE/iterator parse, central partial-JSON tool assembly, error classification, and usage mapping. The `zai` adapter (over shared `internal/openaicompat`) already must be raw (no Z.ai Go SDK); the other three follow for one model.

Settled choices / rationale:
- **The design already owns the SDKs' value-adds, or disables them** — retry (D11, with the no-retry-after-first-byte rule), error taxonomy + raw-body capture (D7), usage normalization (D8), central tool-JSON assembly (D9). Wrapping would re-translate SDK types into ours while switching off their retry — little gained.
- **Dependency isolation (D5) becomes real** — each sub-package adds essentially `net/http`, not a churning vendor tree.
- **Consistency** — one mental model and shared `internal/sse`/`internal/httpx` across all four, rather than wrapped-and-raw mixed.
- **Testability (enables D13)** — injected `*http.Client`/base URL against `httptest` + golden SSE fixtures gives exact, credit-free coverage.
- **Aligns with principles** — simplicity through discipline, minimal deps, own your invariants. Matches the serious neutral gateways (gollm, langchaingo, bifrost, LiteLLM).
- **Accepted cost** — hand-written SSE parsing, partial-JSON accumulation, and retry-signal extraction per provider, and tracking provider API changes ourselves; judged bounded, with `internal/*` absorbing the shared parts.

**Rejected.**
- *Wrap all three official SDKs* — three heavy, independently-versioned deps; forces disabling their retry; we still translate their types and re-derive usage.
- *Hybrid (wrap some, raw others)* — mixes two dependency and testing models; the central-assembly design (D9) already does uniformly what the SDKs would buy.

**Verification.** Structural/implementation-strategy decision with no consumer-observable behavior of its own; its proof is the behavioral ids of the adapters it implements (provider-conformance ids in D2/D7/D8/D9 and the dependency-isolation id R-01HL-I6TM). Carries **no ids of its own.**

## Decision 13 — Testing strategy

**Decision.** The suite runs offline, deterministically, and without API credits. Live API access is opt-in and isolated.

- **Fake-server harness.** Each provider `New` accepts `WithBaseURL(string)` and `WithHTTPClient(*http.Client)` — a supported D5 public construction option (not a test-only hook), which the harness also uses. Unit tests point an adapter at an `httptest.Server` replaying recorded fixtures, exercising real request build, JSON/SSE decode, error mapping, and usage math; the identical seam lets a consumer target an alternate/compatible endpoint.
- **Golden SSE replay.** Recorded raw byte streams under each adapter's `testdata/*.sse`; tests assert the assembled turn (events + final `Message`) and `Usage` against golden JSON, with a `-update` flag to regenerate.
- **Table-driven matrices.** Error classification (§6.1 → `Category`) and usage mapping (§6.3 → disjoint buckets) are table-driven per provider against fixtures — discharging the D7/D8 ids.
- **Retry tests with an injected clock.** Fake server returns 429/503 N times then 200; the D11 clock makes backoff instant; asserts attempt count, honored delay, and that mid-stream failures are not retried.
- **Live integration tests, double-gated.** `//go:build integration` plus an env-presence skip (no key → `t.Skip`). Fixtures captured once in a recording mode against the live API, keys scrubbed, committed; unit tests replay them offline thereafter.
- **Fake MCP server (Decision 17).** The `internal/mcp` client is tested against an `httptest.Server` speaking Streamable-HTTP JSON-RPC, with fixtures covering `initialize`/`tools/list`/`tools/call`, the `application/json`-vs-`text/event-stream` response split, `isError:true` results, JSON-RPC `error` objects, and a `401`+`WWW-Authenticate` — the same offline/deterministic discipline as the provider adapters.

Settled choices:
- **Offline determinism is non-negotiable** — fixtures + httptest + injected clock; the live path is opt-in and gated twice.
- **The injected `*http.Client`/base URL is the single seam** every adapter uses for tests — and the same public D5 option a consumer uses to override an endpoint; no provider-specific scaffolding beyond fixtures.
- **One golden mechanism (`-update`)** across all adapters.

**Rejected.**
- *Mocking at the Go-type level* — bypasses the real JSON/SSE decode and error-mapping code, the bug-prone surface; the fake HTTP server exercises it for real.
- *Hitting live APIs in the default suite* — non-deterministic, costs credits, flaky; gated integration only.
- *`iter`-level fakes without recorded bytes* — golden raw SSE is the regression canary on provider wire changes.

**Verification.**
- R-WJLM-7QRP — an integration-tagged test is skipped (not failed) when its provider's credential env var is absent.
- R-WKTI-LIIE — an adapter given `WithBaseURL`/`WithHTTPClient` pointing at an `httptest.Server` routes its request there and decodes the served fixture (the harness works end-to-end for at least one provider).
- R-WM1E-ZA93 — re-running golden `-update` against unchanged fixtures produces no diff (golden output is deterministic).
- R-711P-17EO — the `internal/mcp` client, driven against a fake MCP `httptest.Server`, completes the handshake and the four calls offline, exercising both response content-types, an `isError` result, a JSON-RPC `error`, and a `401`+`WWW-Authenticate`.

## Decision 15 — Structured JSONL event log & conversation lifecycle

**Decision.** The consumer supplies an `io.Writer`; AgentKit writes one JSON object per line for each protocol event of a turn — a `codex exec --json`-style message stream. A `Close()` lifecycle method emits a final cumulative `summary` record.

```go
// On Conversation:
Log io.Writer                              // nil → no logging; one JSONL record per protocol event
func (c *Conversation) Close() error       // optional; emits a final "summary" record, marks closed. Idempotent.
func (c *Conversation) TotalUsage() Usage  // cumulative token usage across all turns so far

// Orchestration sentinel, surfaced via Stream.Err(), matchable with errors.Is.
var ErrClosed = errors.New("agentkit: conversation closed")

// LogRecord is one line of the log. Type discriminates the payload; Time comes
// from the injected clock (D11). One JSON object per line, in stream order.
type LogRecord struct {
    Type    string      `json:"type"`               // turn_start|message|tool_use|tool_result|usage|warning|error|retry|turn_end|summary
    Time    time.Time   `json:"time"`
    Seq     int         `json:"seq"`                // monotonic within the turn
    Message *Message    `json:"message,omitempty"`
    ToolUse *ToolUse    `json:"tool_use,omitempty"`
    Result  *ToolResult `json:"tool_result,omitempty"`
    Usage   *Usage      `json:"usage,omitempty"`    // per-turn on usage; cumulative on summary
    Warning *Warning    `json:"warning,omitempty"`
    Error   *Error      `json:"error,omitempty"`    // carries verbatim provider Raw body
    Turns   int         `json:"turns,omitempty"`    // summary only
    Cost    *Cost       `json:"cost,omitempty"`     // present on usage/summary records; always priced (Decision 16)
    // turn_start carries provider+model; turn_end carries final status.
}
```

Settled choices:
- **`Log io.Writer`, AgentKit owns the JSONL schema** — a stable, self-describing protocol, not arbitrary log lines. Nil disables it with zero overhead.
- **Message-granular, not token-granular** — one record per assembled message / tool call / result / usage / lifecycle event; token deltas stay on the live `Stream`.
- **Written from the orchestration layer (D10)** — the serialized projection of what the consumer also gets in-memory.
- **Timestamps from the injected clock (D11)**; `Seq` monotonic within the turn — deterministic under test.
- **Best-effort** — a write failure to `Log` never aborts the turn or becomes `Stream.Err()`.
- **`Close()` emits one cumulative `summary`** (`TotalUsage`, `Turns`, total `Cost`), marks closed, idempotent; `Send` after `Close` returns `ErrClosed` (fail loudly on a known-done state).
- **Cumulative usage** accumulates at conversation level (sum of per-turn `Stream.Usage()`), readable via `TotalUsage()`.

**Rejected.**
- *`*slog.Logger`* — handler-controlled schema, not a stable protocol; mixes diagnostics with the event trace. A consumer can still bridge JSONL into slog.
- *A callback `func(LogRecord)`* — `io.Writer` is the universal sink and matches "stream of jsonl."
- *Token-delta granularity in the log* — noisy and huge; the live `Stream` serves incremental needs.
- *Logging raw provider HTTP wire* — out of scope; protocol-level trace + `Error.Raw` suffices.
- *Lenient `Close` (no `ErrClosed`)* — a terminated conversation is a known-done state; reuse should fail loudly.

**Verification.**
- R-PH7W-BVH0 — with `Log` set, a turn writes valid JSONL (one JSON object per line, in stream order) covering `turn_start`, each assistant `message`, each `tool_use`/`tool_result`, `usage`, and `turn_end`.
- R-PIFS-PN7P — each record's `Time` comes from the injected clock (deterministic under test) and `Seq` is monotonic within the turn.
- R-PJNP-3EYE — warnings, errors, and retries each emit their own record type, and an `error` record carries the verbatim provider `Raw` body.
- R-PKVL-H6P3 — a failing `Log` writer does not abort the turn or change `Stream.Err()`.
- R-PM3H-UYFS — with `Log == nil`, no records are written.
- R-PNBE-8Q6H — `Close()` emits exactly one `summary` record carrying cumulative `Usage` and `Turns`; a second `Close()` emits nothing.
- R-POJA-MHX6 — `TotalUsage()` equals the sum of every turn's `Stream.Usage()` over the conversation.
- R-PPR7-09NV — `Send` after `Close` returns `ErrClosed` (matchable via `errors.Is`).

## Decision 16 — The model registry: pricing, cost, and reasoning introspection

**Decision.** AgentKit ships per-model data in each provider sub-package and computes dollar cost from the disjoint `Usage` buckets (D8). Pricing reaches the orchestrator through the SPI (D9 `Pricing`), so root imports no sub-package. Because the supported-model set is closed and curated (product), **every supported model is priced by construction** — there is no "unpriced" runtime state, so cost is always available. The same registry also carries each model's **native reasoning spec** (D6), exposed credential-blind so a consumer can render and accept exactly what each model supports without embedding provider knowledge.

**Single source of truth — the model registry.** Each provider sub-package holds one registry mapping a model id to its `Pricing` **and** its `ReasoningSpec`, co-located with the exported model constant. D5's "model validity is checked at the Send boundary by the adapter" resolves against *this same registry*: the set of models an adapter will run is, by construction, exactly the set it can price *and* the set whose reasoning vocabulary it can describe and validate. There is no second table to drift out of sync.

**Reasoning introspection (the native-vocabulary descriptor).** The descriptor types and a `ReasoningInspector` interface live in root `agentkit` (one shared shape for root and every sub-package); the per-model data lives in each provider sub-package, exposed as a **credential-blind package-level value** that implements the interface — pure registry reads, no `Provider` handle, no credentials, no network. A consumer (agentrepl) builds a `map[string]ReasoningInspector` keyed by provider name once, then renders `--help` and answers runtime `/get`/`/dump` uniformly with no provider-specific branching — the polymorphism the interface exists to provide.

```go
// package agentkit — the descriptor types + the introspection interface

type ReasoningKind int
const (
    ReasoningEnum   ReasoningKind = iota // discrete native level strings
    ReasoningRange                       // integer token budget in [Min,Max]
    ReasoningToggle                      // on/off only, no depth control (e.g. GLM 4.6/4.7)
)

// ReasoningSpec is the inspectable native-vocabulary descriptor for one model.
type ReasoningSpec struct {
    Term       string         // native label: "effort" | "thinking level" | "thinking budget"
    Kind       ReasoningKind
    Levels     []string       // Kind==ReasoningEnum: accepted native strings, in the model's own order
    Min, Max   int            // Kind==ReasoningRange: inclusive valid budget range
    Sentinels  []Sentinel     // Kind==ReasoningRange: magic ints with native meaning (0=off, -1=dynamic)
    Default    ReasoningValue // the model's default — what the warn-fallback path (D6) applies
    CanDisable bool
}
type Sentinel struct{ Value int; Meaning string } // e.g. {0,"off"}, {-1,"dynamic"}

// ReasoningInspector reads a provider's native reasoning vocabulary. Implemented
// by a credential-blind package-level value in each provider sub-package (see
// below) — NOT by the Provider handle, so introspection needs no credentials.
type ReasoningInspector interface {
    ReasoningSpec(model string) (ReasoningSpec, bool) // false if unknown / no reasoning control
    SupportedReasoning() map[string]ReasoningSpec     // every model's spec, for catalog rendering
}
```

```go
// package anthropic (and identically google, openai, zai): a credential-blind
// introspector value backed by the package registry — no handle, no network.
var Reasoning agentkit.ReasoningInspector
```

```go
// Pricing is one model's per-token rates, as one or more context-length tiers.
// Rates are nano-USD per token (1e-9 USD) — integer, so cost is exact with no
// float drift. Reasoning output bills at the Output rate.
type Pricing struct {
    // Tiers are ordered ascending by MinInputTokens; Tiers[0].MinInputTokens
    // is always 0 (the base tier). A turn is rated entirely at the highest
    // tier whose MinInputTokens <= the turn's total input tokens. Most models
    // have exactly one tier.
    Tiers []RateTier
}

// RateTier is the per-token rate set for one context band.
type RateTier struct {
    MinInputTokens int64 // inclusive lower bound on a turn's total input tokens
    InputUncached  int64
    CacheReadInput int64
    CacheWrite5m   int64
    CacheWrite1h   int64
    Output         int64 // reasoning output billed at this rate too
}

// Cost computes one turn's nano-USD cost: it selects the tier by the turn's
// total input tokens (InputUncached + CacheReadInput + CacheWriteInput), then
// rates every bucket at that tier. Centralizes tier selection + the math.
func (p Pricing) Cost(u Usage) Cost

// Cost is an amount in nano-USD. USD() converts only at display.
type Cost int64
func (c Cost) USD() float64

func (s *Stream) Cost() Cost            // this turn — always available
func (c *Conversation) TotalCost() Cost // cumulative over the conversation — always available
```

For the selected tier `t`, cost = `InputUncached·t.InputUncached + CacheReadInput·t.CacheReadInput + CacheWrite5m·t.CacheWrite5m + CacheWrite1h·t.CacheWrite1h + (Output+ReasoningOutput)·t.Output`, integer math. `TotalCost()` is the sum of per-turn costs (each turn rated by *its own* input size — correct under D10's stateless resend-history model).

**Context-length tiers.** Several models charge a higher rate above an input-token threshold (gemini-2.5-pro, gemini-3.1-pro-preview at >200K; gpt-5.5, gpt-5.4 at >272K); `RateTier` captures both bands and the orchestrator picks per turn from `Usage`. Most models ship a single base tier.

**Preview-channel model.** `gemini-3.1-pro-preview` is the served id for Google's 3.x Pro reasoning model; there is no GA `gemini-3.1-pro` (research §2.2). It is a real, resolvable, priced id, kept in the curated set because v0.x is explicitly a pre-stable surface (product) — but it rides Google's preview channel and may change underneath us. Its exported constant (`google.ModelFlash31ProPreview`'s sibling, e.g. `google.ModelPro31Preview`) carries a doc-comment noting the preview caveat; every other supported model is a stable/GA id.

Settled choices:
- **nano-USD integers** — typical rates are exact integers in nano-USD/token, so cost is exact with no float drift; `Cost.USD()` converts only at display.
- **Pricing lives in provider sub-packages, in the model registry, exposed via the SPI** — co-located with model constants, no import cycle; a provider's table ships with its package and is the same map that gates model validity.
- **No unpriced state — cost is always available.** A turn produces usage only if the adapter accepted the model at Send (D5); accepted ⇒ in the registry ⇒ priced. The public `Cost()`/`TotalCost()` therefore return a bare `Cost` (no `ok`); zero usage yields `Cost(0)` naturally. The internal SPI keeps `Pricing(model) (Pricing, bool)` as a *structural* "does this provider know this id" check — a runtime `false` for a turn that ran is a broken invariant, failed loudly, never a silent zero surfaced to the consumer.
- **Tiered pricing via `RateTier`** — a turn is rated wholly at the highest tier whose `MinInputTokens` it meets (matching how providers re-rate the entire request once the prompt crosses the threshold), not a piecewise split.
- **Reasoning bills at the output rate** (research §6.3) — `ReasoningOutput` priced with `Output`.
- **Cost surfaced** per-turn (`Stream.Cost()`), cumulative (`Conversation.TotalCost()`), and in the `summary` log record (D15).
- **Reasoning spec rides the same registry as pricing.** Each registry entry carries both `Pricing` and `ReasoningSpec`, so "supported ⇒ priced" and "supported ⇒ reasoning-describable" are one structural guarantee; the validity gate (D5), the price, and the reasoning vocabulary cannot drift apart.
- **Introspection types live in root; the implementing value lives per sub-package.** `ReasoningKind`/`ReasoningSpec`/`Sentinel`/`ReasoningInspector` are root types (one shared shape); each sub-package exposes a package-level `Reasoning` value implementing `ReasoningInspector` over its registry. agentrepl holds a `map[string]ReasoningInspector` and renders/validates uniformly — no provider-specific branching, the whole point of the interface.
- **The introspector is credential-blind — a package-level value, not the `Provider` handle.** It reads only static registry data, so `--help` runs with no keys and constructs no client (agent-repl research §6.88). The `Provider` SPI gains **no** reasoning method: validation+warn live in the adapter (D6/D9), which reads its own registry directly, so root never needs the spec polymorphically (contrast `Pricing(model)`, which root *does* need via the SPI to compute cost).

**Baked-in rate tables** (nano-USD/token; gathered from official provider pricing 2026-06-17 per research §6.5 — re-verify before release). Each row is one `RateTier`; tiered models have two. Anthropic cache-write rates are derived from Anthropic's conventional 0.1×/1.25×/2× multipliers (base input/output published, high confidence); all other providers have no cache-write bucket (0). **gpt-5.5-pro has no cached-input discount**, so its `CacheReadInput` rate equals `InputUncached` (cached reads bill at full input rate).

| Model | MinInputTokens | InputUncached | CacheReadInput | CacheWrite5m | CacheWrite1h | Output |
|---|---|---|---|---|---|---|
| claude-opus-4-8 | 0 | 5000 | 500 | 6250 | 10000 | 25000 |
| claude-sonnet-4-6 | 0 | 3000 | 300 | 3750 | 6000 | 15000 |
| claude-haiku-4-5 | 0 | 1000 | 100 | 1250 | 2000 | 5000 |
| gemini-2.5-flash | 0 | 300 | 30 | 0 | 0 | 2500 |
| gemini-2.5-pro | 0 | 1250 | 125 | 0 | 0 | 10000 |
| gemini-2.5-pro | 200001 | 2500 | 250 | 0 | 0 | 15000 |
| gemini-3.5-flash | 0 | 1500 | 150 | 0 | 0 | 9000 |
| gemini-3.1-flash-lite | 0 | 250 | 25 | 0 | 0 | 1500 |
| gemini-3.1-pro-preview | 0 | 2000 | 200 | 0 | 0 | 12000 |
| gemini-3.1-pro-preview | 200001 | 4000 | 400 | 0 | 0 | 18000 |
| gpt-5.5-pro | 0 | 30000 | 30000 | 0 | 0 | 180000 |
| gpt-5.5 | 0 | 5000 | 500 | 0 | 0 | 30000 |
| gpt-5.5 | 272001 | 10000 | 1000 | 0 | 0 | 45000 |
| gpt-5.4 | 0 | 2500 | 250 | 0 | 0 | 15000 |
| gpt-5.4 | 272001 | 5000 | 500 | 0 | 0 | 22500 |
| gpt-5.4-mini | 0 | 750 | 75 | 0 | 0 | 4500 |
| gpt-5.4-nano | 0 | 200 | 20 | 0 | 0 | 1250 |
| glm-5.2 | 0 | 1400 | 260 | 0 | 0 | 4400 |
| glm-5.1 | 0 | 1400 | 260 | 0 | 0 | 4400 |
| glm-4.7 | 0 | 600 | 110 | 0 | 0 | 2200 |
| glm-4.6 | 0 | 600 | 110 | 0 | 0 | 2200 |

**Baked-in reasoning specs** (the `ReasoningSpec` per model; native vocabulary verified 2026-06-18 per research §7.1 — re-verify before release). `Default` is given as the `ReasoningValue` constructor form. `CanDisable` reflects whether `DisableReasoning()` is honored (else it warns).

| Model | Term | Kind | Levels / Range | Sentinels | Default | CanDisable |
|---|---|---|---|---|---|---|
| claude-opus-4-8 | effort | Enum | low, medium, high, xhigh, max | — | `Level("high")` | yes |
| claude-sonnet-4-6 | effort | Enum | low, medium, high, max | — | `Level("high")` | yes |
| claude-haiku-4-5 | thinking budget | Range | 1024 … max_tokens−1 ⚠ | — | `DisableReasoning()` (off) | yes |
| gpt-5.5-pro | effort | Enum | high, xhigh *(est.)* | — | `Level("high")` *(est.)* | no |
| gpt-5.5 | effort | Enum | none, low, medium, high, xhigh | — | `Level("medium")` | yes |
| gpt-5.4 | effort | Enum | none, low, medium, high, xhigh | — | `Level("none")` | yes |
| gpt-5.4-mini | effort | Enum | none, low, medium, high, xhigh | — | `Level("none")` *(est.)* | yes |
| gpt-5.4-nano | effort | Enum | none, low, medium, high, xhigh | — | `Level("none")` *(est.)* | yes |
| gemini-2.5-flash | thinking budget | Range | 0 … 24576 | 0=off, −1=dynamic | `Budget(-1)` (dynamic) | yes |
| gemini-2.5-pro | thinking budget | Range | 128 … 32768 | −1=dynamic (0 rejected) | `Budget(-1)` (dynamic) | no |
| gemini-3.5-flash | thinking level | Enum | minimal, low, medium, high | — | `Level("medium")` | no |
| gemini-3.1-flash-lite | thinking level | Enum | minimal, low, medium, high | — | `Level("medium")` *(est.)* | no |
| gemini-3.1-pro-preview | thinking level | Enum | low, medium, high | — | `Level("high")` | no |
| glm-5.2 | effort (+ toggle) | Enum | high, max | — | `Level("max")` | yes |
| glm-5.1 | effort (+ toggle) | Enum | high, max *(under-doc.)* | — | `Level("max")` *(est.)* | yes |
| glm-4.7 | thinking | Toggle | — (on/off only) | — | unset (enabled) | yes |
| glm-4.6 | thinking | Toggle | — (on/off only) | — | unset (enabled) | yes |

Caveats the spec data must encode:
- ⚠ **haiku-4-5's budget upper bound is request-dependent** (`max_tokens−1`). The static spec records `Min=1024` and the model's max-output ceiling as `Max`; the adapter clamps to `min(Max, request.MaxTokens−1)` at build time. Its default reasoning is **off**, so `Default = DisableReasoning()`.
- **GLM 5.x is two-axis** (an on/off `thinking` toggle *plus* an effort enum): modeled as `Kind=Enum` with `Levels=[high,max]` and `CanDisable=true` (the toggle). GLM 4.6/4.7 have only the toggle: `Kind=Toggle`, no levels, `CanDisable=true`, default enabled (`Default=unset`).
- **Anthropic effort default is `high`** for display/fallback; the thinking-block axis (off until a level engages `type:"adaptive"`) is handled by the D9 lowering, not the spec value.
- *(est.)* cells (gpt-5.5-pro levels/default, gpt-5.4-mini/nano defaults, gemini-3.1-flash-lite default) are research estimates — the plan/release process verifies them against a live provider response before release, exactly as for the lower-confidence pricing rows.

**Both tables are the contractual source.** The shipped per-provider registries copy the rate values *and* the reasoning-spec values verbatim, and golden tests hold the code to them — so a transcription slip can't ship a wrong price or a wrong reasoning vocabulary. **Live-rate re-verification is a release obligation, not a unit test:** these are commercial rates that drift, and two figures are lower-confidence — the gpt-5.5/gpt-5.4 `>272K` threshold and high-tier rates, and Anthropic's multiplier-derived cache-write/read rates (base input/output are published; cache rates use the conventional 0.1×/1.25×/2×). Re-checking them against each provider's live pricing page before a release is owned by the **plan/release process**; when a rate changes, this table is the one place edited and the golden test re-baselined.

**Rejected.**
- *Consumer-supplied rate table* — AgentKit owns the price data; provider rates ship with it.
- *float64 USD* — float drift on accumulation; nano-USD integers are exact.
- *A flat single-tier `Pricing` struct (bake low-tier rates, document the >threshold undercount)* — simpler, but knowingly under-reports cost on large-context turns; the product makes cost a first-class promise, so `RateTier` pays the small extra surface to stay exact across context bands.
- *`(Cost, bool)` public return / unpriced `ok=false` path* — models a state the closed curated set makes impossible; a vestigial surface that lies. Removed; the registry guarantees availability.
- *Two separate tables (model constants + prices) reconciled by a test* — lets the supported set and the priced set drift; the single registry makes "supported ⇒ priced" structural, with the completeness test as belt-and-suspenders.
- *Introspection as a `Provider`/handle method* — would force `--help` to construct a (fake-keyed) client for static metadata; the credential-blind package-level value avoids it.
- *Bare package-level functions instead of a `ReasoningInspector` interface* — forces a 4-way `switch provider {…}` in agentrepl for both `--help` and runtime lookups; the interface lets it hold one `map[string]ReasoningInspector` and stay provider-agnostic.
- *A root-level `agentkit.ReasoningSpecFor(provider, model)`* — root cannot import sub-packages (D9 import-cycle rule), so it cannot host the registry; introspection must originate per sub-package.

**Verification.**
- R-S6NB-RYUE — each sub-package's `Reasoning` introspector returns specs without constructing a `Provider` handle or issuing any network call (credential-blind: callable with no API key set).
- R-S7V8-5QL3 — `SupportedReasoning()` returns a `ReasoningSpec` for every exported model constant that has reasoning control, and the returned map is keyed by those exact model ids.
- R-S934-JIBS — `ReasoningSpec(model)` returns `(spec, true)` for a known reasoning model id and `(_, false)` for an id the provider's registry does not know.
- R-PTEW-5KVY — for any supported model, `Stream.Cost()` (via `Pricing.Cost`) equals the integer sum of each `Usage` bucket times its rate at the selected tier, with reasoning billed at the output rate.
- R-V1KQ-IKI6 — every exported model constant in every provider sub-package resolves to a `Pricing` in its registry (supported ⇒ priced; the registry is complete, so no turn can run unpriced).
- R-VDY4-AP7H — each provider registry's `RateTier` values (per model, per tier) equal the rates published in this decision's table, so the shipped code and the doc cannot silently diverge.
- R-EN2N-9B9F — each provider registry's `ReasoningSpec` per model (Term, Kind, Levels, Min/Max, Sentinels, Default, CanDisable) equals the reasoning-spec table in this decision, so shipped code and doc cannot diverge on a model's native vocabulary.
- R-EPIG-0UQT — for every supported model, `spec.Default` is itself accepted by that model's own `spec` (a value in `Levels`, an in-range/sentinel budget, a valid disable, or unset), so the warn-fallback (D6) can never produce a value that would itself warn.
- R-V2SM-WC8V — a turn whose total input tokens exceed a tiered model's `MinInputTokens` threshold (gemini-2.5-pro, gemini-3.1-pro-preview, gpt-5.5, gpt-5.4) is rated wholly at the high tier; a turn at or below it is rated at the base tier.
- R-PVUO-X4DC — `TotalCost()` equals the sum of per-turn costs, and the `summary` log record carries it.
- R-PX2L-AW41 — `Cost.USD()` converts nano-USD to USD correctly.

## Decision 17 — MCP servers as a tool source

**Decision.** A remote MCP server is a *tool source*, not a fifth provider: AgentKit connects as an MCP client, discovers the server's tools, wraps each as an ordinary `Tool` (Decision 4), and merges them into the same registry the auto-loop already drives (Decision 10). The model and the four provider adapters never know an MCP tool from a custom one. MCP adds one consumer field, one consumer type, one error field, and one internal client; it changes nothing in the canonical message model.

```go
// On Conversation (Decision 1): a bare exported slice, appended to like Tools.
// Servers attach/detach by mutating this slice between turns.
MCPServers []MCPServer

// MCPServer is a remote MCP tool server the consumer attaches. All fields are
// supplied explicitly; AgentKit sources no credentials on its own.
type MCPServer struct {
    Name    string            // prefixes this server's tools; must be unique among attached servers
    URL     string            // Streamable-HTTP endpoint (single URL, POST + optional GET)
    Headers map[string]string // sent on every request — e.g. "Authorization": "Bearer …", "X-API-Key": …
}
```

**Connection & discovery happen at the `Send` boundary (Option A).** There is no `Attach` verb — attaching is appending to `MCPServers`. On the first `Send` after the attached set changes, AgentKit connects, runs the `initialize` → `notifications/initialized` handshake, and discovers tools via `tools/list` (paginated on `cursor`/`nextCursor` until absent). The live session and the discovered tool snapshot are cached on the `Conversation` (unexported) and reused across subsequent turns; a server-set change re-discovers. Unreachable server, handshake/discovery failure, name collision, and schema lossiness all surface at this boundary — before any provider call, with `History` unchanged — exactly where Decision 4 (duplicate tool names) and Decision 5 (unknown model) already validate. `Conversation.Close()` (Decision 15) best-effort `DELETE`s each live session.

**Name prefixing, sanitization, routing.** Each discovered tool is exposed as `<serverName>_<mcpToolName>`, then the whole string is sanitized to the strict tool-name charset `^[a-zA-Z_][a-zA-Z0-9_]{0,63}$` (replace illegal chars with `_`, ensure a letter/`_` start, truncate to ≤64 with a hash suffix on overflow). Real MCP names carry `.`/`/`/`:` which Anthropic and OpenAI reject. Sanitization is lossy and irreversible, so a call is routed back to its origin by a stored `(serverHandle, originalMCPName)` binding — **never** by re-parsing the prefix out of the exposed name.

**The internal client.** A hand-rolled raw-HTTP Streamable-HTTP client (`internal/mcp`, not consumer-importable), targeting MCP revision **`2025-11-25`**, implementing exactly four calls: `initialize`, `notifications/initialized`, `tools/list`, `tools/call`. It reuses `internal/sse` (Decision 12). The one subtle part: a request POST may answer with either `application/json` (single response) or `text/event-stream` (an SSE stream that eventually carries the response); the client handles **both** and reads the JSON-RPC response from whichever arrives. It echoes any `Mcp-Session-Id` from the `InitializeResult` on every subsequent request and always sends `MCP-Protocol-Version: <negotiated>`. On a `404` (session expired) for an idempotent discovery op it transparently re-initializes (fresh session) and retries; on a `404` mid-`tools/call` it re-establishes the session but does **not** replay the call (side-effect risk) — it surfaces the error.

**The two failure channels map onto the two AgentKit already has.** The decision rule is the JSON-RPC envelope: a *successful* `result` with `isError:true` = the tool ran and its business logic failed → `ToolResultBlock{IsError:true}` fed back to the model, loop continues (Decision 4). A JSON-RPC `error` object, or any HTTP/transport failure = AgentKit uniform error via `Stream.Err()` (Decision 7). `isError` is **only** read to set the block flag, **never** to decide whether to raise.

**Auth is a static token in a header; no interactive OAuth.** The consumer's `Headers` go on every request. A `401` with a `WWW-Authenticate` header (server wants full OAuth) is **not** followed — it surfaces as `ErrAuthentication` with the `WWW-Authenticate` value preserved in `Error.Raw`/`Message` so the consumer learns a token is needed; `403` → `ErrPermission` (Decision 7 owns the mapping).

Settled choices:
- **MCP rides the `Tool` abstraction** — wrap-and-merge, no new block type, no special-casing in the loop or adapters (research §9, line 410).
- **Lazy connect at the `Send` boundary** (Option A) — preserves Decision 1's bare-struct, no-constructor surface; "attach/detach between turns" is slice mutation, symmetric with provider/model switching. The eager-on-attach goal (surface failures pre-turn, not mid-turn) is met because the boundary *is* pre-provider-call.
- **`Headers map[string]string`, not a dedicated `Token` field** — covers `Authorization: Bearer` and arbitrary schemes (`X-API-Key`) uniformly; matches the no-OAuth, consumer-supplies-credential rule.
- **Route by stored `(server, originalName)` map, never by prefix re-parsing** — sanitization is irreversible (research line 410).
- **Per-server failure isolation on discovery** — one unreachable server fails its own `Send` boundary cleanly; collisions/lossiness are attributed per server+tool.
- **`tools/list_changed` deferred** — v1 re-lists on attach / server-set change only; honoring mid-conversation churn would bust the cache prefix (Decision 10, research line 444).
- **MCP resources/prompts, local stdio servers, and OAuth negotiation are out** (product scope).

**Rejected.**
- *Explicit `Attach(ctx)/Detach` methods* (Option B) — second error path, breaks Decision 1's fields-plus-`Send` surface, asymmetric with provider/model switching.
- *A dedicated `Token` field* — narrower than `Headers` for no gain; `X-API-Key`-style servers wouldn't fit.
- *Re-parsing the server prefix from the exposed tool name to route* — sanitization is lossy; a stored binding is exact.
- *A new `ErrMCP`/`ErrToolTransport` sentinel* — would reduce the very uniformity the taxonomy exists for; the existing categories absorb MCP (Decision 7).
- *Inspecting `isError` to decide raise-vs-feedback* — eino's anti-pattern; the `result`-vs-`error` envelope decides (research line 415).

**Verification.**
- R-6GBE-J3SV — attaching a server via `MCPServers` causes connect + handshake + `tools/list` (paginated to exhaustion) at the next `Send`; the discovered tools become callable through the auto-loop with results fed back, identically across all four providers.
- R-6HJA-WVJK — a discovered tool is exposed as `<serverName>_<originalName>` sanitized to `^[a-zA-Z_][a-zA-Z0-9_]{0,63}$`, and a `tools/call` is routed to its origin server via the stored `(server, originalName)` binding, not by re-parsing the exposed name.
- R-6IR7-ANA9 — a discovered MCP tool whose exposed name collides with an existing tool (a custom tool or another server's) surfaces `ErrInvalidConfig` (matchable via `errors.Is`) at the `Send` boundary, leaves `History` unchanged, and issues no provider call.
- R-6L70-26RN — a server unreachable at the `Send` boundary (or whose handshake/discovery fails) surfaces a uniform classifiable error via `Stream.Err()` before any provider call with `History` unchanged; one failing server is isolated to its own attribution and does not corrupt other servers' tools.
- R-6MEW-FYIC — the client reads the JSON-RPC response correctly whether a request POST answers with `application/json` or `text/event-stream` (both response paths handled).
- R-6NMS-TQ91 — a `tools/call` returning a `result` with `isError:true` yields a `ToolResultBlock{IsError:true}` fed back and the loop continues; a JSON-RPC `error` object or a transport failure surfaces as a uniform `Stream.Err()` — the `result`-vs-`error` envelope decides, never `isError`.
- R-6OUP-7HZQ — an `Mcp-Session-Id` returned on `InitializeResult` is echoed on every subsequent request, and `MCP-Protocol-Version: <negotiated>` is sent on every post-init request.
- R-6Q2L-L9QF — consumer `Headers` are sent on every request; a `401` carrying `WWW-Authenticate` surfaces `ErrAuthentication` with that header value preserved in `Error.Raw`/`Message` and no OAuth flow is attempted; a `403` surfaces `ErrPermission`.
- R-6RAH-Z1H4 — a `404` (expired session) on an idempotent discovery op transparently re-initializes and retries; a `404` mid-`tools/call` re-establishes the session but does **not** replay the call and surfaces the failure.
- R-6SIE-CT7T — detaching a server (removing it from `MCPServers`) between turns removes its tools at the next `Send`, and `Conversation.Close()` best-effort `DELETE`s each live MCP session.

## Status

Fully decided: Decisions 1–17. Consumer surface: D1 (`Conversation` + `Send`), D2 (`Stream` + `Event`), D3 (message & block model), D4 (tools), D5 (provider packaging), D6 (generation settings & the native `ReasoningValue` + warn-and-default), D7 (error model), D8 (usage), D15 (JSONL event log & lifecycle), D16 (model registry: pricing, cost & reasoning introspection), D17 (MCP servers as a tool source). Internal: D9 (package architecture, adapter SPI & per-adapter reasoning lowering), D10 (orchestration layer), D11 (retry & backoff), D12 (raw HTTP), D13 (testing strategy). Reasoning is native-per-model: a tagged `ReasoningValue` (D6), credential-blind per-model introspection via `ReasoningInspector` (D16), adapter-owned lowering + request-build-time validation that warns and falls back to the model default (D6/D9) — no cross-model enum. MCP support (D17) reuses the `Tool` abstraction and threads targeted edits through D4 (third-party schema lossiness → warning), D7 (`MCPServer` attribution, no new sentinel), D10 (tool ordering + cache invalidation), D11 (retry discovery, not `tools/call`), D12 (`internal/mcp` client), D13 (fake MCP server). Seams, public interfaces, naming, types, data model, and the testing approach are all decided. The construction order that realizes this design lives in the plan.
