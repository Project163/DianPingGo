package voucherorder

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"dianping/internal/middleware"
	"dianping/internal/module/seckillvoucher"
	"dianping/internal/tx"
	"dianping/pkg/idgen"
	"dianping/pkg/validator"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// Mock types for handler tests
// =============================================================================

type mockVoucherOrderRepoForHandler struct {
	createVoucherOrderFunc   func(ctx context.Context, order *VoucherOrder) error
	countByUserAndVoucherFunc func(ctx context.Context, userID uint64, voucherID uint64) (int64, error)
	getVoucherOrderByIDFunc  func(ctx context.Context, orderID uint64) (*VoucherOrder, error)
}

func (m *mockVoucherOrderRepoForHandler) CreateVoucherOrder(ctx context.Context, order *VoucherOrder) error {
	if m.createVoucherOrderFunc != nil {
		return m.createVoucherOrderFunc(ctx, order)
	}
	return nil
}

func (m *mockVoucherOrderRepoForHandler) CountByUserAndVoucher(ctx context.Context, userID uint64, voucherID uint64) (int64, error) {
	if m.countByUserAndVoucherFunc != nil {
		return m.countByUserAndVoucherFunc(ctx, userID, voucherID)
	}
	return 0, nil
}

func (m *mockVoucherOrderRepoForHandler) GetVoucherOrderByID(ctx context.Context, orderID uint64) (*VoucherOrder, error) {
	if m.getVoucherOrderByIDFunc != nil {
		return m.getVoucherOrderByIDFunc(ctx, orderID)
	}
	return nil, nil
}

type mockSeckillVoucherRepoForHandler struct{}

func (m *mockSeckillVoucherRepoForHandler) GetSeckillVoucherByID(ctx context.Context, voucherID uint64) (*seckillvoucher.SeckillVoucher, error) {
	return nil, nil
}

func (m *mockSeckillVoucherRepoForHandler) DeductStock(ctx context.Context, voucherID uint64) (bool, error) {
	return true, nil
}

type noopTxManager struct{}

func (m *noopTxManager) Transaction(ctx context.Context, fn func(ctx context.Context) error) error {
	return fn(ctx)
}

// =============================================================================
// Setup
// =============================================================================

// setUpVoucherOrderHandler creates a test gin Engine with voucher order handler routes.
// Uses a real Service backed by mock repositories, miniredis, and a real ID worker.
func setUpVoucherOrderHandler(t *testing.T) (*gin.Engine, *mockVoucherOrderRepoForHandler, *miniredis.Miniredis) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	validator.InitValidator()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { require.NoError(t, rdb.Close()) })

	orderRepo := new(mockVoucherOrderRepoForHandler)
	seckillRepo := new(mockSeckillVoucherRepoForHandler)
	idWorker := idgen.NewRedisIDWorker(rdb)
	txMgr := new(noopTxManager)

	svc := NewService(orderRepo, seckillRepo, rdb, idWorker, txMgr)
	handler := NewHandler(svc)

	r := gin.New()

	// Routes requiring auth (with mock userID injection)
	auth := r.Group("/api")
	auth.Use(func(ctx *gin.Context) {
		ctx.Set(middleware.CtxUserIDKey, uint64(1))
		ctx.Next()
	})
	{
		auth.POST("/voucher-order/seckill/:id", handler.SeckillVoucher)
	}

	// Routes without auth
	r.GET("/voucher-order/:id", handler.GetVoucherOrderByID)

	return r, orderRepo, mr
}

// decodeBody decodes the httptest.ResponseRecorder body into a map.
func decodeBody(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	return body
}

// =============================================================================
// SeckillVoucher
// =============================================================================

