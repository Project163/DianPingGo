package shop

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"dianping/pkg/errmsg"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

// mockShopRepo implements ShopRepository for handler-level testing.
type mockShopRepo struct {
	createShopFunc     func(ctx context.Context, shop *Shop) error
	getShopByIDFunc    func(ctx context.Context, id uint64) (*Shop, error)
	updateShopFunc     func(ctx context.Context, shop *Shop) error
	getShopsByTypeFunc func(ctx context.Context, typeID uint64, offset, limit int) ([]Shop, error)
	getShopsByIDsFunc  func(ctx context.Context, ids []uint64) ([]Shop, error)
	getShopsByNameFunc func(ctx context.Context, name string, offset, limit int) ([]Shop, error)
}

func (m *mockShopRepo) CreateShop(ctx context.Context, shop *Shop) error {
	if m.createShopFunc != nil {
		return m.createShopFunc(ctx, shop)
	}
	return nil
}

func (m *mockShopRepo) GetShopByID(ctx context.Context, id uint64) (*Shop, error) {
	if m.getShopByIDFunc != nil {
		return m.getShopByIDFunc(ctx, id)
	}
	return nil, nil
}

func (m *mockShopRepo) UpdateShop(ctx context.Context, shop *Shop) error {
	if m.updateShopFunc != nil {
		return m.updateShopFunc(ctx, shop)
	}
	return nil
}

func (m *mockShopRepo) GetShopsByType(ctx context.Context, typeID uint64, offset, limit int) ([]Shop, error) {
	if m.getShopsByTypeFunc != nil {
		return m.getShopsByTypeFunc(ctx, typeID, offset, limit)
	}
	return nil, nil
}

func (m *mockShopRepo) GetShopsByIDs(ctx context.Context, ids []uint64) ([]Shop, error) {
	if m.getShopsByIDsFunc != nil {
		return m.getShopsByIDsFunc(ctx, ids)
	}
	return nil, nil
}

func (m *mockShopRepo) GetShopsByName(ctx context.Context, name string, offset, limit int) ([]Shop, error) {
	if m.getShopsByNameFunc != nil {
		return m.getShopsByNameFunc(ctx, name, offset, limit)
	}
	return nil, nil
}

// setUpShopHandler creates a test gin engine with all shop routes registered.
// Returns the engine, mock repository, and miniredis instance.
func setUpShopHandler(t *testing.T) (*gin.Engine, *mockShopRepo, *miniredis.Miniredis) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() {
		require.NoError(t, rdb.Close())
	})

	repo := new(mockShopRepo)
	svc := NewService(repo, rdb, nil)
	handler := NewHandler(svc)

	r := gin.New()
	r.POST("/api/shops", handler.CreateShop)
	r.GET("/api/shops/:id", handler.GetShopByID)
	r.PUT("/api/shops/:id", handler.UpdateShop)
	r.GET("/api/shops/of/type", handler.GetShopsByType)
	r.GET("/api/shops/of/name", handler.GetShopsByName)

	return r, repo, mr
}

// decodebody decodes the response body into a map for assertions.
func decodebody(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	return body
}

// =============================================================================
// CreateShop
// =============================================================================

func TestHandler_CreateShop(t *testing.T) {
	t.Run("create shop successfully", func(t *testing.T) {
		r, repo, _ := setUpShopHandler(t)
		repo.createShopFunc = func(ctx context.Context, shop *Shop) error {
			require.Equal(t, "Test Shop", shop.Name)
			require.Equal(t, uint64(1), shop.TypeID)
			shop.ID = 100
			return nil
		}
		reqBody := []byte(`{"name":"Test Shop","type_id":1,"images":"img.jpg","area":"Test Area","address":"Test Address","open_time":"10:00-22:00"}`)
		req := httptest.NewRequest(http.MethodPost, "/api/shops", bytes.NewBuffer(reqBody))
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)
		body := decodebody(t, w)
		require.Equal(t, true, body["success"])
		data := body["data"].(map[string]any)
		require.Equal(t, float64(100), data["id"])
		require.Equal(t, "Test Shop", data["name"])
	})

	t.Run("create shop with invalid JSON returns ErrInvalidParam", func(t *testing.T) {
		r, repo, _ := setUpShopHandler(t)
		repo.createShopFunc = func(ctx context.Context, shop *Shop) error {
			t.Fatalf("service should not be called when binding fails")
			return nil
		}
		reqBody := []byte(`{"name":""}`)
		req := httptest.NewRequest(http.MethodPost, "/api/shops", bytes.NewBuffer(reqBody))
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusBadRequest, w.Code)
		body := decodebody(t, w)
		require.Equal(t, false, body["success"])
		require.Equal(t, float64(4001), body["code"].(float64))
	})

	t.Run("create shop with missing required field returns ErrInvalidParam", func(t *testing.T) {
		r, repo, _ := setUpShopHandler(t)
		repo.createShopFunc = func(ctx context.Context, shop *Shop) error {
			t.Fatalf("service should not be called when binding fails")
			return nil
		}
		// Missing type_id (required)
		reqBody := []byte(`{"name":"Test Shop","area":"Area","address":"Addr","open_time":"10:00"}`)
		req := httptest.NewRequest(http.MethodPost, "/api/shops", bytes.NewBuffer(reqBody))
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusBadRequest, w.Code)
		body := decodebody(t, w)
		require.Equal(t, false, body["success"])
	})

	t.Run("create shop with repository error propagates", func(t *testing.T) {
		r, repo, _ := setUpShopHandler(t)
		repo.createShopFunc = func(ctx context.Context, shop *Shop) error {
			return &errmsg.ErrInternalSec
		}
		reqBody := []byte(`{"name":"Test Shop","type_id":1,"images":"img.jpg","area":"Area","address":"Addr","open_time":"10:00"}`)
		req := httptest.NewRequest(http.MethodPost, "/api/shops", bytes.NewBuffer(reqBody))
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusInternalServerError, w.Code)
		body := decodebody(t, w)
		require.Equal(t, false, body["success"])
	})
}

