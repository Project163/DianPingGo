package shop

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"dianping/pkg/errmsg"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

// mockShopSvcRepo implements ShopRepository for service-level testing.
type mockShopSvcRepo struct {
	createShopFunc     func(ctx context.Context, shop *Shop) error
	getShopByIDFunc    func(ctx context.Context, id uint64) (*Shop, error)
	updateShopFunc     func(ctx context.Context, shop *Shop) error
	getShopsByTypeFunc func(ctx context.Context, typeID uint64, offset, limit int) ([]Shop, error)
	getShopsByIDsFunc  func(ctx context.Context, ids []uint64) ([]Shop, error)
	getShopsByNameFunc func(ctx context.Context, name string, offset, limit int) ([]Shop, error)
}

func (m *mockShopSvcRepo) CreateShop(ctx context.Context, shop *Shop) error {
	if m.createShopFunc != nil {
		return m.createShopFunc(ctx, shop)
	}
	return nil
}

func (m *mockShopSvcRepo) GetShopByID(ctx context.Context, id uint64) (*Shop, error) {
	if m.getShopByIDFunc != nil {
		return m.getShopByIDFunc(ctx, id)
	}
	return nil, nil
}

func (m *mockShopSvcRepo) UpdateShop(ctx context.Context, shop *Shop) error {
	if m.updateShopFunc != nil {
		return m.updateShopFunc(ctx, shop)
	}
	return nil
}

func (m *mockShopSvcRepo) GetShopsByType(ctx context.Context, typeID uint64, offset, limit int) ([]Shop, error) {
	if m.getShopsByTypeFunc != nil {
		return m.getShopsByTypeFunc(ctx, typeID, offset, limit)
	}
	return nil, nil
}

func (m *mockShopSvcRepo) GetShopsByIDs(ctx context.Context, ids []uint64) ([]Shop, error) {
	if m.getShopsByIDsFunc != nil {
		return m.getShopsByIDsFunc(ctx, ids)
	}
	return nil, nil
}

func (m *mockShopSvcRepo) GetShopsByName(ctx context.Context, name string, offset, limit int) ([]Shop, error) {
	if m.getShopsByNameFunc != nil {
		return m.getShopsByNameFunc(ctx, name, offset, limit)
	}
	return nil, nil
}

// setUpShopService creates a Service with mock repository and miniredis.
func setUpShopService(t *testing.T) (*Service, *mockShopSvcRepo, *miniredis.Miniredis) {
	t.Helper()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() {
		require.NoError(t, rdb.Close())
	})
	repo := new(mockShopSvcRepo)
	return NewService(repo, rdb, nil), repo, mr
}

// =============================================================================
// CreateShop
// =============================================================================

func TestService_CreateShop(t *testing.T) {
	t.Run("create shop successfully", func(t *testing.T) {
		svc, repo, _ := setUpShopService(t)
		ctx := context.Background()

		repo.createShopFunc = func(ctx context.Context, shop *Shop) error {
			require.Equal(t, "New Shop", shop.Name)
			require.Equal(t, uint64(1), shop.TypeID)
			shop.ID = 100
			return nil
		}

		shop := &Shop{Name: "New Shop", TypeID: 1, Area: "Area", Address: "Addr", OpenTime: "10:00"}
		err := svc.CreateShop(ctx, shop)
		require.NoError(t, err)
		require.Equal(t, uint64(100), shop.ID)
	})

	t.Run("create shop with repository error propagates", func(t *testing.T) {
		svc, repo, _ := setUpShopService(t)
		ctx := context.Background()

		dbErr := errors.New("db connection lost")
		repo.createShopFunc = func(ctx context.Context, shop *Shop) error {
			return dbErr
		}

		err := svc.CreateShop(ctx, &Shop{Name: "Test"})
		require.Error(t, err)
		require.Equal(t, dbErr, err)
	})
}

// =============================================================================
// GetShopByID (with cache pass-through)
// =============================================================================

