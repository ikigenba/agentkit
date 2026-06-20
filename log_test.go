package agentkit

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

type logRecordView struct {
	Type     string          `json:"type"`
	Time     time.Time       `json:"time"`
	Seq      int             `json:"seq"`
	Message  json.RawMessage `json:"message"`
	ToolUse  *ToolUse        `json:"tool_use"`
	Result   *ToolResult     `json:"tool_result"`
	Usage    *Usage          `json:"usage"`
	Warning  *Warning        `json:"warning"`
	Error    json.RawMessage `json:"error"`
	Turns    int             `json:"turns"`
	Cost     *Cost           `json:"cost"`
	Provider string          `json:"provider"`
	Model    string          `json:"model"`
	Status   string          `json:"status"`
}

type logClock struct {
	now    time.Time
	sleeps []time.Duration
}

func (c *logClock) Now() time.Time {
	return c.now
}

func (c *logClock) Sleep(ctx context.Context, delay time.Duration) error {
	c.sleeps = append(c.sleeps, delay)
	c.now = c.now.Add(delay)
	return ctx.Err()
}

func (c *logClock) Jitter(time.Duration) time.Duration {
	return time.Millisecond
}

type failWriter struct{}

func (failWriter) Write([]byte) (int, error) {
	return 0, errors.New("write failed")
}

func TestJSONLLogRecordsFollowProtocolEventOrder(t *testing.T) {
	// R-PH7W-BVH0
	tool := RawTool("lookup", "lookup", json.RawMessage(`{"type":"object"}`), func(context.Context, json.RawMessage) (string, error) {
		return "tool ok", nil
	})
	provider := &retryProvider{roundTrips: []*RoundTrip{
		NewRoundTrip(Message{Role: RoleAssistant, Blocks: []Block{ToolUseBlock{ID: "toolu_log", Name: "lookup", Input: json.RawMessage(`{"q":"x"}`)}}}, FinishToolUse, Usage{InputUncached: 1, Total: 1}, nil, nil),
		retryTextRoundTrip("done"),
	}}
	var buf bytes.Buffer
	conv := &Conversation{Provider: provider, Model: "log-model", Tools: []Tool{tool}, Log: &buf}

	stream := conv.Send(context.Background(), "hello")
	drainRetry(stream)
	if err := stream.Err(); err != nil {
		t.Fatalf("Err() = %v, want nil", err)
	}

	records := parseLogRecords(t, buf.String())
	gotTypes := recordTypes(records)
	wantTypes := []string{"turn_start", "message", "tool_use", "tool_result", "message", "usage", "turn_end"}
	if !reflect.DeepEqual(gotTypes, wantTypes) {
		t.Fatalf("record types = %v, want %v", gotTypes, wantTypes)
	}
	if records[2].ToolUse == nil || records[2].ToolUse.Name != "lookup" || string(records[2].ToolUse.Input) != `{"q":"x"}` {
		t.Fatalf("tool_use record = %#v, want complete tool call", records[2].ToolUse)
	}
	if records[3].Result == nil || records[3].Result.Output != "tool ok" || records[3].Result.IsError {
		t.Fatalf("tool_result record = %#v, want successful result", records[3].Result)
	}
	if records[5].Usage == nil || *records[5].Usage != stream.Usage() {
		t.Fatalf("usage record = %#v, want stream usage %#v", records[5].Usage, stream.Usage())
	}
	if records[6].Status != "ok" {
		t.Fatalf("turn_end status = %q, want ok", records[6].Status)
	}
}

func TestLogTimestampsUseInjectedClockAndSeqIsMonotonic(t *testing.T) {
	// R-PIFS-PN7P
	now := time.Date(2026, 6, 18, 12, 34, 56, 0, time.UTC)
	clock := &logClock{now: now}
	var buf bytes.Buffer
	conv := &Conversation{Provider: &retryProvider{roundTrips: []*RoundTrip{retryTextRoundTrip("ok")}}, Model: "log-model", Log: &buf, retryClock: clock}

	stream := conv.Send(context.Background(), "hello")
	drainRetry(stream)
	if err := stream.Err(); err != nil {
		t.Fatalf("Err() = %v, want nil", err)
	}

	records := parseLogRecords(t, buf.String())
	for i, record := range records {
		if !record.Time.Equal(now) {
			t.Fatalf("record %d time = %s, want injected clock time %s", i, record.Time, now)
		}
		if record.Seq != i+1 {
			t.Fatalf("record %d seq = %d, want %d", i, record.Seq, i+1)
		}
	}
}

