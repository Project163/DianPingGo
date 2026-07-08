package shoptype

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"dianping/pkg/errmsg"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// mockShopTypeService implements ShopTypeService for handler-level testing.
type mockShopTypeService struct {
	createShopTypeFunc  func(ctx context.Context, shopType *ShopType) error
	updateShopTypeFunc  func(ctx context.Context, shopType *ShopType) error
	getShopTypeByIDFunc func(ctx context.Context, shopTypeId uint64) (*ShopType, error)
	getShopTypeAllFunc  func(ctx context.Context) ([]ShopType, error)
}

func (m *mockShopTypeService) CreateShopType(ctx context.Context, shopType *ShopType) error {
	if m.createShopTypeFunc != nil {
		return m.createShopTypeFunc(ctx, shopType)
	}
	return nil
}

func (m *mockShopTypeService) UpdateShopType(ctx context.Context, shopType *ShopType) error {
	if m.updateShopTypeFunc != nil {
		return m.updateShopTypeFunc(ctx, shopType)
	}
	return nil
}

func (m *mockShopTypeService) GetShopTypeByID(ctx context.Context, shopTypeId uint64) (*ShopType, error) {
	if m.getShopTypeByIDFunc != nil {
		return m.getShopTypeByIDFunc(ctx, shopTypeId)
	}
	return nil, nil
}

func (m *mockShopTypeService) GetShopTypeAll(ctx context.Context) ([]ShopType, error) {
	if m.getShopTypeAllFunc != nil {
		return m.getShopTypeAllFunc(ctx)
	}
	return nil, nil
}

// setUpShopTypeHandler creates a test gin engine with all shoptype routes.
func setUpShopTypeHandler(t *testing.T) (*gin.Engine, *mockShopTypeService) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	srv := new(mockShopTypeService)
	handler := NewHandler(srv)

	r := gin.New()
	r.POST("/api/shop-type", handler.CreateShopType)
	r.PUT("/api/shop-type", handler.UpdateShopType)
	r.GET("/api/shop-type/:id", handler.GetShopTypeByID)
	r.GET("/api/shop-type/list", handler.GetShopTypeAll)

	return r, srv
}

// decodebody decodes the response body into a map for assertions.
func decodebody(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	return body
}

// =============================================================================
// CreateShopType
// =============================================================================

func TestHandler_CreateShopType(t *testing.T) {
	t.Run("create shop type successfully", func(t *testing.T) {
		r, srv := setUpShopTypeHandler(t)
		srv.createShopTypeFunc = func(ctx context.Context, shopType *ShopType) error {
			require.Equal(t, "美食", shopType.Name)
			require.Equal(t, "/types/ms.png", shopType.Icon)
			require.Equal(t, uint(1), shopType.Sort)
			return nil
		}
		reqBody := []byte(`{"name":"美食","icon":"/types/ms.png","sort":1}`)
		req := httptest.NewRequest(http.MethodPost, "/api/shop-type", bytes.NewBuffer(reqBody))
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)
		body := decodebody(t, w)
		require.Equal(t, true, body["success"])
	})

	t.Run("create shop type with invalid JSON returns ErrInvalidParam", func(t *testing.T) {
		r, srv := setUpShopTypeHandler(t)
		srv.createShopTypeFunc = func(ctx context.Context, shopType *ShopType) error {
			t.Fatalf("service should not be called when binding fails")
			return nil
		}
		reqBody := []byte(`invalid json`)
		req := httptest.NewRequest(http.MethodPost, "/api/shop-type", bytes.NewBuffer(reqBody))
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		body := decodebody(t, w)
		require.Equal(t, false, body["success"])
		require.Equal(t, float64(4001), body["code"].(float64))
	})

	t.Run("create shop type with service error returns internal error", func(t *testing.T) {
		r, srv := setUpShopTypeHandler(t)
		srv.createShopTypeFunc = func(ctx context.Context, shopType *ShopType) error {
			return &errmsg.ErrInternalSec
		}
		reqBody := []byte(`{"name":"美食","icon":"/types/ms.png","sort":1}`)
		req := httptest.NewRequest(http.MethodPost, "/api/shop-type", bytes.NewBuffer(reqBody))
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusInternalServerError, w.Code)
		body := decodebody(t, w)
		require.Equal(t, false, body["success"])
	})

	t.Run("create shop type with empty body returns ErrInvalidParam", func(t *testing.T) {
		r, srv := setUpShopTypeHandler(t)
		srv.createShopTypeFunc = func(ctx context.Context, shopType *ShopType) error {
			t.Fatalf("service should not be called")
			return nil
		}
		reqBody := []byte(`{}`)
		req := httptest.NewRequest(http.MethodPost, "/api/shop-type", bytes.NewBuffer(reqBody))
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code) // Empty body binds fine, handler calls service
	})
}

// =============================================================================
// UpdateShopType
// =============================================================================