func TestService_GetShopByID(t *testing.T) {
	t.Run("cache hit returns shop without querying DB", func(t *testing.T) {
		svc, _, mr := setUpShopService(t)
		ctx := context.Background()

		// Pre-warm cache by storing shop data
		cachedShop := Shop{ID: 1, Name: "Cached Shop", TypeID: 1, Area: "Area", Address: "Addr", OpenTime: "10:00-22:00"}
		cacheKey := CacheShopKey + "1"
		bytes := jsonMarshal(cachedShop)
		mr.Set(cacheKey, string(bytes))

		// repo not set — nil panic if DB is called, verifying cache hit
		resp, err := svc.GetShopByID(ctx, 1)
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, uint64(1), resp.ID)
		require.Equal(t, "Cached Shop", resp.Name)
	})

	t.Run("cache miss queries DB and caches result", func(t *testing.T) {
		svc, repo, mr := setUpShopService(t)
		ctx := context.Background()

		repo.getShopByIDFunc = func(ctx context.Context, id uint64) (*Shop, error) {
			require.Equal(t, uint64(2), id)
			return &Shop{ID: 2, Name: "DB Shop", TypeID: 1, Area: "Area", Address: "Addr", OpenTime: "09:00-21:00"}, nil
		}

		resp, err := svc.GetShopByID(ctx, 2)
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, "DB Shop", resp.Name)

		// Verify result was written to cache
		cacheKey := CacheShopKey + "2"
		cached, _ := mr.Get(cacheKey)
		require.NotEmpty(t, cached)
	})

	t.Run("cache miss with no DB record returns ErrShopNotFound", func(t *testing.T) {
		svc, repo, _ := setUpShopService(t)
		ctx := context.Background()

		repo.getShopByIDFunc = func(ctx context.Context, id uint64) (*Shop, error) {
			return nil, nil
		}

		resp, err := svc.GetShopByID(ctx, 9999)
		require.Error(t, err)
		require.Nil(t, resp)
		require.Equal(t, &errmsg.ErrShopNotFound, err)
	})

	t.Run("get shop by ID with DB error propagates", func(t *testing.T) {
		svc, repo, _ := setUpShopService(t)
		ctx := context.Background()

		dbErr := errors.New("db timeout")
		repo.getShopByIDFunc = func(ctx context.Context, id uint64) (*Shop, error) {
			return nil, dbErr
		}

		resp, err := svc.GetShopByID(ctx, 1)
		require.Error(t, err)
		require.Nil(t, resp)
	})
}

// =============================================================================
// GetShopByIDWithMutex
// =============================================================================

func TestService_GetShopByIDWithMutex(t *testing.T) {
	t.Run("cache miss queries DB and caches result", func(t *testing.T) {
		svc, repo, mr := setUpShopService(t)
		ctx := context.Background()

		repo.getShopByIDFunc = func(ctx context.Context, id uint64) (*Shop, error) {
			return &Shop{ID: 3, Name: "Mutex Shop", TypeID: 1, Area: "Area", Address: "Addr", OpenTime: "10:00-22:00"}, nil
		}

		resp, err := svc.GetShopByIDWithMutex(ctx, 3)
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, "Mutex Shop", resp.Name)

		// Verify cached
		cacheKey := CacheShopKey + "3"
		cached, _ := mr.Get(cacheKey)
		require.NotEmpty(t, cached)
	})

	t.Run("cache hit returns shop without querying DB", func(t *testing.T) {
		svc, _, mr := setUpShopService(t)
		ctx := context.Background()

		cachedShop := Shop{ID: 4, Name: "Cached Shop", TypeID: 1, Area: "Area", Address: "Addr", OpenTime: "10:00"}
		cacheKey := CacheShopKey + "4"
		bytes := jsonMarshal(cachedShop)
		mr.Set(cacheKey, string(bytes))

		resp, err := svc.GetShopByIDWithMutex(ctx, 4)
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, "Cached Shop", resp.Name)
	})

	t.Run("cache miss with no DB record returns ErrShopNotFound", func(t *testing.T) {
		svc, repo, _ := setUpShopService(t)
		ctx := context.Background()

		repo.getShopByIDFunc = func(ctx context.Context, id uint64) (*Shop, error) {
			return nil, nil
		}

		resp, err := svc.GetShopByIDWithMutex(ctx, 9999)
		require.Error(t, err)
		require.Nil(t, resp)
		require.Equal(t, &errmsg.ErrShopNotFound, err)
	})
}

// =============================================================================
// GetShopByIDWithLogicalExpire
// =============================================================================

