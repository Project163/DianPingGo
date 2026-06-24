package response

import (
	"dianping/pkg/errmsg"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func setUpGinCtx() (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	ctx.Request, _ = http.NewRequest(http.MethodGet, "/", nil)
	return ctx, w
}

func TestOk(t *testing.T) {
	ctx, w := setUpGinCtx()
	OK(ctx, gin.H{"name": "test"})
	var resp Response
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if !resp.Success {
		t.Errorf("Expected success to be true, got false")
	}
	if resp.BusinessCode != 2000 {
		t.Errorf("Expected business code to be 2000, got %d", resp.BusinessCode)
	}
	if resp.Message != "success" {
		t.Errorf("Expected message to be 'success', got '%s'", resp.Message)
	}
	if resp.Data == nil {
		t.Errorf("Expected data to be non-nil, got nil")
	}
}

func TestOK_WithNilData(t *testing.T) {
	ctx, w := setUpGinCtx()
	OK(ctx, nil)
	var resp Response
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if !resp.Success {
		t.Errorf("Expected success to be true, got false")
	}
	if resp.BusinessCode != 2000 {
		t.Errorf("Expected business code to be 2000, got %d", resp.BusinessCode)
	}
	if resp.Message != "success" {
		t.Errorf("Expected message to be 'success', got '%s'", resp.Message)
	}
	if resp.Data != nil {
		t.Errorf("Expected data to be nil, got non-nil")
	}
}

func TestFail_WithCustomError(t *testing.T) {
	ctx, w := setUpGinCtx()
	realErr := fmt.Errorf("db timeout")
	Fail(ctx, errmsg.NewError(errmsg.ErrInvalidParam, realErr))

	var resp Response
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if resp.Success {
		t.Errorf("Expected success to be false, got true")
	}
	if resp.BusinessCode != errmsg.ErrInvalidParam.BusinessCode {
		t.Errorf("Expected business code to be %d, got %d", errmsg.ErrInvalidParam.BusinessCode, resp.BusinessCode)
	}
	if resp.Message != errmsg.ErrInvalidParam.Message {
		t.Errorf("Expected message to be '%s', got '%s'", errmsg.ErrInvalidParam.Message, resp.Message)
	}
	if resp.Data != nil {
		t.Errorf("Expected data to be nil, got non-nil")
	}
}

func TestFail_WithUnknownError(t *testing.T) {
	ctx, w := setUpGinCtx()
	Fail(ctx, fmt.Errorf("unexpected error"))

	var resp Response
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if resp.Success {
		t.Errorf("Expected success to be false, got true")
	}
	if resp.BusinessCode != errmsg.ErrInternalSec.BusinessCode {
		t.Errorf("Expected business code to be %d, got %d", errmsg.ErrInternalSec.BusinessCode, resp.BusinessCode)
	}
	if resp.Message != errmsg.ErrInternalSec.Message {
		t.Errorf("Expected message to be '%s', got '%s'", errmsg.ErrInternalSec.Message, resp.Message)
	}
	if resp.Data != nil {
		t.Errorf("Expected data to be nil, got non-nil")
	}
}

func TestGetTraceID(t *testing.T) {
	t.Run("traceId exits", func(t *testing.T) {
		w := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(w)
		ctx.Request = httptest.NewRequest(http.MethodGet, "/", nil)
		ctx.Set("traceId", "test-trace-123")

		if got := GetTraceID(ctx); got != "test-trace-123" {
			t.Errorf("GetTraceID() = %v, want %v", got, "test-trace-123")
		}
	})

	t.Run("traceId does not exist", func(t *testing.T) {
		w := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(w)
		ctx.Request = httptest.NewRequest(http.MethodGet, "/", nil)

		if got := GetTraceID(ctx); got != "" {
			t.Errorf("GetTraceID() = %v, want empty string", got)
		}
	})
}
