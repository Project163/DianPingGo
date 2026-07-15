package requestctx

import (
	"context"
)

const GinTraceIDKey = "trace_id"

type contextKey uint8

const traceIDKey contextKey = iota

func WithTraceID(ctx context.Context, traceID string) context.Context {
	return context.WithValue(ctx, traceIDKey, traceID)
}

func TraceID(ctx context.Context) string {
	traceID, _ := ctx.Value(traceIDKey).(string)
	return traceID
}
