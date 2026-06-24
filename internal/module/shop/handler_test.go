package shop

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func setupHandler(t *testing.T) (*Handler, *mockShopRepo) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	repo := new(mockShopRepo)
	svc := NewService(repo, rdb)
	return NewHandler(svc), repo
}

func setupGinContext(method, path string, body any) (*gin.Context, *httptest.ResponseRecorder) {
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

func parseFailResp(t *testing.T, w *httptest.ResponseRecorder) (int, string) {
	t.Helper()
	var resp struct {
		Success bool   `json:"success"`
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	require.False(t, resp.Success)
	return resp.Code, resp.Message
}

func TestHandler_GetShopByID(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		h, repo := setupHandler(t)
		repo.getShopByIDFunc = func(ctx context.Context, id uint64) (*Shop, error) {
			return &Shop{ID: 1, Name: "Test Shop", TypeID: 1, Area: "Area", Address: "Addr"}, nil
		}

		c, w := setupGinContext(http.MethodGet, "/shop/1", nil)
		c.Params = gin.Params{{Key: "id", Value: "1"}}

		h.GetShopByID(c)
		require.Equal(t, http.StatusOK, w.Code)

		shop := parseOKResp[*QueryShopResp](t, w)
		require.Equal(t, "Test Shop", shop.Name)
	})

	t.Run("invalid id param", func(t *testing.T) {
		h, _ := setupHandler(t)
		c, w := setupGinContext(http.MethodGet, "/shop/abc", nil)
		c.Params = gin.Params{{Key: "id", Value: "abc"}}

		h.GetShopByID(c)
		require.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("shop not found", func(t *testing.T) {
		h, repo := setupHandler(t)
		repo.getShopByIDFunc = func(ctx context.Context, id uint64) (*Shop, error) {
			return nil, nil
		}

		c, w := setupGinContext(http.MethodGet, "/shop/999", nil)
		c.Params = gin.Params{{Key: "id", Value: "999"}}

		h.GetShopByID(c)
		require.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("service error", func(t *testing.T) {
		h, repo := setupHandler(t)
		repo.getShopByIDFunc = func(ctx context.Context, id uint64) (*Shop, error) {
			return nil, fmt.Errorf("db down")
		}

		c, w := setupGinContext(http.MethodGet, "/shop/1", nil)
		c.Params = gin.Params{{Key: "id", Value: "1"}}

		h.GetShopByID(c)
		require.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

func TestHandler_UpdateShop(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		h, repo := setupHandler(t)
		shop := &Shop{ID: 1, Name: "Old", TypeID: 1}
		repo.getShopByIDFunc = func(ctx context.Context, id uint64) (*Shop, error) {
			return shop, nil
		}
		repo.updateShopFunc = func(ctx context.Context, s *Shop) error {
			return nil
		}

		req := UpdateShopReq{Name: "New Name", Area: "New Area"}
		c, w := setupGinContext(http.MethodPut, "/shop/1", req)
		c.Params = gin.Params{{Key: "id", Value: "1"}}

		h.UpdateShop(c)
		require.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("invalid id param", func(t *testing.T) {
		h, _ := setupHandler(t)
		c, w := setupGinContext(http.MethodPut, "/shop/abc", nil)
		c.Params = gin.Params{{Key: "id", Value: "abc"}}

		h.UpdateShop(c)
		require.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("invalid json body", func(t *testing.T) {
		h, _ := setupHandler(t)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPut, "/shop/1", strings.NewReader("not json"))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Params = gin.Params{{Key: "id", Value: "1"}}

		h.UpdateShop(c)
		require.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("shop not found", func(t *testing.T) {
		h, repo := setupHandler(t)
		repo.getShopByIDFunc = func(ctx context.Context, id uint64) (*Shop, error) {
			return nil, nil
		}

		c, w := setupGinContext(http.MethodPut, "/shop/999", UpdateShopReq{Name: "X"})
		c.Params = gin.Params{{Key: "id", Value: "999"}}

		h.UpdateShop(c)
		require.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("service error", func(t *testing.T) {
		h, repo := setupHandler(t)
		shop := &Shop{ID: 1, Name: "Old"}
		repo.getShopByIDFunc = func(ctx context.Context, id uint64) (*Shop, error) {
			return shop, nil
		}
		repo.updateShopFunc = func(ctx context.Context, s *Shop) error {
			return fmt.Errorf("update failed")
		}

		c, w := setupGinContext(http.MethodPut, "/shop/1", UpdateShopReq{Name: "X"})
		c.Params = gin.Params{{Key: "id", Value: "1"}}

		h.UpdateShop(c)
		require.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

func TestHandler_GetShopsByType(t *testing.T) {
	t.Run("success without coordinates", func(t *testing.T) {
		h, repo := setupHandler(t)
		repo.getShopsByTypeFunc = func(ctx context.Context, typeID uint64, offset, limit int) ([]Shop, error) {
			return []Shop{
				{ID: 1, Name: "Shop A", TypeID: 1},
				{ID: 2, Name: "Shop B", TypeID: 1},
			}, nil
		}

		c, w := setupGinContext(http.MethodGet, "/shops?typeId=1&current=1", nil)

		h.GetShopsByType(c)
		require.Equal(t, http.StatusOK, w.Code)

		shops := parseOKResp[[]QueryShopResp](t, w)
		require.Len(t, shops, 2)
	})

	t.Run("default current parameter", func(t *testing.T) {
		h, repo := setupHandler(t)
		repo.getShopsByTypeFunc = func(ctx context.Context, typeID uint64, offset, limit int) ([]Shop, error) {
			require.Equal(t, 0, offset) // current=1 → offset=0
			return []Shop{}, nil
		}

		c, w := setupGinContext(http.MethodGet, "/shops?typeId=1", nil)

		h.GetShopsByType(c)
		require.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("invalid typeId", func(t *testing.T) {
		h, _ := setupHandler(t)
		c, w := setupGinContext(http.MethodGet, "/shops?typeId=abc", nil)

		h.GetShopsByType(c)
		require.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("invalid current parameter", func(t *testing.T) {
		h, _ := setupHandler(t)
		c, w := setupGinContext(http.MethodGet, "/shops?typeId=1&current=0", nil)

		h.GetShopsByType(c)
		require.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("service error", func(t *testing.T) {
		h, repo := setupHandler(t)
		repo.getShopsByTypeFunc = func(ctx context.Context, typeID uint64, offset, limit int) ([]Shop, error) {
			return nil, fmt.Errorf("db down")
		}

		c, w := setupGinContext(http.MethodGet, "/shops?typeId=1&current=1", nil)

		h.GetShopsByType(c)
		require.Equal(t, http.StatusInternalServerError, w.Code)
	})
}
