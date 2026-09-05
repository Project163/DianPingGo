package shoptype

import (
	"context"
	"errors"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redismock/v9"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

// mockShopTypeSvcRepo implements ShopTypeRepository for service-level testing.
type mockShopTypeSvcRepo struct {
	createShopTypeFunc  func(ctx context.Context, shopType *ShopType) error
	updateShopTypeFunc  func(ctx context.Context, shopType *ShopType) error
	getShopTypeByIDFunc func(ctx context.Context, shopTypeId uint64) (*ShopType, error)
	getShopTypeAllFunc  func(ctx context.Context) ([]ShopType, error)
}

func (m *mockShopTypeSvcRepo) CreateShopType(ctx context.Context, shopType *ShopType) error {
	if m.createShopTypeFunc != nil {
		return m.createShopTypeFunc(ctx, shopType)
	}
	return nil
}

func (m *mockShopTypeSvcRepo) UpdateShopType(ctx context.Context, shopType *ShopType) error {
	if m.updateShopTypeFunc != nil {
		return m.updateShopTypeFunc(ctx, shopType)
	}
	return nil
}

func (m *mockShopTypeSvcRepo) GetShopTypeByID(ctx context.Context, shopTypeId uint64) (*ShopType, error) {
	if m.getShopTypeByIDFunc != nil {
		return m.getShopTypeByIDFunc(ctx, shopTypeId)
	}
	return nil, nil
}

func (m *mockShopTypeSvcRepo) GetShopTypeAll(ctx context.Context) ([]ShopType, error) {
	if m.getShopTypeAllFunc != nil {
		return m.getShopTypeAllFunc(ctx)
	}
	return nil, nil
}

// setUpShopTypeService creates a Service with mock repository and miniredis.
func setUpShopTypeService(t *testing.T) (*Service, *mockShopTypeSvcRepo, *miniredis.Miniredis) {
	t.Helper()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() {
		require.NoError(t, rdb.Close())
	})
	repo := new(mockShopTypeSvcRepo)
	return NewService(repo, newModuleTestCacheClient(t, rdb)), repo, mr
}

// =============================================================================
// CreateShopType
// =============================================================================

func TestService_CreateShopType(t *testing.T) {
	t.Run("create shop type successfully", func(t *testing.T) {
		svc, repo, _ := setUpShopTypeService(t)
		ctx := context.Background()

		repo.createShopTypeFunc = func(ctx context.Context, shopType *ShopType) error {
			require.Equal(t, "美食", shopType.Name)
			require.Equal(t, "/types/ms.png", shopType.Icon)
			require.Equal(t, uint(1), shopType.Sort)
			return nil
		}

		shopType := &ShopType{Name: "美食", Icon: "/types/ms.png", Sort: 1}
		err := svc.CreateShopType(ctx, shopType)
		require.NoError(t, err)
	})

	t.Run("create shop type with repository error propagates", func(t *testing.T) {
		svc, repo, _ := setUpShopTypeService(t)
		ctx := context.Background()

		dbErr := errors.New("db connection lost")
		repo.createShopTypeFunc = func(ctx context.Context, shopType *ShopType) error {
			return dbErr
		}

		err := svc.CreateShopType(ctx, &ShopType{Name: "Test"})
		require.Error(t, err)
		require.Equal(t, dbErr, err)
	})
}

// =============================================================================
// UpdateShopType
// =============================================================================

func TestService_UpdateShopType(t *testing.T) {
	t.Run("update shop type successfully and invalidates cache", func(t *testing.T) {
		svc, repo, mr := setUpShopTypeService(t)
		ctx := context.Background()

		// Pre-populate cache to verify invalidation
		mr.Set(BizShopTypeKey, `[{"id":1,"name":"Old"}]`)

		repo.updateShopTypeFunc = func(ctx context.Context, shopType *ShopType) error {
			require.Equal(t, "KTV", shopType.Name)
			return nil
		}

		err := svc.UpdateShopType(ctx, &ShopType{ID: 1, Name: "KTV", Icon: "/types/KTV.png", Sort: 2})
		require.NoError(t, err)

		// Verify cache was deleted
		require.False(t, mr.Exists(BizShopTypeKey))
	})

	t.Run("update shop type with repository error propagates", func(t *testing.T) {
		svc, repo, _ := setUpShopTypeService(t)
		ctx := context.Background()

		dbErr := errors.New("db error")
		repo.updateShopTypeFunc = func(ctx context.Context, shopType *ShopType) error {
			return dbErr
		}

		err := svc.UpdateShopType(ctx, &ShopType{ID: 1, Name: "Test"})
		require.Error(t, err)
		require.Equal(t, dbErr, err)
	})
}

