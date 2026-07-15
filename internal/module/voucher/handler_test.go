package voucher

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"dianping/internal/module/seckillvoucher"
	"dianping/pkg/errmsg"
	"dianping/pkg/validator"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

// mockVoucherRepoForHandler is a mock for VoucherRepository used in handler tests.
type mockVoucherRepoForHandler struct {
	createVoucherFunc        func(ctx context.Context, voucher *Voucher) error
	createSeckillVoucherFunc func(ctx context.Context, v *Voucher, sv *seckillvoucher.SeckillVoucher) error
	getVoucherByIDFunc       func(ctx context.Context, id uint64) (*Voucher, error)
	getByShopIDFunc          func(ctx context.Context, shopID uint64) ([]Voucher, error)
}

func (m *mockVoucherRepoForHandler) CreateVoucher(ctx context.Context, voucher *Voucher) error {
	if m.createVoucherFunc != nil {
		return m.createVoucherFunc(ctx, voucher)
	}
	return nil
}

func (m *mockVoucherRepoForHandler) CreateSeckillVoucher(ctx context.Context, v *Voucher, sv *seckillvoucher.SeckillVoucher) error {
	if m.createSeckillVoucherFunc != nil {
		return m.createSeckillVoucherFunc(ctx, v, sv)
	}
	return nil
}

func (m *mockVoucherRepoForHandler) GetVoucherByID(ctx context.Context, id uint64) (*Voucher, error) {
	if m.getVoucherByIDFunc != nil {
		return m.getVoucherByIDFunc(ctx, id)
	}
	return nil, nil
}

func (m *mockVoucherRepoForHandler) GetByShopID(ctx context.Context, shopID uint64) ([]Voucher, error) {
	if m.getByShopIDFunc != nil {
		return m.getByShopIDFunc(ctx, shopID)
	}
	return nil, nil
}

// setUpVoucherHandler creates a test gin Engine with voucher handler routes.
// Uses a real Service backed by a mock repository and miniredis.
// Returns the Engine, mock repo, and miniredis for test case configuration.
func setUpVoucherHandler(t *testing.T) (*gin.Engine, *mockVoucherRepoForHandler, *miniredis.Miniredis) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	validator.InitValidator()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { require.NoError(t, rdb.Close()) })

	repo := new(mockVoucherRepoForHandler)
	svc := NewService(repo, rdb, nil)
	handler := NewHandler(svc)

	r := gin.New()
	r.POST("/voucher", handler.CreateVoucher)
	r.POST("/voucher/seckill", handler.CreateSeckillVoucher)
	r.GET("/voucher/:id", handler.GetVoucherByID)
	r.GET("/voucher/shop/:shop_id", handler.GetVoucherByShopID)

	return r, repo, mr
}

// decodebody decodes the httptest.ResponseRecorder body into a map.
func decodebody(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	return body
}

// =============================================================================
// CreateVoucher
// =============================================================================

func TestHandler_CreateVoucher(t *testing.T) {
	t.Run("create voucher successfully", func(t *testing.T) {
		r, repo, _ := setUpVoucherHandler(t)
		repo.createVoucherFunc = func(ctx context.Context, voucher *Voucher) error {
			voucher.ID = 10
			require.Equal(t, uint64(1), voucher.ShopID)
			require.Equal(t, "Test Voucher", voucher.Title)
			require.Equal(t, uint(0), voucher.Type)
			return nil
		}

		reqBody := []byte(`{
			"shop_id": 1,
			"title": "Test Voucher",
			"sub_title": "Test Sub",
			"rules": "No rules",
			"pay_value": 100,
			"actual_value": 200,
			"type": 0,
			"stock": 50,
			"begin_time": "2026-01-01T00:00:00Z",
			"end_time": "2026-12-31T23:59:59Z"
		}`)
		req := httptest.NewRequest(http.MethodPost, "/voucher", bytes.NewBuffer(reqBody))
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)
		body := decodebody(t, w)
		require.Equal(t, true, body["success"])
		require.Equal(t, float64(10), body["data"].(float64))
	})

	t.Run("create voucher with invalid params", func(t *testing.T) {
		r, repo, _ := setUpVoucherHandler(t)
		repo.createVoucherFunc = func(ctx context.Context, voucher *Voucher) error {
			t.Fatalf("service should not be called when binding fails")
			return nil
		}

		reqBody := []byte(`{}`)
		req := httptest.NewRequest(http.MethodPost, "/voucher", bytes.NewBuffer(reqBody))
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		body := decodebody(t, w)
		require.Equal(t, false, body["success"])
		require.Equal(t, float64(4001), body["code"].(float64))
	})

	t.Run("create voucher with service error", func(t *testing.T) {
		r, repo, _ := setUpVoucherHandler(t)
		repo.createVoucherFunc = func(ctx context.Context, voucher *Voucher) error {
			return &errmsg.ErrInternalSec
		}

		reqBody := []byte(`{
			"shop_id": 1,
			"title": "Test Voucher",
			"sub_title": "Test Sub",
			"rules": "No rules",
			"pay_value": 100,
			"actual_value": 200,
			"type": 0,
			"stock": 50,
			"begin_time": "2026-01-01T00:00:00Z",
			"end_time": "2026-12-31T23:59:59Z"
		}`)
		req := httptest.NewRequest(http.MethodPost, "/voucher", bytes.NewBuffer(reqBody))
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusInternalServerError, w.Code)
		body := decodebody(t, w)
		require.Equal(t, false, body["success"])
	})
}

