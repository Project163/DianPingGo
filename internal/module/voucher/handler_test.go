package voucher

import (
	"context"
	"dianping/internal/module/seckillvoucher"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func setupHandler(t *testing.T) (*Handler, *mockVoucherRepo, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	repo := new(mockVoucherRepo)
	svc := NewService(repo, rdb)
	return NewHandler(svc), repo, mr
}

func setupGinContext(method, path string, body any) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(method, path, nil)

	if body != nil {
		jsonBytes, _ := json.Marshal(body)
		c.Request = httptest.NewRequest(method, path, strings.NewReader(string(jsonBytes)))
		c.Request.Header.Set("Content-Type", "application/json")
	}

	return c, w
}

func parseResponse[T any](t *testing.T, w *httptest.ResponseRecorder) T {
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

func TestHandler_CreateVoucher(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		h, repo, _ := setupHandler(t)
		repo.createVoucherFunc = func(ctx context.Context, v *Voucher) error {
			v.ID = 100
			return nil
		}

		now := time.Now()
		c, w := setupGinContext(http.MethodPost, "/voucher", CreateVoucherReq{
			ShopID:      1,
			Title:       "测试券",
			SubTitle:    "副标题",
			Rules:       "规则",
			PayValue:    80,
			ActualValue: 100,
			Type:        0,
			Stock:       50,
			BeginTime:   now,
			EndTime:     now.Add(24 * time.Hour),
		})

		h.CreateVoucher(c)
		require.Equal(t, http.StatusOK, w.Code)

		id := parseResponse[float64](t, w)
		require.Equal(t, float64(100), id)
	})

	t.Run("invalid json body", func(t *testing.T) {
		h, _, _ := setupHandler(t)
		c, w := setupGinContext(http.MethodPost, "/voucher", nil)
		c.Request = httptest.NewRequest(http.MethodPost, "/voucher", strings.NewReader("invalid json"))
		c.Request.Header.Set("Content-Type", "application/json")

		h.CreateVoucher(c)
		require.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("missing required fields", func(t *testing.T) {
		h, _, _ := setupHandler(t)
		c, w := setupGinContext(http.MethodPost, "/voucher", map[string]any{
			"shopId": 1,
		})

		h.CreateVoucher(c)
		require.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("service error", func(t *testing.T) {
		h, repo, _ := setupHandler(t)
		repo.createVoucherFunc = func(ctx context.Context, v *Voucher) error {
			return fmt.Errorf("db connection lost")
		}

		now := time.Now()
		c, w := setupGinContext(http.MethodPost, "/voucher", CreateVoucherReq{
			ShopID:      1,
			Title:       "测试券",
			SubTitle:    "副标题",
			Rules:       "规则",
			PayValue:    80,
			ActualValue: 100,
			Type:        0,
			Stock:       50,
			BeginTime:   now,
			EndTime:     now.Add(24 * time.Hour),
		})

		h.CreateVoucher(c)
		require.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

func TestHandler_CreateSeckillVoucher(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		h, repo, _ := setupHandler(t)
		repo.createSeckillVoucherFunc = func(ctx context.Context, v *Voucher, sv *seckillvoucher.SeckillVoucher) error {
			v.ID = 200
			sv.VoucherID = 200
			return nil
		}

		now := time.Now()
		c, w := setupGinContext(http.MethodPost, "/voucher/seckill", CreateVoucherReq{
			ShopID:      1,
			Title:       "秒杀券",
			SubTitle:    "限时秒杀",
			Rules:       "秒杀规则",
			PayValue:    50,
			ActualValue: 100,
			Type:        1,
			Stock:       10,
			BeginTime:   now,
			EndTime:     now.Add(1 * time.Hour),
		})

		h.CreateSeckillVoucher(c)
		require.Equal(t, http.StatusOK, w.Code)

		id := parseResponse[float64](t, w)
		require.Equal(t, float64(200), id)
	})

	t.Run("invalid json body", func(t *testing.T) {
		h, _, _ := setupHandler(t)
		c, w := setupGinContext(http.MethodPost, "/voucher/seckill", nil)
		c.Request = httptest.NewRequest(http.MethodPost, "/voucher/seckill", strings.NewReader("{"))
		c.Request.Header.Set("Content-Type", "application/json")

		h.CreateSeckillVoucher(c)
		require.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("service error", func(t *testing.T) {
		h, repo, _ := setupHandler(t)
		repo.createSeckillVoucherFunc = func(ctx context.Context, v *Voucher, sv *seckillvoucher.SeckillVoucher) error {
			return fmt.Errorf("transaction failed")
		}

		now := time.Now()
		c, w := setupGinContext(http.MethodPost, "/voucher/seckill", CreateVoucherReq{
			ShopID:      1,
			Title:       "秒杀券",
			SubTitle:    "限时秒杀",
			Rules:       "秒杀规则",
			PayValue:    50,
			ActualValue: 100,
			Type:        1,
			Stock:       10,
			BeginTime:   now,
			EndTime:     now.Add(1 * time.Hour),
		})

		h.CreateSeckillVoucher(c)
		require.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

func TestHandler_GetVoucherByShopID(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		h, repo, _ := setupHandler(t)
		now := time.Now()
		repo.getByShopIDFunc = func(ctx context.Context, shopID uint64) ([]Voucher, error) {
			return []Voucher{
				{
					ID:          1,
					ShopID:      1,
					Title:       "优惠券A",
					SubTitle:    "副标题A",
					Rules:       "规则A",
					PayValue:    80,
					ActualValue: 100,
					Type:        0,
					Status:      1,
					Stock:       50,
					BeginTime:   now,
					EndTime:     now.Add(24 * time.Hour),
				},
			}, nil
		}

		c, w := setupGinContext(http.MethodGet, "/voucher/shop/1", nil)
		c.Params = gin.Params{{Key: "shopid", Value: "1"}}

		h.GetVoucherByShopID(c)
		require.Equal(t, http.StatusOK, w.Code)

		vouchers := parseResponse[[]VoucherResp](t, w)
		require.Len(t, vouchers, 1)
		require.Equal(t, "优惠券A", vouchers[0].Title)
	})

	t.Run("invalid shop id param", func(t *testing.T) {
		h, _, _ := setupHandler(t)
		c, w := setupGinContext(http.MethodGet, "/voucher/shop/abc", nil)
		c.Params = gin.Params{{Key: "shopid", Value: "abc"}}

		h.GetVoucherByShopID(c)
		require.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("service error", func(t *testing.T) {
		h, repo, _ := setupHandler(t)
		repo.getByShopIDFunc = func(ctx context.Context, shopID uint64) ([]Voucher, error) {
			return nil, fmt.Errorf("db connection lost")
		}

		c, w := setupGinContext(http.MethodGet, "/voucher/shop/1", nil)
		c.Params = gin.Params{{Key: "shopid", Value: "1"}}

		h.GetVoucherByShopID(c)
		require.Equal(t, http.StatusInternalServerError, w.Code)
	})
}
