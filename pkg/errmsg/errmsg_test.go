package errmsg

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestCustomError_Error(t *testing.T) {
	t.Run("with wrapped error", func(t *testing.T) {
		ce := NewError(ErrInvalidParam, fmt.Errorf("invalid value for field 'name'"))
		s := ce.Error()
		if !strings.Contains(s, "HTTP 400") {
			t.Errorf("unexpected error string: %s", s)
		}
		if !strings.Contains(s, "Business 4001") {
			t.Errorf("unexpected error string: %s", s)
		}
		if !strings.Contains(s, "请求参数错误") {
			t.Errorf("unexpected error string: %s", s)
		}
		if !strings.Contains(s, "invalid value for field 'name'") {
			t.Errorf("unexpected error string: %s", s)
		}
	})

	t.Run("without wrapped error", func(t *testing.T) {
		ce := NewError(ErrNotFound, nil)
		s := ce.Error()
		if !strings.Contains(s, "HTTP 404") {
			t.Errorf("unexpected error string: %s", s)
		}
		if !strings.Contains(s, "Business 4002") {
			t.Errorf("unexpected error string: %s", s)
		}
		if !strings.Contains(s, "资源不存在") {
			t.Errorf("unexpected error string: %s", s)
		}
	})
}

func TestCustomError_Unwrap(t *testing.T) {
	realErr := fmt.Errorf("database connection failed")
	ce := NewError(ErrInternalSec, realErr)
	if !errors.Is(ce, realErr) {
		t.Errorf("Unwrap did not return the original error")
	}

	var targetErr *CustomError
	if !errors.As(ce, &targetErr) {
		t.Errorf("As did not recognize CustomError type")
	}

	if targetErr.HttpCode != ErrInternalSec.HttpCode {
		t.Errorf("CustomError fields do not match expected values: want %v, got %v", ErrInternalSec.HttpCode, targetErr.HttpCode)
	}
	if targetErr.BusinessCode != ErrInternalSec.BusinessCode {
		t.Errorf("CustomError fields do not match expected values: want %v, got %v", ErrInternalSec.BusinessCode, targetErr.BusinessCode)
	}
	if targetErr.Message != ErrInternalSec.Message {
		t.Errorf("CustomError fields do not match expected values: want %v, got %v", ErrInternalSec.Message, targetErr.Message)
	}
}

func TestNewError(t *testing.T) {
	realErr := fmt.Errorf("some underlying error")
	base := ErrUserNotFound
	ce := NewError(base, realErr)

	if ce.HttpCode != base.HttpCode {
		t.Errorf("CustomError fields do not match base error: want %v, got %v", base.HttpCode, ce.HttpCode)
	}
	if ce.BusinessCode != base.BusinessCode {
		t.Errorf("CustomError fields do not match base error: want %v, got %v", base.BusinessCode, ce.BusinessCode)
	}
	if ce.Message != base.Message {
		t.Errorf("CustomError fields do not match base error: want %v, got %v", base.Message, ce.Message)
	}
	if ce.Err != realErr {
		t.Errorf("CustomError Err field does not match real error: want %v, got %v", realErr, ce.Err)
	}
}
