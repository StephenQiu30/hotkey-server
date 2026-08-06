package requestcontext

import (
	"context"
	"testing"
)

func TestTypedRequestAndTraceValuesStayInStandardContext(t *testing.T) {
	ctx := WithRequestID(context.Background(), "request-77")
	ctx = WithTraceID(ctx, "4bf92f3577b34da6a3ce929d0e0e4736")

	if got := RequestID(ctx); got != "request-77" {
		t.Fatalf("RequestID() = %q, want request-77", got)
	}
	if got := TraceID(ctx); got != "4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Fatalf("TraceID() = %q, want trace ID", got)
	}
}