// =============================================================================
// GetShopTypeByID
// =============================================================================

func TestService_GetShopTypeByID(t *testing.T) {
	t.Run("get shop type by ID successfully", func(t *testing.T) {
		svc, repo, _ := setUpShopTypeService(t)
		ctx := context.Background()

		repo.getShopTypeByIDFunc = func(ctx context.Context, shopTypeId uint64) (*ShopType, error) {
			require.Equal(t, uint64(1), shopTypeId)
			return &ShopType{ID: 1, Name: "美食", Icon: "/types/ms.png", Sort: 1}, nil
		}

		got, err := svc.GetShopTypeByID(ctx, 1)
		require.NoError(t, err)
		require.NotNil(t, got)
		require.Equal(t, uint64(1), got.ID)
		require.Equal(t, "美食", got.Name)
		require.Equal(t, "/types/ms.png", got.Icon)
		require.Equal(t, uint(1), got.Sort)
	})

	t.Run("get shop type by ID not found returns nil", func(t *testing.T) {
		svc, repo, _ := setUpShopTypeService(t)
		ctx := context.Background()

		repo.getShopTypeByIDFunc = func(ctx context.Context, shopTypeId uint64) (*ShopType, error) {
			return nil, nil
		}

		got, err := svc.GetShopTypeByID(ctx, 9999)
		require.NoError(t, err)
		require.Nil(t, got)
	})

	t.Run("get shop type by ID with repository error propagates", func(t *testing.T) {
		svc, repo, _ := setUpShopTypeService(t)
		ctx := context.Background()

		dbErr := errors.New("db error")
		repo.getShopTypeByIDFunc = func(ctx context.Context, shopTypeId uint64) (*ShopType, error) {
			return nil, dbErr
		}

		got, err := svc.GetShopTypeByID(ctx, 1)
		require.Error(t, err)
		require.Nil(t, got)
	})
}

// =============================================================================
// GetShopTypeAll
// =============================================================================

