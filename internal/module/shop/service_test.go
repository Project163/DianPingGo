package shop

import (
	"context"
	"dianping/pkg/errmsg"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

type mockShopRepo struct {
	getShopByIDFunc    func(ctx context.Context, id uint64) (*Shop, error)
	updateShopFunc     func(ctx context.Context, shop *Shop) error
	getShopsByTypeFunc func(ctx context.Context, typeID uint64, offset, limit int) ([]Shop, error)
	getShopsByIDsFunc  func(ctx context.Context, ids []uint64) ([]Shop, error)
	getShopsByNameFunc func(ctx context.Context, name string, offset, limit int) ([]Shop, error)
	CreateShopFunc     func(ctx context.Context, shop *Shop) error
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

func (m *mockShopRepo) CreateShop(ctx context.Context, shop *Shop) error {
	if m.CreateShopFunc != nil {
		return m.CreateShopFunc(ctx, shop)
	}
	return nil
}

func setupService(t *testing.T) (*Service, *mockShopRepo, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	repo := new(mockShopRepo)
	svc := NewService(repo, rdb)
	return svc, repo, mr
}

func TestCreateShop_Service(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		srv, repo, _ := setupService(t)
		repo.CreateShopFunc = func(ctx context.Context, shop *Shop) error {
			require.Equal(t, "New Shop", shop.Name)
			return nil
		}

		err := srv.CreateShop(context.Background(), &Shop{Name: "New Shop"})
		require.NoError(t, err)
	})

	t.Run("repository error", func(t *testing.T) {
		srv, repo, _ := setupService(t)
		repo.CreateShopFunc = func(ctx context.Context, shop *Shop) error {
			return fmt.Errorf("db error")
		}

		err := srv.CreateShop(context.Background(), &Shop{Name: "Fail Shop"})
		require.Error(t, err)
		require.Contains(t, err.Error(), "db error")
	})
}

func TestGetShopByID_Service(t *testing.T) {
	t.Run("cache miss queries database", func(t *testing.T) {
		srv, repo, _ := setupService(t)
		repo.getShopByIDFunc = func(ctx context.Context, id uint64) (*Shop, error) {
			require.Equal(t, uint64(1), id)
			return &Shop{ID: 1, Name: "Test Shop", TypeID: 1, Area: "Area", Address: "Addr"}, nil
		}

		resp, err := srv.GetShopByID(context.Background(), 1)
		require.NoError(t, err)
		require.Equal(t, "Test Shop", resp.Name)
	})

	t.Run("cache hit returns cached data", func(t *testing.T) {
		srv, repo, mr := setupService(t)
		repo.getShopByIDFunc = func(ctx context.Context, id uint64) (*Shop, error) {
			t.Fatal("should not call database when cache hits")
			return nil, nil
		}

		shop := Shop{ID: 2, Name: "Cached Shop", TypeID: 1, Area: "Area", Address: "Addr"}
		bytes, _ := json.Marshal(shop)
		mr.Set("cache:shop:2", string(bytes))

		resp, err := srv.GetShopByID(context.Background(), 2)
		require.NoError(t, err)
		require.Equal(t, "Cached Shop", resp.Name)
	})

	t.Run("not found returns shop not found error", func(t *testing.T) {
		srv, repo, mr := setupService(t)
		repo.getShopByIDFunc = func(ctx context.Context, id uint64) (*Shop, error) {
			return nil, nil
		}
		// 确保缓存未命中
		mr.Del("cache:shop:999")

		_, err := srv.GetShopByID(context.Background(), 999)
		require.Error(t, err)
		require.True(t, errors.Is(err, &errmsg.ErrShopNotFound))
	})

	t.Run("database error", func(t *testing.T) {
		srv, repo, mr := setupService(t)
		mr.Del("cache:shop:1")
		repo.getShopByIDFunc = func(ctx context.Context, id uint64) (*Shop, error) {
			return nil, fmt.Errorf("db connection lost")
		}

		_, err := srv.GetShopByID(context.Background(), 1)
		require.Error(t, err)
		require.Contains(t, err.Error(), "db connection lost")
	})
}