// =============================================================================
// GetShopByID
// =============================================================================

func TestHandler_GetShopByID(t *testing.T) {
	t.Run("get shop by ID successfully", func(t *testing.T) {
		r, repo, _ := setUpShopHandler(t)
		repo.getShopByIDFunc = func(ctx context.Context, id uint64) (*Shop, error) {
			require.Equal(t, uint64(1), id)
			return &Shop{
				ID: 1, Name: "Test Shop", TypeID: 1, Area: "Test Area",
				Address: "Test Address", OpenTime: "10:00-22:00",
			}, nil
		}
		req := httptest.NewRequest(http.MethodGet, "/api/shops/1", nil)

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)
		body := decodebody(t, w)
		require.Equal(t, true, body["success"])
		data := body["data"].(map[string]any)
		require.Equal(t, float64(1), data["id"])
		require.Equal(t, "Test Shop", data["name"])
	})

	t.Run("get shop by non-numeric ID returns ErrInvalidParam", func(t *testing.T) {
		r, repo, _ := setUpShopHandler(t)
		repo.getShopByIDFunc = func(ctx context.Context, id uint64) (*Shop, error) {
			t.Fatalf("service should not be called when param is invalid")
			return nil, nil
		}
		req := httptest.NewRequest(http.MethodGet, "/api/shops/abc", nil)

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		body := decodebody(t, w)
		require.Equal(t, false, body["success"])
		require.Equal(t, float64(4001), body["code"].(float64))
	})

	t.Run("get shop not found returns ErrShopNotFound", func(t *testing.T) {
		r, repo, _ := setUpShopHandler(t)
		repo.getShopByIDFunc = func(ctx context.Context, id uint64) (*Shop, error) {
			return nil, nil
		}
		req := httptest.NewRequest(http.MethodGet, "/api/shops/9999", nil)

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusNotFound, w.Code)
		body := decodebody(t, w)
		require.Equal(t, false, body["success"])
		require.Equal(t, float64(4201), body["code"].(float64))
	})

	t.Run("get shop with repository error propagates", func(t *testing.T) {
		r, repo, _ := setUpShopHandler(t)
		repo.getShopByIDFunc = func(ctx context.Context, id uint64) (*Shop, error) {
			return nil, &errmsg.ErrInternalSec
		}
		req := httptest.NewRequest(http.MethodGet, "/api/shops/1", nil)

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusInternalServerError, w.Code)
		body := decodebody(t, w)
		require.Equal(t, false, body["success"])
	})
}

// =============================================================================
// UpdateShop
// =============================================================================

