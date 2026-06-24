package voucherorder

import (
	"dianping/internal/middleware"
	"dianping/pkg/errmsg"
	"dianping/pkg/idgen"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func setupHandler(t *testing.T) (*Handler, *mockVoucherOrderRepo, *mockSeckillVoucherRepo, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	voucherRepo := new(mockVoucherOrderRepo)
	seckillRepo := new(mockSeckillVoucherRepo)
	idWorker := idgen.NewRedisIDWorker(rdb)
	svc := NewService(voucherRepo, seckillRepo, rdb, idWorker)
	return NewHandler(svc), voucherRepo, seckillRepo, mr
}

func setupGinCtx(method, path string, body any) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	if body != nil {
		jsonBytes, _ := json.Marshal(body)
		c.Request = httptest.NewRequest(method, path, strings.NewReader(string(jsonBytes)))
		c.Request.Header.Set("Content-Type", "application/json")
	} else {
		c.Request = httptest.NewRequest(method, path, nil)
	}

	return c, w
}

func parseOKResp[T any](t *testing.T, w *httptest.ResponseRecorder) T {
	t.Helper()
	var resp struct {
		Success bool   `json:"success"`
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    T      `json:"data"`
	}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	require.True(t, resp.Success)
	return resp.Data
}

func parseFailCode(t *testing.T, w *httptest.ResponseRecorder) int {
	t.Helper()
	var resp struct {
		Success bool   `json:"success"`
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	require.False(t, resp.Success)
	return resp.Code
}

// ============================================================================
// NewHandler test
// ============================================================================

func TestNewHandler(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	svc := NewService(new(mockVoucherOrderRepo), new(mockSeckillVoucherRepo), rdb, idgen.NewRedisIDWorker(rdb))

	h := NewHandler(svc)
	require.NotNil(t, h)
	require.NotNil(t, h.srv)
}

// ============================================================================
// SeckillVoucher handler tests
// ============================================================================

func TestHandler_SeckillVoucher(t *testing.T) {
	t.Run("success: returns order id and http 200", func(t *testing.T) {
		h, _, _, mr := setupHandler(t)

		mr.Set("seckill:stock:1", "10")

		c, w := setupGinCtx(http.MethodPost, "/api/seckill/1", SeckillReq{VoucherID: 1})
		c.Set(middleware.CtxUserIDKey, uint64(100))

		h.SeckillVoucher(c)

		require.Equal(t, http.StatusOK, w.Code)
		orderID := parseOKResp[uint64](t, w)
		require.NotZero(t, orderID)
	})

	t.Run("unauthorized: no userId in context", func(t *testing.T) {
		h, _, _, _ := setupHandler(t)

		c, w := setupGinCtx(http.MethodPost, "/api/seckill/1", SeckillReq{VoucherID: 1})
		// 不设置 CtxUserIDKey

		h.SeckillVoucher(c)

		code := parseFailCode(t, w)
		require.Equal(t, errmsg.ErrUnauthorized.BusinessCode, code)
	})

	t.Run("invalid body: nil json", func(t *testing.T) {
		h, _, _, _ := setupHandler(t)

		gin.SetMode(gin.TestMode)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/api/seckill/1", http.NoBody)
		c.Set(middleware.CtxUserIDKey, uint64(100))

		h.SeckillVoucher(c)

		code := parseFailCode(t, w)
		require.Equal(t, errmsg.ErrInvalidParam.BusinessCode, code)
	})

	t.Run("invalid body: zero voucherId", func(t *testing.T) {
		h, _, _, _ := setupHandler(t)

		c, w := setupGinCtx(http.MethodPost, "/api/seckill/1", map[string]int{"voucherId": 0})
		c.Set(middleware.CtxUserIDKey, uint64(100))

		h.SeckillVoucher(c)

		code := parseFailCode(t, w)
		require.Equal(t, errmsg.ErrInvalidParam.BusinessCode, code)
	})

	t.Run("invalid body: non-json content", func(t *testing.T) {
		h, _, _, _ := setupHandler(t)

		gin.SetMode(gin.TestMode)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/api/seckill/1", strings.NewReader("not-json"))
		c.Set(middleware.CtxUserIDKey, uint64(100))

		h.SeckillVoucher(c)

		code := parseFailCode(t, w)
		require.Equal(t, errmsg.ErrInvalidParam.BusinessCode, code)
	})

	t.Run("business error: no stock", func(t *testing.T) {
		h, _, _, mr := setupHandler(t)

		mr.Set("seckill:stock:1", "0")

		c, w := setupGinCtx(http.MethodPost, "/api/seckill/1", SeckillReq{VoucherID: 1})
		c.Set(middleware.CtxUserIDKey, uint64(100))

		h.SeckillVoucher(c)

		code := parseFailCode(t, w)
		require.Equal(t, errmsg.ErrNoStock.BusinessCode, code)
	})

	t.Run("business error: repeated order", func(t *testing.T) {
		h, _, _, mr := setupHandler(t)

		mr.Set("seckill:stock:1", "10")

		// 第一次成功
		{
			c, _ := setupGinCtx(http.MethodPost, "/api/seckill/1", SeckillReq{VoucherID: 1})
			c.Set(middleware.CtxUserIDKey, uint64(100))
			h.SeckillVoucher(c)
		}

		// 第二次重复 → 被拒绝
		c, w := setupGinCtx(http.MethodPost, "/api/seckill/1", SeckillReq{VoucherID: 1})
		c.Set(middleware.CtxUserIDKey, uint64(100))
		h.SeckillVoucher(c)

		code := parseFailCode(t, w)
		require.Equal(t, errmsg.ErrRepeatedOrder.BusinessCode, code)
	})

	t.Run("different users succeed independently", func(t *testing.T) {
		h, _, _, mr := setupHandler(t)

		mr.Set("seckill:stock:1", "10")

		c1, w1 := setupGinCtx(http.MethodPost, "/api/seckill/1", SeckillReq{VoucherID: 1})
		c1.Set(middleware.CtxUserIDKey, uint64(100))
		h.SeckillVoucher(c1)
		require.Equal(t, http.StatusOK, w1.Code)

		c2, w2 := setupGinCtx(http.MethodPost, "/api/seckill/1", SeckillReq{VoucherID: 1})
		c2.Set(middleware.CtxUserIDKey, uint64(200))
		h.SeckillVoucher(c2)
		require.Equal(t, http.StatusOK, w2.Code)

		require.NotEqual(t,
			parseOKResp[uint64](t, w1),
			parseOKResp[uint64](t, w2),
		)
	})
}

// ============================================================================
// SeckillVoucher handler — service error propagation
// ============================================================================

func TestHandler_SeckillVoucher_ServiceErrors(t *testing.T) {
	t.Run("service returns wrapped error → handler retains business code", func(t *testing.T) {
		h, voucherRepo, seckillRepo, mr := setupHandler(t)

		mr.Set("seckill:stock:1", "10")

		// 第一次成功，第二次由 Lua 脚本检测重复（不依赖 mock）
		c1, _ := setupGinCtx(http.MethodPost, "/api/seckill/1", SeckillReq{VoucherID: 1})
		c1.Set(middleware.CtxUserIDKey, uint64(100))
		h.SeckillVoucher(c1)

		c2, w2 := setupGinCtx(http.MethodPost, "/api/seckill/1", SeckillReq{VoucherID: 1})
		c2.Set(middleware.CtxUserIDKey, uint64(100))
		h.SeckillVoucher(c2)

		code := parseFailCode(t, w2)
		require.Equal(t, errmsg.ErrRepeatedOrder.BusinessCode, code)
		_ = voucherRepo
		_ = seckillRepo
	})
}

// ============================================================================
// SeckillVoucher handler — userId type edge case
// ============================================================================

func TestHandler_SeckillVoucher_UserIdTypes(t *testing.T) {
	t.Run("userId stored as uint64 in context", func(t *testing.T) {
		h, _, _, mr := setupHandler(t)

		mr.Set("seckill:stock:1", "10")

		c, w := setupGinCtx(http.MethodPost, "/api/seckill/1", SeckillReq{VoucherID: 1})
		c.Set(middleware.CtxUserIDKey, uint64(100))

		h.SeckillVoucher(c)

		require.Equal(t, http.StatusOK, w.Code)
		orderID := parseOKResp[uint64](t, w)
		require.NotZero(t, orderID)
	})
}

// ============================================================================
// misc
// ============================================================================

func TestSeckillReq_Binding(t *testing.T) {
	t.Run("zero voucherId fails binding", func(t *testing.T) {
		gin.SetMode(gin.TestMode)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		body := `{"voucherId":0}`
		c.Request = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Set(middleware.CtxUserIDKey, uint64(100))

		var req SeckillReq
		err := c.ShouldBindJSON(&req)
		require.Error(t, err)
	})

	t.Run("positive voucherId passes binding", func(t *testing.T) {
		gin.SetMode(gin.TestMode)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		body := `{"voucherId":1}`
		c.Request = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Set(middleware.CtxUserIDKey, uint64(100))

		var req SeckillReq
		err := c.ShouldBindJSON(&req)
		require.NoError(t, err)
		require.Equal(t, uint64(1), req.VoucherID)
	})
}