func TestService_GetShopTypeAll(t *testing.T) {
	t.Run("cache hit returns list without querying DB", func(t *testing.T) {
		svc, _, mr := setUpShopTypeService(t)
		ctx := context.Background()

		// Pre-warm cache with JSON array
		mr.Set(BizShopTypeKey, `[{"id":1,"name":"美食","icon":"/types/ms.png","sort":1},{"id":2,"name":"KTV","icon":"/types/KTV.png","sort":2}]`)

		// repo not set — nil panic if DB is called, verifying cache hit
		got, err := svc.GetShopTypeAll(ctx)
		require.NoError(t, err)
		require.Len(t, got, 2)
		require.Equal(t, uint64(1), got[0].ID)
		require.Equal(t, "美食", got[0].Name)
		require.Equal(t, uint64(2), got[1].ID)
		require.Equal(t, "KTV", got[1].Name)
	})

	t.Run("cache miss queries DB and caches result", func(t *testing.T) {
		svc, repo, mr := setUpShopTypeService(t)
		ctx := context.Background()

		repo.getShopTypeAllFunc = func(ctx context.Context) ([]ShopType, error) {
			return []ShopType{
				{ID: 1, Name: "美食", Icon: "/types/ms.png", Sort: 1},
				{ID: 2, Name: "KTV", Icon: "/types/KTV.png", Sort: 2},
			}, nil
		}

		got, err := svc.GetShopTypeAll(ctx)
		require.NoError(t, err)
		require.Len(t, got, 2)
		require.Equal(t, "美食", got[0].Name)
		require.Equal(t, "KTV", got[1].Name)

		// Verify result was written to cache
		cached := waitForModuleCacheValue(t, mr, BizShopTypeKey)
		require.NotEmpty(t, cached)
	})

	t.Run("empty repository result is cached as JSON array", func(t *testing.T) {
		svc, repo, mr := setUpShopTypeService(t)
		ctx := context.Background()

		loadCount := 0
		repo.getShopTypeAllFunc = func(ctx context.Context) ([]ShopType, error) {
			loadCount++
			return nil, nil
		}

		got, err := svc.GetShopTypeAll(ctx)

		require.NoError(t, err)
		require.NotNil(t, got)
		require.Empty(t, got)

		cacheKey := BizShopTypeKey
		cached := waitForModuleCacheValue(t, mr, cacheKey)
		require.JSONEq(t, `[]`, cached)

		got, err = svc.GetShopTypeAll(ctx)
		require.NoError(t, err)
		require.NotNil(t, got)
		require.Empty(t, got)
		require.Equal(t, 1, loadCount)
	})

	t.Run("get all shop types with DB error propagates", func(t *testing.T) {
		svc, repo, _ := setUpShopTypeService(t)
		ctx := context.Background()

		dbErr := errors.New("db connection lost")
		repo.getShopTypeAllFunc = func(ctx context.Context) ([]ShopType, error) {
			return nil, dbErr
		}

		got, err := svc.GetShopTypeAll(ctx)
		require.Error(t, err)
		require.Nil(t, got)
	})

	t.Run("legacy null marker returns empty array", func(t *testing.T) {
		svc, repo, mr := setUpShopTypeService(t)
		ctx := context.Background()

		mr.Set(BizShopTypeKey, "")

		repo.getShopTypeAllFunc = func(ctx context.Context) ([]ShopType, error) {
			t.Fatal("repository should not be called")
			return nil, nil
		}

		got, err := svc.GetShopTypeAll(ctx)

		require.NoError(t, err)
		require.NotNil(t, got)
		require.Empty(t, got)
	})

	t.Run("corrupted cache reloads from repository", func(t *testing.T) {
		svc, repo, mr := setUpShopTypeService(t)
		ctx := context.Background()

		// Corrupted cache data is discarded and the repository remains the source of truth.
		mr.Set(BizShopTypeKey, `not-valid-json`)

		repo.getShopTypeAllFunc = func(ctx context.Context) ([]ShopType, error) {
			return []ShopType{}, nil
		}

		got, err := svc.GetShopTypeAll(ctx)
		require.NoError(t, err)
		require.Empty(t, got)
	})
}

func TestService_GetShopTypeAll_RedisUnavailable_FallbackToRepository(t *testing.T) {
	rdb, redisMock := redismock.NewClientMock()
	redisMock.ExpectGet(BizShopTypeKey).SetErr(errors.New("redis unavailable"))
	repo := new(mockShopTypeSvcRepo)
	repo.getShopTypeAllFunc = func(context.Context) ([]ShopType, error) {
		return []ShopType{{ID: 1, Name: "美食", Sort: 1}}, nil
	}
	svc := NewService(repo, newModuleTestCacheClientWithoutPool(t, rdb))

	result, err := svc.GetShopTypeAll(context.Background())

	require.NoError(t, err)
	require.Len(t, result, 1)
	require.Equal(t, "美食", result[0].Name)
	require.NoError(t, redisMock.ExpectationsWereMet())
}

func TestService_GetShopTypeAll_NullResult(t *testing.T) {
	t.Run("nil result from repo returns empty slice", func(t *testing.T) {
		svc, repo, mr := setUpShopTypeService(t)
		ctx := context.Background()

		callCount := 0
		repo.getShopTypeAllFunc = func(ctx context.Context) ([]ShopType, error) {
			callCount++
			return nil, nil
		}

		got, err := svc.GetShopTypeAll(ctx)
		require.NoError(t, err)
		require.Empty(t, got)

		// nil repository results are normalized to an empty, found list.
		cached := waitForModuleCacheValue(t, mr, BizShopTypeKey)
		require.JSONEq(t, `[]`, cached)

		// Second call should hit the cached empty list without querying the DB.
		got2, err2 := svc.GetShopTypeAll(ctx)
		require.NoError(t, err2)
		require.Empty(t, got2)
		require.Equal(t, 1, callCount) // DB only called once
	})
}