func TestHandler_UpdateShop(t *testing.T) {
	t.Run("update shop successfully", func(t *testing.T) {
		r, repo, _ := setUpShopHandler(t)
		repo.getShopByIDFunc = func(ctx context.Context, id uint64) (*Shop, error) {
			require.Equal(t, uint64(1), id)
			return &Shop{ID: 1, Name: "Old Name", TypeID: 1}, nil
		}
		repo.updateShopFunc = func(ctx context.Context, shop *Shop) error {
			require.Equal(t, "Updated Name", shop.Name)
			return nil
		}
		reqBody := []byte(`{"name":"Updated Name","type_id":2,"images":"img.jpg","area":"New Area","address":"New Addr","open_time":"09:00-21:00"}`)
		req := httptest.NewRequest(http.MethodPut, "/api/shops/1", bytes.NewBuffer(reqBody))
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)
		body := decodebody(t, w)
		require.Equal(t, true, body["success"])
	})

	t.Run("update shop with invalid ID returns ErrInvalidParam", func(t *testing.T) {
		r, repo, _ := setUpShopHandler(t)
		repo.getShopByIDFunc = func(ctx context.Context, id uint64) (*Shop, error) {
			t.Fatalf("service should not be called when param is invalid")
			return nil, nil
		}
		reqBody := []byte(`{"name":"Updated Name"}`)
		req := httptest.NewRequest(http.MethodPut, "/api/shops/abc", bytes.NewBuffer(reqBody))
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		body := decodebody(t, w)
		require.Equal(t, false, body["success"])
		require.Equal(t, float64(4001), body["code"].(float64))
	})

	t.Run("update shop with invalid JSON returns ErrInvalidParam", func(t *testing.T) {
		r, repo, _ := setUpShopHandler(t)
		repo.getShopByIDFunc = func(ctx context.Context, id uint64) (*Shop, error) {
			t.Fatalf("GetShopByID should not be called when binding fails")
			return nil, nil
		}
		reqBody := []byte(`invalid json`)
		req := httptest.NewRequest(http.MethodPut, "/api/shops/1", bytes.NewBuffer(reqBody))
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		body := decodebody(t, w)
		require.Equal(t, false, body["success"])
		require.Equal(t, float64(4001), body["code"].(float64))
	})

	t.Run("update shop not found returns ErrShopNotFound", func(t *testing.T) {
		r, repo, _ := setUpShopHandler(t)
		repo.getShopByIDFunc = func(ctx context.Context, id uint64) (*Shop, error) {
			return nil, nil
		}
		reqBody := []byte(`{"name":"Updated Name"}`)
		req := httptest.NewRequest(http.MethodPut, "/api/shops/9999", bytes.NewBuffer(reqBody))
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusNotFound, w.Code)
		body := decodebody(t, w)
		require.Equal(t, false, body["success"])
		require.Equal(t, float64(4201), body["code"].(float64))
	})

	t.Run("update shop with repository error propagates", func(t *testing.T) {
		r, repo, _ := setUpShopHandler(t)
		repo.getShopByIDFunc = func(ctx context.Context, id uint64) (*Shop, error) {
			return &Shop{ID: 1, Name: "Old"}, nil
		}
		repo.updateShopFunc = func(ctx context.Context, shop *Shop) error {
			return &errmsg.ErrInternalSec
		}
		reqBody := []byte(`{"name":"Updated Name"}`)
		req := httptest.NewRequest(http.MethodPut, "/api/shops/1", bytes.NewBuffer(reqBody))
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusInternalServerError, w.Code)
		body := decodebody(t, w)
		require.Equal(t, false, body["success"])
	})
}

// =============================================================================
// GetShopsByType
// =============================================================================