func TestWarningsErrorsAndRetriesAreLogged(t *testing.T) {
	// R-PJNP-3EYE
	warningProvider := &retryProvider{roundTrips: []*RoundTrip{
		retryErrorRoundTrip(&Error{Category: ErrRateLimited, Provider: "log-test", Raw: json.RawMessage(`{"message":"retry"}`)}),
		NewRoundTrip(Message{Role: RoleAssistant, Blocks: []Block{TextBlock{Text: "ok"}}}, FinishStop, Usage{InputUncached: 1, Output: 1, Total: 2}, []Warning{{Setting: "reasoning", Detail: "degraded"}}, nil),
	}}
	var warningBuf bytes.Buffer
	conv := &Conversation{
		Provider:   warningProvider,
		Model:      "log-model",
		Log:        &warningBuf,
		Retry:      RetryPolicy{MaxAttempts: 2, BaseDelay: time.Millisecond, MaxDelay: time.Millisecond},
		retryClock: &logClock{now: time.Date(2026, 6, 18, 1, 0, 0, 0, time.UTC)},
	}
	drainRetry(conv.Send(context.Background(), "hello"))
	records := parseLogRecords(t, warningBuf.String())
	if !hasRecordType(records, "retry") {
		t.Fatalf("records = %v, want retry record", recordTypes(records))
	}
	warning := firstRecord(records, "warning")
	if warning == nil || warning.Warning == nil || warning.Warning.Setting != "reasoning" {
		t.Fatalf("warning record = %#v, want reasoning warning", warning)
	}

	providerErr := &Error{Category: ErrServerError, Provider: "log-test", Raw: json.RawMessage(`{"message":"boom"}`)}
	errorProvider := &retryProvider{roundTrips: []*RoundTrip{retryErrorRoundTrip(providerErr)}}
	var errorBuf bytes.Buffer
	errorConv := &Conversation{
		Provider: errorProvider,
		Model:    "log-model",
		Log:      &errorBuf,
		Retry:    RetryPolicy{MaxAttempts: 1},
	}
	stream := errorConv.Send(context.Background(), "hello")
	drainRetry(stream)
	if !errors.Is(stream.Err(), ErrServerError) {
		t.Fatalf("Err() = %v, want ErrServerError", stream.Err())
	}
	errorRecords := parseLogRecords(t, errorBuf.String())
	if !hasRecordType(errorRecords, "error") {
		t.Fatalf("records = %v, want error record", recordTypes(errorRecords))
	}
	if !strings.Contains(errorBuf.String(), `"Raw":{"message":"boom"}`) {
		t.Fatalf("error log does not carry verbatim raw provider body: %s", errorBuf.String())
	}
}

func TestFailingLogWriterDoesNotAffectTurn(t *testing.T) {
	// R-PKVL-H6P3
	conv := &Conversation{Provider: &retryProvider{roundTrips: []*RoundTrip{retryTextRoundTrip("ok")}}, Model: "log-model", Log: failWriter{}}

	stream := conv.Send(context.Background(), "hello")
	drainRetry(stream)

	if err := stream.Err(); err != nil {
		t.Fatalf("Err() = %v, want nil despite failing log writer", err)
	}
	if len(conv.History) != 2 {
		t.Fatalf("History len = %d, want successful committed turn", len(conv.History))
	}
}

func TestNilLogDisablesRecordWriting(t *testing.T) {
	// R-PM3H-UYFS
	conv := &Conversation{Provider: &retryProvider{roundTrips: []*RoundTrip{retryTextRoundTrip("ok")}}, Model: "log-model", Log: nil}

	stream := conv.Send(context.Background(), "hello")
	drainRetry(stream)

	if err := stream.Err(); err != nil {
		t.Fatalf("Err() = %v, want nil with nil Log", err)
	}
	if conv.TotalUsage() != stream.Usage() {
		t.Fatalf("TotalUsage() = %#v, want successful turn usage %#v", conv.TotalUsage(), stream.Usage())
	}
}