func TestService_GetShopByIDWithLogicalExpire(t *testing.T) {
	t.Run("cache miss queries DB and sets logical expire", func(t *testing.T) {
		svc, repo, mr := setUpShopService(t)
		ctx := context.Background()

		repo.getShopByIDFunc = func(ctx context.Context, id uint64) (*Shop, error) {
			return &Shop{ID: 5, Name: "Logical Shop", TypeID: 1, Area: "Area", Address: "Addr", OpenTime: "10:00-22:00"}, nil
		}

		resp, err := svc.GetShopByIDWithLogicalExpire(ctx, 5)
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, "Logical Shop", resp.Name)

		// Verify cache key exists with RedisData envelope
		cacheKey := CacheShopKey + "5"
		cached, _ := mr.Get(cacheKey)
		require.NotEmpty(t, cached)
	})

	t.Run("cache hit with valid expiry returns cached shop", func(t *testing.T) {
		svc, _, mr := setUpShopService(t)
		ctx := context.Background()

		// Use the service's own SetWithLogicalExpire to populate cache
		shop := Shop{ID: 6, Name: "Fresh Shop", TypeID: 1, Area: "Area", Address: "Addr", OpenTime: "10:00"}
		err := svc.cacheClient.SetWithLogicalExpire(ctx, CacheShopKey+"6", &shop, LogicalShopTTL)
		require.NoError(t, err)

		resp, err := svc.GetShopByIDWithLogicalExpire(ctx, 6)
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, "Fresh Shop", resp.Name)

		_ = mr // miniredis used above
	})

	t.Run("cache hit with no DB record returns ErrShopNotFound", func(t *testing.T) {
		svc, repo, _ := setUpShopService(t)
		ctx := context.Background()

		repo.getShopByIDFunc = func(ctx context.Context, id uint64) (*Shop, error) {
			return nil, nil
		}

		resp, err := svc.GetShopByIDWithLogicalExpire(ctx, 9999)
		require.Error(t, err)
		require.Nil(t, resp)
		require.Equal(t, &errmsg.ErrShopNotFound, err)
	})

	t.Run("db error propagates", func(t *testing.T) {
		svc, repo, _ := setUpShopService(t)
		ctx := context.Background()

		dbErr := errors.New("db error")
		repo.getShopByIDFunc = func(ctx context.Context, id uint64) (*Shop, error) {
			return nil, dbErr
		}

		resp, err := svc.GetShopByIDWithLogicalExpire(ctx, 1)
		require.Error(t, err)
		require.Nil(t, resp)
	})
}

// =============================================================================
// Update
// =============================================================================

func TestService_Update(t *testing.T) {
	t.Run("update shop successfully and invalidates cache", func(t *testing.T) {
		svc, repo, mr := setUpShopService(t)
		ctx := context.Background()

		// Pre-populate cache so we can verify invalidation
		cacheKey := CacheShopKey + "10"
		mr.Set(cacheKey, `{"id":10,"name":"Old Name"}`)

		repo.getShopByIDFunc = func(ctx context.Context, id uint64) (*Shop, error) {
			require.Equal(t, uint64(10), id)
			return &Shop{ID: 10, Name: "Old Name", TypeID: 1}, nil
		}
		repo.updateShopFunc = func(ctx context.Context, shop *Shop) error {
			require.Equal(t, "New Name", shop.Name)
			return nil
		}

		req := &UpdateShopReq{Name: "New Name", TypeID: 2}
		err := svc.Update(ctx, 10, req)
		require.NoError(t, err)

		// Verify cache was deleted
		require.False(t, mr.Exists(cacheKey))
	})

	t.Run("update shop not found returns ErrShopNotFound", func(t *testing.T) {
		svc, repo, _ := setUpShopService(t)
		ctx := context.Background()

		repo.getShopByIDFunc = func(ctx context.Context, id uint64) (*Shop, error) {
			return nil, nil
		}

		req := &UpdateShopReq{Name: "New Name"}
		err := svc.Update(ctx, 9999, req)
		require.Error(t, err)
		require.Equal(t, &errmsg.ErrShopNotFound, err)
	})

	t.Run("update shop with repository error on fetch propagates", func(t *testing.T) {
		svc, repo, _ := setUpShopService(t)
		ctx := context.Background()

		dbErr := errors.New("db error")
		repo.getShopByIDFunc = func(ctx context.Context, id uint64) (*Shop, error) {
			return nil, dbErr
		}

		req := &UpdateShopReq{Name: "New Name"}
		err := svc.Update(ctx, 1, req)
		require.Error(t, err)
	})

	t.Run("update shop with repository error on save propagates", func(t *testing.T) {
		svc, repo, _ := setUpShopService(t)
		ctx := context.Background()

		repo.getShopByIDFunc = func(ctx context.Context, id uint64) (*Shop, error) {
			return &Shop{ID: 1, Name: "Old Name"}, nil
		}
		repo.updateShopFunc = func(ctx context.Context, shop *Shop) error {
			return errors.New("save failed")
		}

		req := &UpdateShopReq{Name: "New Name"}
		err := svc.Update(ctx, 1, req)
		require.Error(t, err)
	})
}

