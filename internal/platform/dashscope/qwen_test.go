package dashscope

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	servicereport "github.com/StephenQiu30/hotkey-server/internal/service/report"
)

func TestQwenClientSendsChatCompletionRequest(t *testing.T) {
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Header.Get("Authorization") != "Bearer test-key" {
			t.Fatalf("missing bearer auth")
		}
		body, err := io.ReadAll(req.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		text := string(body)
		if !strings.Contains(text, `"model":"qwen-test"`) || !strings.Contains(text, `"messages"`) || !strings.Contains(text, "中文日报") {
			t.Fatalf("unexpected request body: %s", text)
		}
		return jsonResponse(http.StatusOK, `{"choices":[{"message":{"role":"assistant","content":"# 中文日报\n来源引用：[1]"}}]}`), nil
	})
	client := NewQwenClient(QwenConfig{APIKey: "test-key", BaseURL: "https://example.test/v1", Model: "qwen-test"}, &http.Client{Transport: transport})
	got, err := client.GenerateReport(context.Background(), "请生成中文日报")
	if err != nil {
		t.Fatalf("GenerateReport returned error: %v", err)
	}
	if !strings.Contains(got, "中文日报") {
		t.Fatalf("expected Chinese response, got %q", got)
	}
}

func TestQwenClientMissingConfig(t *testing.T) {
	client := NewQwenClient(QwenConfig{}, &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("transport should not be called")
		return nil, nil
	})})
	if _, err := client.GenerateReport(context.Background(), "prompt"); !errorsIs(err, servicereport.ErrFailedConfig) {
		t.Fatalf("expected ErrFailedConfig, got %v", err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func jsonResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(bytes.NewBufferString(body)),
		Header:     make(http.Header),
	}
}

func errorsIs(err error, target error) bool {
	return err == target
}
