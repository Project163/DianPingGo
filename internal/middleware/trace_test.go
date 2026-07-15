package middleware

import (
	"dianping/pkg/requestctx"
	"dianping/pkg/response"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestTraceIDMiddleware(t *testing.T) {
	tests := []struct {
		name     string
		incoming string
		wantSame bool
	}{
		{
			name:     "没有传入时生成",
			incoming: "",
		},
		{
			name:     "合法ID继续使用",
			incoming: "0123456789abcdef0123456789abcdef",
			wantSame: true,
		},
		{
			name:     "非法ID重新生成",
			incoming: "invalid\r\nheader",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := gin.New()
			r.Use(TraceIDMiddleware())
			r.GET("/", func(ctx *gin.Context) {
				response.OK(ctx, gin.H{
					"service_trace_id": requestctx.TraceID(
						ctx.Request.Context(),
					),
				})
			})

			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.incoming != "" {
				req.Header.Set(TraceIDHeader, tt.incoming)
			}

			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			got := w.Header().Get(TraceIDHeader)
			require.Len(t, got, 32)

			if tt.wantSame {
				require.Equal(t, tt.incoming, got)
			} else {
				require.NotEqual(t, tt.incoming, got)
			}
		})
	}
}