// =============================================================================
// GetShopsByType
// =============================================================================

func TestService_GetShopsByType(t *testing.T) {
	t.Run("get shops by type without coordinates queries DB directly", func(t *testing.T) {
		svc, repo, _ := setUpShopService(t)
		ctx := context.Background()

		repo.getShopsByTypeFunc = func(ctx context.Context, typeID uint64, offset, limit int) ([]Shop, error) {
			require.Equal(t, uint64(1), typeID)
			return []Shop{
				{ID: 1, Name: "Shop A", TypeID: 1, Area: "Area", Address: "Addr", OpenTime: "10:00"},
				{ID: 2, Name: "Shop B", TypeID: 1, Area: "Area", Address: "Addr", OpenTime: "11:00"},
			}, nil
		}

		resp, err := svc.GetShopsByType(ctx, 1, 1, nil, nil)
		require.NoError(t, err)
		require.Len(t, resp, 2)
		require.Equal(t, "Shop A", resp[0].Name)
		require.Equal(t, "Shop B", resp[1].Name)
	})

	t.Run("get shops by type with coordinates uses GeoSearch", func(t *testing.T) {
		svc, repo, mr := setUpShopService(t)
		ctx := context.Background()

		// Populate Redis GEO data via real client (miniredis does not support GeoAdd directly)
		geoKey := CacheShopGeoKey + "1"
		rdbGeo := redis.NewClient(&redis.Options{Addr: mr.Addr()})
		require.NoError(t, rdbGeo.GeoAdd(ctx, geoKey, &redis.GeoLocation{Longitude: 120.15, Latitude: 30.32, Name: "1"}).Err())
		require.NoError(t, rdbGeo.GeoAdd(ctx, geoKey, &redis.GeoLocation{Longitude: 120.16, Latitude: 30.33, Name: "2"}).Err())
		require.NoError(t, rdbGeo.Close())

		repo.getShopsByIDsFunc = func(ctx context.Context, ids []uint64) ([]Shop, error) {
			require.Contains(t, ids, uint64(1))
			require.Contains(t, ids, uint64(2))
			return []Shop{
				{ID: 1, Name: "Geo Shop A", TypeID: 1, Area: "Area", Address: "Addr", OpenTime: "10:00"},
				{ID: 2, Name: "Geo Shop B", TypeID: 1, Area: "Area", Address: "Addr", OpenTime: "11:00"},
			}, nil
		}

		x := 120.15
		y := 30.32
		resp, err := svc.GetShopsByType(ctx, 1, 1, &x, &y)
		require.NoError(t, err)
		require.Len(t, resp, 2)
	})

	t.Run("get shops by type empty result", func(t *testing.T) {
		svc, repo, _ := setUpShopService(t)
		ctx := context.Background()

		repo.getShopsByTypeFunc = func(ctx context.Context, typeID uint64, offset, limit int) ([]Shop, error) {
			return []Shop{}, nil
		}

		resp, err := svc.GetShopsByType(ctx, 1, 1, nil, nil)
		require.NoError(t, err)
		require.Empty(t, resp)
	})

	t.Run("get shops by type with DB error propagates", func(t *testing.T) {
		svc, repo, _ := setUpShopService(t)
		ctx := context.Background()

		dbErr := errors.New("db error")
		repo.getShopsByTypeFunc = func(ctx context.Context, typeID uint64, offset, limit int) ([]Shop, error) {
			return nil, dbErr
		}

		resp, err := svc.GetShopsByType(ctx, 1, 1, nil, nil)
		require.Error(t, err)
		require.Nil(t, resp)
	})
}

// =============================================================================
// GetShopsByName
// =============================================================================