func TestHandler_SeckillVoucher(t *testing.T) {
	t.Run("seckill voucher successfully", func(t *testing.T) {
		r, _, mr := setUpVoucherOrderHandler(t)

		// Pre-set stock in Redis so the Lua script finds it
		mr.Set("seckill:stock:100", "10")

		reqBody := []byte(`{"voucher_id": 100}`)
		req := httptest.NewRequest(http.MethodPost, "/api/voucher-order/seckill/100", bytes.NewBuffer(reqBody))
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)
		body := decodeBody(t, w)
		require.Equal(t, true, body["success"])
		// data is the order ID from idWorker
		require.NotNil(t, body["data"])

		// Verify stock was decremented
		stockAfter, _ := mr.Get("seckill:stock:100")
		require.Equal(t, "9", stockAfter)
	})

	t.Run("seckill voucher without auth returns ErrUnauthorized", func(t *testing.T) {
		gin.SetMode(gin.TestMode)

		mr := miniredis.RunT(t)
		rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
		defer rdb.Close()

		idWorker := idgen.NewRedisIDWorker(rdb)
		svc := NewService(
			new(mockVoucherOrderRepoForHandler),
			new(mockSeckillVoucherRepoForHandler),
			rdb, idWorker, new(noopTxManager),
		)
		handler := NewHandler(svc)

		rr := gin.New()
		rr.POST("/seckill/:id", handler.SeckillVoucher)

		reqBody := []byte(`{"voucher_id": 100}`)
		req := httptest.NewRequest(http.MethodPost, "/seckill/100", bytes.NewBuffer(reqBody))
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		rr.ServeHTTP(w, req)

		require.Equal(t, http.StatusUnauthorized, w.Code)
		body := decodeBody(t, w)
		require.Equal(t, false, body["success"])
		require.Equal(t, float64(4010), body["code"].(float64))
	})

	t.Run("seckill voucher with invalid params", func(t *testing.T) {
		r, _, _ := setUpVoucherOrderHandler(t)

		reqBody := []byte(`{"voucher_id": 0}`)
		req := httptest.NewRequest(http.MethodPost, "/api/voucher-order/seckill/100", bytes.NewBuffer(reqBody))
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		body := decodeBody(t, w)
		require.Equal(t, false, body["success"])
		require.Equal(t, float64(4001), body["code"].(float64))
	})

	t.Run("seckill voucher with no stock returns error", func(t *testing.T) {
		r, _, mr := setUpVoucherOrderHandler(t)

		// Pre-set stock to zero
		mr.Set("seckill:stock:100", "0")

		reqBody := []byte(`{"voucher_id": 100}`)
		req := httptest.NewRequest(http.MethodPost, "/api/voucher-order/seckill/100", bytes.NewBuffer(reqBody))
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusBadRequest, w.Code)
		body := decodeBody(t, w)
		require.Equal(t, false, body["success"])
		require.Equal(t, float64(4301), body["code"].(float64))
	})

	t.Run("seckill voucher repeated order returns error", func(t *testing.T) {
		r, _, mr := setUpVoucherOrderHandler(t)

		// Pre-set stock
		mr.Set("seckill:stock:200", "5")

		// First order succeeds
		reqBody := []byte(`{"voucher_id": 200}`)
		req := httptest.NewRequest(http.MethodPost, "/api/voucher-order/seckill/200", bytes.NewBuffer(reqBody))
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code)

		// Second order by same user should fail
		w2 := httptest.NewRecorder()
		r.ServeHTTP(w2, req)

		require.Equal(t, http.StatusBadRequest, w2.Code)
		body := decodeBody(t, w2)
		require.Equal(t, false, body["success"])
		require.Equal(t, float64(4302), body["code"].(float64))
	})
}

// =============================================================================
// GetVoucherOrderByID
// =============================================================================

func TestHandler_GetVoucherOrderByID(t *testing.T) {
	t.Run("get voucher order by ID successfully", func(t *testing.T) {
		r, orderRepo, _ := setUpVoucherOrderHandler(t)
		orderRepo.getVoucherOrderByIDFunc = func(ctx context.Context, orderID uint64) (*VoucherOrder, error) {
			require.Equal(t, uint64(12345), orderID)
			return &VoucherOrder{
				ID: 12345, UserID: 1, VoucherID: 100, PayType: 1, Status: 0,
			}, nil
		}

		req := httptest.NewRequest(http.MethodGet, "/voucher-order/12345", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)
		body := decodeBody(t, w)
		require.Equal(t, true, body["success"])
		data := body["data"].(map[string]any)
		require.Equal(t, float64(12345), data["id"])
		require.Equal(t, float64(1), data["user_id"])
		require.Equal(t, float64(100), data["voucher_id"])
	})

	t.Run("get voucher order with empty ID returns ErrInvalidParam", func(t *testing.T) {
		gin.SetMode(gin.TestMode)

		mr := miniredis.RunT(t)
		rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
		defer rdb.Close()

		idWorker := idgen.NewRedisIDWorker(rdb)
		svc := NewService(
			new(mockVoucherOrderRepoForHandler),
			new(mockSeckillVoucherRepoForHandler),
			rdb, idWorker, new(noopTxManager),
		)
		handler := NewHandler(svc)

		w := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(w)
		ctx.Request = httptest.NewRequest(http.MethodGet, "/voucher-order/", nil)
		ctx.Params = gin.Params{{Key: "id", Value: ""}}
		handler.GetVoucherOrderByID(ctx)

		require.Equal(t, http.StatusBadRequest, w.Code)
		body := decodeBody(t, w)
		require.Equal(t, false, body["success"])
		require.Equal(t, float64(4001), body["code"].(float64))
	})

	t.Run("get voucher order with non-numeric ID returns ErrInvalidParam", func(t *testing.T) {
		r, _, _ := setUpVoucherOrderHandler(t)

		req := httptest.NewRequest(http.MethodGet, "/voucher-order/abc", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		body := decodeBody(t, w)
		require.Equal(t, false, body["success"])
		require.Equal(t, float64(4001), body["code"].(float64))
	})

	t.Run("get voucher order not found returns error", func(t *testing.T) {
		r, orderRepo, _ := setUpVoucherOrderHandler(t)
		orderRepo.getVoucherOrderByIDFunc = func(ctx context.Context, orderID uint64) (*VoucherOrder, error) {
			return nil, nil
		}

		req := httptest.NewRequest(http.MethodGet, "/voucher-order/99999", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusNotFound, w.Code)
		body := decodeBody(t, w)
		require.Equal(t, false, body["success"])
		require.Equal(t, float64(4303), body["code"].(float64))
	})
}

// Ensure mocks satisfy interfaces
var _ VoucherOrderRepository = (*mockVoucherOrderRepoForHandler)(nil)
var _ SeckillVoucherRepository = (*mockSeckillVoucherRepoForHandler)(nil)
var _ tx.Manager = (*noopTxManager)(nil)

// Ensure imported packages are used
var _ = idgen.NewRedisIDWorker
var _ = fmt.Sprintf