func TestCloseSummaryAndCumulativeUsageCost(t *testing.T) {
	// R-PNBE-8Q6H
	// R-POJA-MHX6
	// R-PVUO-X4DC
	pricing := Pricing{Tiers: []RateTier{{MinInputTokens: 0, InputUncached: 10, Output: 20}}}
	provider := &retryProvider{roundTrips: []*RoundTrip{
		NewRoundTrip(Message{Role: RoleAssistant, Blocks: []Block{TextBlock{Text: "one"}}}, FinishStop, Usage{InputUncached: 2, Output: 3, Total: 5}, nil, nil),
		NewRoundTrip(Message{Role: RoleAssistant, Blocks: []Block{TextBlock{Text: "two"}}}, FinishStop, Usage{InputUncached: 4, Output: 5, Total: 9}, nil, nil),
	}}
	var buf bytes.Buffer
	conv := &Conversation{Provider: providerWithPricing{Provider: provider, pricing: pricing}, Model: "log-model", Log: &buf}

	first := conv.Send(context.Background(), "one")
	drainRetry(first)
	second := conv.Send(context.Background(), "two")
	drainRetry(second)
	if err := first.Err(); err != nil {
		t.Fatalf("first Err() = %v, want nil", err)
	}
	if err := second.Err(); err != nil {
		t.Fatalf("second Err() = %v, want nil", err)
	}

	wantUsage := addUsage(first.Usage(), second.Usage())
	if conv.TotalUsage() != wantUsage {
		t.Fatalf("TotalUsage() = %#v, want %#v", conv.TotalUsage(), wantUsage)
	}
	wantCost := first.Cost() + second.Cost()
	if conv.TotalCost() != wantCost {
		t.Fatalf("TotalCost() = %d, want sum of turn costs %d", conv.TotalCost(), wantCost)
	}

	buf.Reset()
	if err := conv.Close(); err != nil {
		t.Fatalf("Close() = %v, want nil", err)
	}
	if err := conv.Close(); err != nil {
		t.Fatalf("second Close() = %v, want nil", err)
	}
	records := parseLogRecords(t, buf.String())
	if len(records) != 1 || records[0].Type != "summary" {
		t.Fatalf("summary records = %#v, want exactly one summary", records)
	}
	if records[0].Turns != 2 {
		t.Fatalf("summary turns = %d, want 2", records[0].Turns)
	}
	if records[0].Usage == nil || *records[0].Usage != wantUsage {
		t.Fatalf("summary usage = %#v, want %#v", records[0].Usage, wantUsage)
	}
	if records[0].Cost == nil || *records[0].Cost != wantCost {
		t.Fatalf("summary cost = %#v, want %#v", records[0].Cost, wantCost)
	}
}

func TestSendAfterCloseReturnsErrClosed(t *testing.T) {
	// R-PPR7-09NV
	provider := &retryProvider{roundTrips: []*RoundTrip{retryTextRoundTrip("unreached")}}
	conv := &Conversation{Provider: provider, Model: "log-model"}
	if err := conv.Close(); err != nil {
		t.Fatalf("Close() = %v, want nil", err)
	}

	stream := conv.Send(context.Background(), "hello")
	drainRetry(stream)

	if !errors.Is(stream.Err(), ErrClosed) {
		t.Fatalf("Err() = %v, want ErrClosed", stream.Err())
	}
	if provider.calls != 0 {
		t.Fatalf("provider calls = %d, want 0 after Close", provider.calls)
	}
}

type providerWithPricing struct {
	Provider
	pricing Pricing
}

func (p providerWithPricing) Pricing(string) (Pricing, bool) {
	return p.pricing, true
}

func parseLogRecords(t *testing.T, jsonl string) []logRecordView {
	t.Helper()
	lines := strings.Split(strings.TrimSpace(jsonl), "\n")
	if len(lines) == 1 && lines[0] == "" {
		return nil
	}
	records := make([]logRecordView, 0, len(lines))
	for i, line := range lines {
		if !json.Valid([]byte(line)) {
			t.Fatalf("line %d is not valid JSON: %q", i, line)
		}
		var record logRecordView
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("unmarshal line %d: %v\n%s", i, err, line)
		}
		records = append(records, record)
	}
	return records
}

func recordTypes(records []logRecordView) []string {
	types := make([]string, len(records))
	for i, record := range records {
		types[i] = record.Type
	}
	return types
}

func hasRecordType(records []logRecordView, typ string) bool {
	return firstRecord(records, typ) != nil
}

func firstRecord(records []logRecordView, typ string) *logRecordView {
	for i := range records {
		if records[i].Type == typ {
			return &records[i]
		}
	}
	return nil
}
