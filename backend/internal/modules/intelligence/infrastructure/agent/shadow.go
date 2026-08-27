package agent

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"time"

	intelligencedomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/intelligence/domain"
)

// ShadowOptions bounds optional comparison traffic. Shadow work is never part
// of the primary AI Run result and is safe to disable by passing a nil Client.
type ShadowOptions struct {
	Timeout        time.Duration
	MaxConcurrency int
	Observe        func(ShadowObservation)
}

// ShadowObservation contains only bounded, sanitized comparison facts.
// It deliberately excludes prompts, source text, model output, and provider
// error text. Output digests are comparison evidence, never metric labels; the
// production Bootstrap observer omits them from ordinary logs.
type ShadowObservation struct {
	TaskType            string
	SchemaName          string
	SchemaVersion       string
	Result              string
	ErrorCode           int
	PrimaryJSONValid    bool
	ShadowJSONValid     bool
	PrimaryOutputSHA256 string
	ShadowOutputSHA256  string
	DurationMS          int64
	Dropped             bool
}

// ShadowRunner submits bounded, asynchronous Agent comparisons. Submit never
// waits for the Agent and never changes the primary provider response.
type ShadowRunner struct {
	client  *Client
	timeout time.Duration
	slots   chan struct{}
	observe func(ShadowObservation)
}

func NewShadowRunner(client *Client, options ShadowOptions) (*ShadowRunner, error) {
	if options.Timeout <= 0 || options.Timeout > 30*time.Second {
		return nil, fmt.Errorf("shadow timeout must be between 1ns and 30s")
	}
	if options.MaxConcurrency <= 0 || options.MaxConcurrency > 32 {
		return nil, fmt.Errorf("shadow concurrency must be between 1 and 32")
	}
	if options.Observe == nil {
		options.Observe = func(ShadowObservation) {}
	}
	return &ShadowRunner{client: client, timeout: options.Timeout, slots: make(chan struct{}, options.MaxConcurrency), observe: options.Observe}, nil
}

// Submit is intentionally asynchronous: an Agent outage or slow response
// cannot add latency, retry pressure, or a write to the primary AI Run.
func (runner *ShadowRunner) Submit(ctx context.Context, request intelligencedomain.StructuredRequest, primary intelligencedomain.StructuredResponse) {
	if runner == nil || runner.client == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	request = cloneShadowRequest(request)
	primary = cloneShadowResponse(primary)
	select {
	case runner.slots <- struct{}{}:
	default:
		runner.observe(ShadowObservation{
			TaskType: string(request.TaskType), SchemaName: request.SchemaName, SchemaVersion: request.SchemaVersion,
			Result: "dropped", PrimaryJSONValid: json.Valid(primary.JSON), PrimaryOutputSHA256: digest(primary.JSON), Dropped: true,
		})
		return
	}
	go func() {
		defer func() { <-runner.slots }()
		start := time.Now()
		shadowContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), runner.timeout)
		defer cancel()
		shadow, err := runner.client.GenerateStructured(shadowContext, request)
		observation := ShadowObservation{
			TaskType: string(request.TaskType), SchemaName: request.SchemaName, SchemaVersion: request.SchemaVersion,
			PrimaryJSONValid: json.Valid(primary.JSON), PrimaryOutputSHA256: digest(primary.JSON), DurationMS: time.Since(start).Milliseconds(),
		}
		if err != nil {
			observation.Result = "agent_error"
			if code, ok := intelligencedomain.CodeOf(err); ok {
				observation.ErrorCode = code
			} else {
				observation.ErrorCode = intelligencedomain.CodeAIProviderTransient
			}
			runner.observe(observation)
			return
		}
		observation.ShadowJSONValid = json.Valid(shadow.JSON)
		observation.ShadowOutputSHA256 = digest(shadow.JSON)
		if !observation.ShadowJSONValid {
			observation.Result = "invalid"
		} else if canonicalJSONEqual(primary.JSON, shadow.JSON) {
			observation.Result = "matched"
		} else {
			observation.Result = "different"
		}
		runner.observe(observation)
	}()
}

func cloneShadowRequest(request intelligencedomain.StructuredRequest) intelligencedomain.StructuredRequest {
	copyOfRequest := request
	copyOfRequest.Schema = append(json.RawMessage(nil), request.Schema...)
	copyOfRequest.Input = append(json.RawMessage(nil), request.Input...)
	if request.Repair != nil {
		repair := *request.Repair
		repair.PreviousOutput = append(json.RawMessage(nil), request.Repair.PreviousOutput...)
		repair.Violations = append([]intelligencedomain.SchemaViolation(nil), request.Repair.Violations...)
		copyOfRequest.Repair = &repair
	}
	return copyOfRequest
}

func cloneShadowResponse(response intelligencedomain.StructuredResponse) intelligencedomain.StructuredResponse {
	response.JSON = append(json.RawMessage(nil), response.JSON...)
	return response
}

func digest(value []byte) string {
	if len(value) == 0 {
		return ""
	}
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:])
}

func canonicalJSONEqual(first, second []byte) bool {
	firstCanonical, err := canonicalJSON(first)
	if err != nil {
		return false
	}
	secondCanonical, err := canonicalJSON(second)
	return err == nil && bytes.Equal(firstCanonical, secondCanonical)
}

func canonicalJSON(value []byte) ([]byte, error) {
	var decoded any
	decoder := json.NewDecoder(bytes.NewReader(value))
	decoder.UseNumber()
	if err := decoder.Decode(&decoded); err != nil {
		return nil, err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return nil, fmt.Errorf("JSON contains trailing data")
	}
	return json.Marshal(decoded)
}