// =============================================================================
// CreateSeckillVoucher
// =============================================================================

func TestHandler_CreateSeckillVoucher(t *testing.T) {
	t.Run("create seckill voucher successfully", func(t *testing.T) {
		r, repo, _ := setUpVoucherHandler(t)
		repo.createSeckillVoucherFunc = func(ctx context.Context, v *Voucher, sv *seckillvoucher.SeckillVoucher) error {
			v.ID = 20
			return nil
		}

		reqBody := []byte(`{
			"shop_id": 2,
			"title": "Seckill Deal",
			"sub_title": "Limited time",
			"rules": "First come first serve",
			"pay_value": 50,
			"actual_value": 150,
			"type": 1,
			"stock": 10,
			"begin_time": "2026-01-01T00:00:00Z",
			"end_time": "2026-12-31T23:59:59Z"
		}`)
		req := httptest.NewRequest(http.MethodPost, "/voucher/seckill", bytes.NewBuffer(reqBody))
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)
		body := decodebody(t, w)
		require.Equal(t, true, body["success"])
		require.Equal(t, float64(20), body["data"].(float64))
	})

	t.Run("create seckill voucher with invalid params", func(t *testing.T) {
		r, repo, _ := setUpVoucherHandler(t)
		repo.createSeckillVoucherFunc = func(ctx context.Context, v *Voucher, sv *seckillvoucher.SeckillVoucher) error {
			t.Fatalf("service should not be called when binding fails")
			return nil
		}

		reqBody := []byte(`{}`)
		req := httptest.NewRequest(http.MethodPost, "/voucher/seckill", bytes.NewBuffer(reqBody))
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		body := decodebody(t, w)
		require.Equal(t, false, body["success"])
		require.Equal(t, float64(4001), body["code"].(float64))
	})

	t.Run("create seckill voucher with service error", func(t *testing.T) {
		r, repo, _ := setUpVoucherHandler(t)
		repo.createSeckillVoucherFunc = func(ctx context.Context, v *Voucher, sv *seckillvoucher.SeckillVoucher) error {
			return &errmsg.ErrInternalSec
		}

		reqBody := []byte(`{
			"shop_id": 2,
			"title": "Seckill Deal",
			"sub_title": "Limited time",
			"rules": "First come first serve",
			"pay_value": 50,
			"actual_value": 150,
			"type": 1,
			"stock": 10,
			"begin_time": "2026-01-01T00:00:00Z",
			"end_time": "2026-12-31T23:59:59Z"
		}`)
		req := httptest.NewRequest(http.MethodPost, "/voucher/seckill", bytes.NewBuffer(reqBody))
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusInternalServerError, w.Code)
		body := decodebody(t, w)
		require.Equal(t, false, body["success"])
	})
}

// =============================================================================
// GetVoucherByID
// =============================================================================