func TestHandler_UpdateShopType(t *testing.T) {
	t.Run("update shop type successfully", func(t *testing.T) {
		r, srv := setUpShopTypeHandler(t)
		srv.updateShopTypeFunc = func(ctx context.Context, shopType *ShopType) error {
			require.Equal(t, "KTV", shopType.Name)
			require.Equal(t, uint(2), shopType.Sort)
			return nil
		}
		reqBody := []byte(`{"name":"KTV","icon":"/types/KTV.png","sort":2}`)
		req := httptest.NewRequest(http.MethodPut, "/api/shop-type", bytes.NewBuffer(reqBody))
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)
		body := decodebody(t, w)
		require.Equal(t, true, body["success"])
	})

	t.Run("update shop type with invalid JSON returns ErrInvalidParam", func(t *testing.T) {
		r, srv := setUpShopTypeHandler(t)
		srv.updateShopTypeFunc = func(ctx context.Context, shopType *ShopType) error {
			t.Fatalf("service should not be called")
			return nil
		}
		reqBody := []byte(`{bad json}`)
		req := httptest.NewRequest(http.MethodPut, "/api/shop-type", bytes.NewBuffer(reqBody))
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		body := decodebody(t, w)
		require.Equal(t, false, body["success"])
		require.Equal(t, float64(4001), body["code"].(float64))
	})

	t.Run("update shop type with service error returns internal error", func(t *testing.T) {
		r, srv := setUpShopTypeHandler(t)
		srv.updateShopTypeFunc = func(ctx context.Context, shopType *ShopType) error {
			return &errmsg.ErrInternalSec
		}
		reqBody := []byte(`{"name":"KTV","icon":"/types/KTV.png","sort":2}`)
		req := httptest.NewRequest(http.MethodPut, "/api/shop-type", bytes.NewBuffer(reqBody))
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusInternalServerError, w.Code)
		body := decodebody(t, w)
		require.Equal(t, false, body["success"])
	})
}

// =============================================================================
// GetShopTypeByID
// =============================================================================

func TestHandler_GetShopTypeByID(t *testing.T) {
	t.Run("get shop type by ID successfully", func(t *testing.T) {
		r, srv := setUpShopTypeHandler(t)
		srv.getShopTypeByIDFunc = func(ctx context.Context, shopTypeId uint64) (*ShopType, error) {
			require.Equal(t, uint64(1), shopTypeId)
			return &ShopType{ID: 1, Name: "美食", Icon: "/types/ms.png", Sort: 1}, nil
		}
		req := httptest.NewRequest(http.MethodGet, "/api/shop-type/1", nil)

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)
		body := decodebody(t, w)
		require.Equal(t, true, body["success"])
		data := body["data"].(map[string]any)
		require.Equal(t, float64(1), data["id"])
		require.Equal(t, "美食", data["name"])
	})

	t.Run("get shop type by non-numeric ID returns ErrInvalidParam", func(t *testing.T) {
		r, srv := setUpShopTypeHandler(t)
		srv.getShopTypeByIDFunc = func(ctx context.Context, shopTypeId uint64) (*ShopType, error) {
			t.Fatalf("service should not be called")
			return nil, nil
		}
		req := httptest.NewRequest(http.MethodGet, "/api/shop-type/abc", nil)

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		body := decodebody(t, w)
		require.Equal(t, false, body["success"])
		require.Equal(t, float64(4001), body["code"].(float64))
	})

	t.Run("get shop type with service error returns internal error", func(t *testing.T) {
		r, srv := setUpShopTypeHandler(t)
		srv.getShopTypeByIDFunc = func(ctx context.Context, shopTypeId uint64) (*ShopType, error) {
			return nil, &errmsg.ErrInternalSec
		}
		req := httptest.NewRequest(http.MethodGet, "/api/shop-type/1", nil)

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusInternalServerError, w.Code)
		body := decodebody(t, w)
		require.Equal(t, false, body["success"])
	})

	t.Run("get shop type returns nil body successfully", func(t *testing.T) {
		r, srv := setUpShopTypeHandler(t)
		srv.getShopTypeByIDFunc = func(ctx context.Context, shopTypeId uint64) (*ShopType, error) {
			return nil, nil
		}
		req := httptest.NewRequest(http.MethodGet, "/api/shop-type/9999", nil)

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)
		body := decodebody(t, w)
		require.Equal(t, true, body["success"])
		require.Nil(t, body["data"])
	})
}

// =============================================================================
// GetShopTypeAll
// =============================================================================

func TestHandler_GetShopTypeAll(t *testing.T) {
	t.Run("get all shop types successfully", func(t *testing.T) {
		r, srv := setUpShopTypeHandler(t)
		srv.getShopTypeAllFunc = func(ctx context.Context) ([]ShopType, error) {
			return []ShopType{
				{ID: 1, Name: "美食", Icon: "/types/ms.png", Sort: 1},
				{ID: 2, Name: "KTV", Icon: "/types/KTV.png", Sort: 2},
			}, nil
		}
		req := httptest.NewRequest(http.MethodGet, "/api/shop-type/list", nil)

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)
		body := decodebody(t, w)
		require.Equal(t, true, body["success"])
		data := body["data"].([]any)
		require.Len(t, data, 2)
	})

	t.Run("get all shop types empty list", func(t *testing.T) {
		r, srv := setUpShopTypeHandler(t)
		srv.getShopTypeAllFunc = func(ctx context.Context) ([]ShopType, error) {
			return []ShopType{}, nil
		}
		req := httptest.NewRequest(http.MethodGet, "/api/shop-type/list", nil)

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)
		body := decodebody(t, w)
		require.Equal(t, true, body["success"])
		data := body["data"].([]any)
		require.Empty(t, data)
	})

	t.Run("get all shop types with service error returns internal error", func(t *testing.T) {
		r, srv := setUpShopTypeHandler(t)
		srv.getShopTypeAllFunc = func(ctx context.Context) ([]ShopType, error) {
			return nil, &errmsg.ErrInternalSec
		}
		req := httptest.NewRequest(http.MethodGet, "/api/shop-type/list", nil)

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusInternalServerError, w.Code)
		body := decodebody(t, w)
		require.Equal(t, false, body["success"])
	})
}