func TestHandler_GetShopsByType(t *testing.T) {
	t.Run("get shops by type successfully without coordinates", func(t *testing.T) {
		r, repo, _ := setUpShopHandler(t)
		repo.getShopsByTypeFunc = func(ctx context.Context, typeID uint64, offset, limit int) ([]Shop, error) {
			require.Equal(t, uint64(1), typeID)
			require.Equal(t, 0, offset)
			require.Equal(t, MaxPageSize, limit)
			return []Shop{
				{ID: 1, Name: "Shop One", TypeID: 1},
				{ID: 2, Name: "Shop Two", TypeID: 1},
			}, nil
		}
		req := httptest.NewRequest(http.MethodGet, "/api/shops/of/type?type_id=1", nil)

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)
		body := decodebody(t, w)
		require.Equal(t, true, body["success"])
		data := body["data"].([]any)
		require.Len(t, data, 2)
	})

	t.Run("get shops by type without type_id returns ErrInvalidParam", func(t *testing.T) {
		r, repo, _ := setUpShopHandler(t)
		repo.getShopsByTypeFunc = func(ctx context.Context, typeID uint64, offset, limit int) ([]Shop, error) {
			t.Fatalf("service should not be called")
			return nil, nil
		}
		req := httptest.NewRequest(http.MethodGet, "/api/shops/of/type", nil)

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		body := decodebody(t, w)
		require.Equal(t, false, body["success"])
		require.Equal(t, float64(4001), body["code"].(float64))
	})

	t.Run("get shops by type with invalid type_id returns ErrInvalidParam", func(t *testing.T) {
		r, repo, _ := setUpShopHandler(t)
		repo.getShopsByTypeFunc = func(ctx context.Context, typeID uint64, offset, limit int) ([]Shop, error) {
			t.Fatalf("service should not be called")
			return nil, nil
		}
		req := httptest.NewRequest(http.MethodGet, "/api/shops/of/type?type_id=abc", nil)

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		body := decodebody(t, w)
		require.Equal(t, false, body["success"])
		require.Equal(t, float64(4001), body["code"].(float64))
	})

	t.Run("get shops by type with negative current returns ErrInvalidParam", func(t *testing.T) {
		r, repo, _ := setUpShopHandler(t)
		repo.getShopsByTypeFunc = func(ctx context.Context, typeID uint64, offset, limit int) ([]Shop, error) {
			t.Fatalf("service should not be called")
			return nil, nil
		}
		req := httptest.NewRequest(http.MethodGet, "/api/shops/of/type?type_id=1&current=0", nil)

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		body := decodebody(t, w)
		require.Equal(t, false, body["success"])
		require.Equal(t, float64(4001), body["code"].(float64))
	})

	t.Run("get shops by type with paging queries second page", func(t *testing.T) {
		r, repo, _ := setUpShopHandler(t)
		repo.getShopsByTypeFunc = func(ctx context.Context, typeID uint64, offset, limit int) ([]Shop, error) {
			require.Equal(t, uint64(1), typeID)
			require.Equal(t, 5, offset) // page 2: (2-1)*5 = 5
			return []Shop{
				{ID: 6, Name: "Shop Six", TypeID: 1},
			}, nil
		}
		req := httptest.NewRequest(http.MethodGet, "/api/shops/of/type?type_id=1&current=2", nil)

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)
		body := decodebody(t, w)
		require.Equal(t, true, body["success"])
	})

	t.Run("get shops by type with service error propagates", func(t *testing.T) {
		r, repo, _ := setUpShopHandler(t)
		repo.getShopsByTypeFunc = func(ctx context.Context, typeID uint64, offset, limit int) ([]Shop, error) {
			return nil, &errmsg.ErrInternalSec
		}
		req := httptest.NewRequest(http.MethodGet, "/api/shops/of/type?type_id=1", nil)

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusInternalServerError, w.Code)
		body := decodebody(t, w)
		require.Equal(t, false, body["success"])
	})
}

// =============================================================================
// GetShopsByName
// =============================================================================

func TestHandler_GetShopsByName(t *testing.T) {
	t.Run("get shops by name successfully", func(t *testing.T) {
		r, repo, _ := setUpShopHandler(t)
		repo.getShopsByNameFunc = func(ctx context.Context, name string, offset, limit int) ([]Shop, error) {
			require.Equal(t, "茶", name)
			return []Shop{
				{ID: 1, Name: "103茶餐厅", TypeID: 1},
			}, nil
		}
		req := httptest.NewRequest(http.MethodGet, "/api/shops/of/name?name=茶", nil)

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)
		body := decodebody(t, w)
		require.Equal(t, true, body["success"])
		data := body["data"].([]any)
		require.Len(t, data, 1)
	})

	t.Run("get shops by name without name returns ErrInvalidParam", func(t *testing.T) {
		r, repo, _ := setUpShopHandler(t)
		repo.getShopsByNameFunc = func(ctx context.Context, name string, offset, limit int) ([]Shop, error) {
			t.Fatalf("service should not be called")
			return nil, nil
		}
		req := httptest.NewRequest(http.MethodGet, "/api/shops/of/name", nil)

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		body := decodebody(t, w)
		require.Equal(t, false, body["success"])
		require.Equal(t, float64(4001), body["code"].(float64))
	})

	t.Run("get shops by name with negative current returns ErrInvalidParam", func(t *testing.T) {
		r, repo, _ := setUpShopHandler(t)
		repo.getShopsByNameFunc = func(ctx context.Context, name string, offset, limit int) ([]Shop, error) {
			t.Fatalf("service should not be called")
			return nil, nil
		}
		req := httptest.NewRequest(http.MethodGet, "/api/shops/of/name?name=茶&current=-1", nil)

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		body := decodebody(t, w)
		require.Equal(t, false, body["success"])
		require.Equal(t, float64(4001), body["code"].(float64))
	})

	t.Run("get shops by name with service error propagates", func(t *testing.T) {
		r, repo, _ := setUpShopHandler(t)
		repo.getShopsByNameFunc = func(ctx context.Context, name string, offset, limit int) ([]Shop, error) {
			return nil, &errmsg.ErrInternalSec
		}
		req := httptest.NewRequest(http.MethodGet, "/api/shops/of/name?name=茶", nil)

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusInternalServerError, w.Code)
		body := decodebody(t, w)
		require.Equal(t, false, body["success"])
	})
}