func TestHandler_GetVoucherByID(t *testing.T) {
	t.Run("get voucher by ID successfully", func(t *testing.T) {
		r, repo, _ := setUpVoucherHandler(t)
		repo.getVoucherByIDFunc = func(ctx context.Context, id uint64) (*Voucher, error) {
			require.Equal(t, uint64(100), id)
			return &Voucher{
				ID: 100, ShopID: 1, Title: "Test Voucher", SubTitle: "A test voucher",
				Rules: "Rule 1", PayValue: 50, ActualValue: 100, Type: 0, Status: 1, Stock: 30,
				BeginTime: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
				EndTime:   time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC),
			}, nil
		}

		req := httptest.NewRequest(http.MethodGet, "/voucher/100", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)
		body := decodebody(t, w)
		require.Equal(t, true, body["success"])
		data := body["data"].(map[string]any)
		require.Equal(t, float64(100), data["id"])
		require.Equal(t, "Test Voucher", data["title"])
	})

	t.Run("get voucher by non-numeric ID returns ErrInvalidParam", func(t *testing.T) {
		r, repo, _ := setUpVoucherHandler(t)
		repo.getVoucherByIDFunc = func(ctx context.Context, id uint64) (*Voucher, error) {
			t.Fatalf("service should not be called when param is invalid")
			return nil, nil
		}

		req := httptest.NewRequest(http.MethodGet, "/voucher/abc", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		body := decodebody(t, w)
		require.Equal(t, false, body["success"])
		require.Equal(t, float64(4001), body["code"].(float64))
	})

	t.Run("get voucher by ID returns nil", func(t *testing.T) {
		r, repo, _ := setUpVoucherHandler(t)
		repo.getVoucherByIDFunc = func(ctx context.Context, id uint64) (*Voucher, error) {
			return nil, nil
		}

		req := httptest.NewRequest(http.MethodGet, "/voucher/999", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusNotFound, w.Code)
		body := decodebody(t, w)
		require.Equal(t, false, body["success"])
		require.Equal(t, float64(4801), body["code"])
	})

	t.Run("get voucher by ID with service error", func(t *testing.T) {
		r, repo, _ := setUpVoucherHandler(t)
		repo.getVoucherByIDFunc = func(ctx context.Context, id uint64) (*Voucher, error) {
			return nil, &errmsg.ErrInternalSec
		}

		req := httptest.NewRequest(http.MethodGet, "/voucher/100", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusInternalServerError, w.Code)
		body := decodebody(t, w)
		require.Equal(t, false, body["success"])
	})
}

// =============================================================================
// GetVoucherByShopID
// =============================================================================

func TestHandler_GetVoucherByShopID(t *testing.T) {
	t.Run("get vouchers by shop ID successfully", func(t *testing.T) {
		r, repo, _ := setUpVoucherHandler(t)
		repo.getByShopIDFunc = func(ctx context.Context, shopID uint64) ([]Voucher, error) {
			require.Equal(t, uint64(1), shopID)
			return []Voucher{
				{ID: 1, ShopID: 1, Title: "Voucher 1", SubTitle: "S1", Rules: "R1", PayValue: 10, ActualValue: 20, Status: 1},
				{ID: 2, ShopID: 1, Title: "Voucher 2", SubTitle: "S2", Rules: "R2", PayValue: 30, ActualValue: 50, Status: 1},
			}, nil
		}

		req := httptest.NewRequest(http.MethodGet, "/voucher/shop/1", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)
		body := decodebody(t, w)
		require.Equal(t, true, body["success"])
		data := body["data"].([]any)
		require.Len(t, data, 2)
	})

	t.Run("get vouchers by non-numeric shop ID returns ErrInvalidParam", func(t *testing.T) {
		r, repo, _ := setUpVoucherHandler(t)
		repo.getByShopIDFunc = func(ctx context.Context, shopID uint64) ([]Voucher, error) {
			t.Fatalf("service should not be called when param is invalid")
			return nil, nil
		}

		req := httptest.NewRequest(http.MethodGet, "/voucher/shop/abc", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		body := decodebody(t, w)
		require.Equal(t, false, body["success"])
		require.Equal(t, float64(4001), body["code"].(float64))
	})

	t.Run("get vouchers by shop ID with empty result", func(t *testing.T) {
		r, repo, _ := setUpVoucherHandler(t)
		repo.getByShopIDFunc = func(ctx context.Context, shopID uint64) ([]Voucher, error) {
			return []Voucher{}, nil
		}

		req := httptest.NewRequest(http.MethodGet, "/voucher/shop/999", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)
		body := decodebody(t, w)
		require.Equal(t, true, body["success"])
		data := body["data"].([]any)
		require.Len(t, data, 0)
	})

	t.Run("get vouchers by shop ID with service error", func(t *testing.T) {
		r, repo, _ := setUpVoucherHandler(t)
		repo.getByShopIDFunc = func(ctx context.Context, shopID uint64) ([]Voucher, error) {
			return nil, &errmsg.ErrInternalSec
		}

		req := httptest.NewRequest(http.MethodGet, "/voucher/shop/1", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusInternalServerError, w.Code)
		body := decodebody(t, w)
		require.Equal(t, false, body["success"])
	})
}