func TestService_GetShopsByName(t *testing.T) {
	t.Run("get shops by name successfully", func(t *testing.T) {
		svc, repo, _ := setUpShopService(t)
		ctx := context.Background()

		repo.getShopsByNameFunc = func(ctx context.Context, name string, offset, limit int) ([]Shop, error) {
			require.Equal(t, "茶", name)
			return []Shop{
				{ID: 1, Name: "茶餐厅", TypeID: 1, Area: "Area", Address: "Addr", OpenTime: "10:00"},
			}, nil
		}

		resp, err := svc.GetShopsByName(ctx, "茶", 1)
		require.NoError(t, err)
		require.Len(t, resp, 1)
		require.Equal(t, "茶餐厅", resp[0].Name)
	})

	t.Run("get shops by name with no results returns empty", func(t *testing.T) {
		svc, repo, _ := setUpShopService(t)
		ctx := context.Background()

		repo.getShopsByNameFunc = func(ctx context.Context, name string, offset, limit int) ([]Shop, error) {
			return []Shop{}, nil
		}

		resp, err := svc.GetShopsByName(ctx, "nonexistent", 1)
		require.NoError(t, err)
		require.Empty(t, resp)
	})

	t.Run("get shops by name with DB error propagates", func(t *testing.T) {
		svc, repo, _ := setUpShopService(t)
		ctx := context.Background()

		dbErr := errors.New("db error")
		repo.getShopsByNameFunc = func(ctx context.Context, name string, offset, limit int) ([]Shop, error) {
			return nil, dbErr
		}

		resp, err := svc.GetShopsByName(ctx, "茶", 1)
		require.Error(t, err)
		require.Nil(t, resp)
	})

	t.Run("get shops by name on second page", func(t *testing.T) {
		svc, repo, _ := setUpShopService(t)
		ctx := context.Background()

		repo.getShopsByNameFunc = func(ctx context.Context, name string, offset, limit int) ([]Shop, error) {
			require.Equal(t, MaxPageSize, offset) // page 2: (2-1)*5
			return []Shop{
				{ID: 6, Name: "茶店6", TypeID: 1, Area: "Area", Address: "Addr", OpenTime: "10:00"},
			}, nil
		}

		resp, err := svc.GetShopsByName(ctx, "茶", 2)
		require.NoError(t, err)
		require.Len(t, resp, 1)
		require.Equal(t, "茶店6", resp[0].Name)
	})
}

// =============================================================================
// ShopToResponse
// =============================================================================

func TestShopToResponse(t *testing.T) {
	t.Run("converts shop to response preserving all fields", func(t *testing.T) {
		shop := &Shop{
			ID: 1, Name: "Test Shop", TypeID: 2, Images: "img.jpg",
			Area: "Test Area", Address: "Test Address",
			Longitude: 120.15, Latitude: 30.32, AvgPrice: 80,
			Sold: 100, Comments: 50, Score: 47, OpenTime: "10:00-22:00",
			Distance: 1.5,
		}
		resp := shopToResponse(shop)
		require.NotNil(t, resp)
		require.Equal(t, uint64(1), resp.ID)
		require.Equal(t, "Test Shop", resp.Name)
		require.Equal(t, uint64(2), resp.TypeID)
		require.Equal(t, "img.jpg", resp.Images)
		require.Equal(t, "Test Area", resp.Area)
		require.Equal(t, "Test Address", resp.Address)
		require.Equal(t, 120.15, resp.Longitude)
		require.Equal(t, 30.32, resp.Latitude)
		require.Equal(t, uint64(80), resp.AvgPrice)
		require.Equal(t, uint(100), resp.Sold)
		require.Equal(t, uint(50), resp.Comments)
		require.Equal(t, uint(47), resp.Score)
		require.Equal(t, "10:00-22:00", resp.OpenTime)
		require.Equal(t, 1.5, resp.Distance)
	})

	t.Run("nil shop returns nil", func(t *testing.T) {
		resp := shopToResponse(nil)
		require.Nil(t, resp)
	})
}

func TestBatchShopToResponse(t *testing.T) {
	t.Run("converts multiple shops to responses", func(t *testing.T) {
		shops := []Shop{
			{ID: 1, Name: "Shop A", TypeID: 1, Area: "A", Address: "A", OpenTime: "10:00"},
			{ID: 2, Name: "Shop B", TypeID: 2, Area: "B", Address: "B", OpenTime: "11:00"},
		}
		resp := batchShopToResponse(shops)
		require.Len(t, resp, 2)
		require.Equal(t, "Shop A", resp[0].Name)
		require.Equal(t, "Shop B", resp[1].Name)
	})

	t.Run("empty input returns empty", func(t *testing.T) {
		resp := batchShopToResponse([]Shop{})
		require.Empty(t, resp)
	})
}

// jsonMarshal helper that panics on error (test-only).
func jsonMarshal(v any) []byte {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return data
}