func TestGetShopByIDWithMutex_Service(t *testing.T) {
	t.Run("cache miss queries database", func(t *testing.T) {
		srv, repo, _ := setupService(t)
		repo.getShopByIDFunc = func(ctx context.Context, id uint64) (*Shop, error) {
			return &Shop{ID: 1, Name: "Mutex Shop", TypeID: 1}, nil
		}

		resp, err := srv.GetShopByIDWithMutex(context.Background(), 1)
		require.NoError(t, err)
		require.Equal(t, "Mutex Shop", resp.Name)
	})

	t.Run("not found returns shop not found error", func(t *testing.T) {
		srv, repo, mr := setupService(t)
		mr.Del("cache:shop:999")
		repo.getShopByIDFunc = func(ctx context.Context, id uint64) (*Shop, error) {
			return nil, nil
		}

		_, err := srv.GetShopByIDWithMutex(context.Background(), 999)
		require.True(t, errors.Is(err, &errmsg.ErrShopNotFound))
	})

	t.Run("database error", func(t *testing.T) {
		srv, repo, mr := setupService(t)
		mr.Del("cache:shop:1")
		repo.getShopByIDFunc = func(ctx context.Context, id uint64) (*Shop, error) {
			return nil, fmt.Errorf("db down")
		}

		_, err := srv.GetShopByIDWithMutex(context.Background(), 1)
		require.Error(t, err)
		require.Contains(t, err.Error(), "db down")
	})
}

func TestGetShopByIDWithLogicalExpire_Service(t *testing.T) {
	t.Run("cache miss queries database", func(t *testing.T) {
		srv, repo, _ := setupService(t)
		repo.getShopByIDFunc = func(ctx context.Context, id uint64) (*Shop, error) {
			return &Shop{ID: 3, Name: "Logical Shop", TypeID: 2}, nil
		}

		resp, err := srv.GetShopByIDWithLogicalExpire(context.Background(), 3)
		require.NoError(t, err)
		require.Equal(t, "Logical Shop", resp.Name)
	})

	t.Run("not found returns shop not found error", func(t *testing.T) {
		srv, repo, mr := setupService(t)
		mr.Del("cache:shop:999")
		repo.getShopByIDFunc = func(ctx context.Context, id uint64) (*Shop, error) {
			return nil, nil
		}

		_, err := srv.GetShopByIDWithLogicalExpire(context.Background(), 999)
		require.True(t, errors.Is(err, &errmsg.ErrShopNotFound))
	})
}

func TestUpdate_Service(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		srv, repo, mr := setupService(t)
		shop := &Shop{ID: 1, Name: "Old", TypeID: 1, Area: "Old Area"}
		repo.getShopByIDFunc = func(ctx context.Context, id uint64) (*Shop, error) {
			return shop, nil
		}
		repo.updateShopFunc = func(ctx context.Context, s *Shop) error {
			require.Equal(t, "New Name", s.Name)
			require.Equal(t, "New Area", s.Area)
			return nil
		}
		// 预置缓存，验证更新后删除
		mr.Set("cache:shop:1", "old data")

		err := srv.Update(context.Background(), 1, &UpdateShopReq{Name: "New Name", Area: "New Area"})
		require.NoError(t, err)
		require.False(t, mr.Exists("cache:shop:1"))
	})

	t.Run("shop not found", func(t *testing.T) {
		srv, repo, _ := setupService(t)
		repo.getShopByIDFunc = func(ctx context.Context, id uint64) (*Shop, error) {
			return nil, nil
		}

		err := srv.Update(context.Background(), 999, &UpdateShopReq{Name: "X"})
		require.True(t, errors.Is(err, &errmsg.ErrShopNotFound))
	})

	t.Run("repository error on get", func(t *testing.T) {
		srv, repo, _ := setupService(t)
		repo.getShopByIDFunc = func(ctx context.Context, id uint64) (*Shop, error) {
			return nil, fmt.Errorf("db error")
		}

		err := srv.Update(context.Background(), 1, &UpdateShopReq{Name: "X"})
		require.Error(t, err)
		require.Contains(t, err.Error(), "db error")
	})

	t.Run("repository error on update", func(t *testing.T) {
		srv, repo, _ := setupService(t)
		repo.getShopByIDFunc = func(ctx context.Context, id uint64) (*Shop, error) {
			return &Shop{ID: 1, Name: "Old"}, nil
		}
		repo.updateShopFunc = func(ctx context.Context, s *Shop) error {
			return fmt.Errorf("update failed")
		}

		err := srv.Update(context.Background(), 1, &UpdateShopReq{Name: "X"})
		require.Error(t, err)
		require.Contains(t, err.Error(), "update failed")
	})
}

func TestGetShopsByType_Service(t *testing.T) {
	t.Run("without coordinates queries database", func(t *testing.T) {
		srv, repo, _ := setupService(t)
		repo.getShopsByTypeFunc = func(ctx context.Context, typeID uint64, offset, limit int) ([]Shop, error) {
			require.Equal(t, uint64(1), typeID)
			require.Equal(t, 0, offset)
			require.Equal(t, MaxPageSize, limit)
			return []Shop{
				{ID: 1, Name: "Shop A", TypeID: 1},
				{ID: 2, Name: "Shop B", TypeID: 1},
			}, nil
		}

		resp, err := srv.GetShopsByType(context.Background(), 1, 1, nil, nil)
		require.NoError(t, err)
		require.Len(t, resp, 2)
		require.Equal(t, "Shop A", resp[0].Name)
	})

	t.Run("without coordinates database error", func(t *testing.T) {
		srv, repo, _ := setupService(t)
		repo.getShopsByTypeFunc = func(ctx context.Context, typeID uint64, offset, limit int) ([]Shop, error) {
			return nil, fmt.Errorf("db error")
		}

		_, err := srv.GetShopsByType(context.Background(), 1, 1, nil, nil)
		require.Error(t, err)
	})

	t.Run("with coordinates uses geo search", func(t *testing.T) {
		mr := miniredis.RunT(t)
		rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
		repo := new(mockShopRepo)
		srv := NewService(repo, rdb)

		geoKey := "shop:geo:1"
		rdb.GeoAdd(context.Background(), geoKey, &redis.GeoLocation{Name: "10", Longitude: 116.397, Latitude: 39.908})
		rdb.GeoAdd(context.Background(), geoKey, &redis.GeoLocation{Name: "20", Longitude: 116.398, Latitude: 39.909})

		repo.getShopsByIDsFunc = func(ctx context.Context, ids []uint64) ([]Shop, error) {
			shops := make([]Shop, 0, len(ids))
			for _, id := range ids {
				shops = append(shops, Shop{ID: id, Name: fmt.Sprintf("Shop%d", id), TypeID: 1})
			}
			return shops, nil
		}

		x, y := 116.397, 39.908
		resp, err := srv.GetShopsByType(context.Background(), 1, 1, &x, &y)
		require.NoError(t, err)
		require.NotEmpty(t, resp)
	})

	t.Run("with coordinates empty result", func(t *testing.T) {
		mr := miniredis.RunT(t)
		rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
		srv := NewService(new(mockShopRepo), rdb)

		x, y := 116.397, 39.908
		resp, err := srv.GetShopsByType(context.Background(), 999, 1, &x, &y)

		require.NoError(t, err)
		require.Empty(t, resp)
	})
}

func TestShopToResponse(t *testing.T) {
	t.Run("normal shop", func(t *testing.T) {
		shop := &Shop{
			ID: 1, Name: "Test", TypeID: 2, Images: "img.png",
			Area: "Area", Address: "Addr",
			Longitude: 116.397, Latitude: 39.908,
			AvgPrice: 100, Sold: 50, Comments: 10, Score: 4,
			OpenTime: "09:00-22:00", Distance: 1.5,
		}

		resp := ShopToResponse(shop)
		require.NotNil(t, resp)
		require.Equal(t, uint64(1), resp.ID)
		require.Equal(t, "Test", resp.Name)
		require.Equal(t, uint64(2), resp.TypeID)
		require.Equal(t, "img.png", resp.Images)
		require.Equal(t, "Area", resp.Area)
		require.Equal(t, "Addr", resp.Address)
		require.Equal(t, 116.397, resp.Longitude)
		require.Equal(t, 39.908, resp.Latitude)
		require.Equal(t, uint64(100), resp.AvgPrice)
		require.Equal(t, uint(50), resp.Sold)
		require.Equal(t, uint(10), resp.Comments)
		require.Equal(t, uint(4), resp.Score)
		require.Equal(t, "09:00-22:00", resp.OpenTime)
		require.Equal(t, 1.5, resp.Distance)
	})

	t.Run("nil shop", func(t *testing.T) {
		resp := ShopToResponse(nil)
		require.Nil(t, resp)
	})
}

func TestBatchShopToResponse(t *testing.T) {
	t.Run("normal list", func(t *testing.T) {
		shops := []Shop{
			{ID: 1, Name: "Shop1", TypeID: 1},
			{ID: 2, Name: "Shop2", TypeID: 2},
		}
		resp := batchShopToResponse(shops)
		require.Len(t, resp, 2)
		require.Equal(t, "Shop1", resp[0].Name)
		require.Equal(t, "Shop2", resp[1].Name)
	})

	t.Run("empty list", func(t *testing.T) {
		resp := batchShopToResponse(nil)
		require.Empty(t, resp)
		resp = batchShopToResponse([]Shop{})
		require.Empty(t, resp)
	})
}
