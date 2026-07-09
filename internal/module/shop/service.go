package shop

import (
	"context"
	"dianping/internal/cache"
	"dianping/pkg/errmsg"
	"errors"
	"fmt"
	"sort"
	"strconv"

	"github.com/redis/go-redis/v9"
)

type ShopRepository interface {
	CreateShop(ctx context.Context, shop *Shop) error
	GetShopByID(ctx context.Context, id uint64) (*Shop, error)
	UpdateShop(ctx context.Context, shop *Shop) error
	GetShopsByType(ctx context.Context, typeID uint64, offset, limit int) ([]Shop, error)
	GetShopsByIDs(ctx context.Context, ids []uint64) ([]Shop, error)
	GetShopsByName(ctx context.Context, name string, offset, limit int) ([]Shop, error)
}

type Service struct {
	repo        ShopRepository
	cacheClient *cache.CacheClient
}

func NewService(repo ShopRepository, rdb redis.Cmdable, pool *cache.RefreshPool) *Service {
	return &Service{
		repo:        repo,
		cacheClient: cache.NewCacheClient(rdb, pool),
	}
}

// CreateShop 创建商户
func (s *Service) CreateShop(ctx context.Context, shop *Shop) error {
	err := s.repo.CreateShop(ctx, shop)
	if err != nil {
		return err
	}
	return nil
}

// GetShopByID 获取商户信息，先查缓存，缓存未命中查数据库并写入缓存（缓存空值）
func (s *Service) GetShopByID(ctx context.Context, id uint64) (*QueryShopResp, error) {
	key := fmt.Sprintf("%s%d", CacheShopKey, id)
	var shop Shop

	err := s.cacheClient.QueryWithPassThrough(
		ctx, key, &shop,
		CacheShopTTL, CacheNullTTL,
		func() (any, error) {
			return s.repo.GetShopByID(ctx, id)
		},
	)

	if err != nil {
		if errors.Is(err, cache.ErrDataNotFound) {
			return nil, &errmsg.ErrShopNotFound
		}
		return nil, err
	}
	return shopToResponse(&shop), nil
}

func (s *Service) GetShopByIDWithMutex(ctx context.Context, id uint64) (*QueryShopResp, error) {
	key := fmt.Sprintf("%s%d", CacheShopKey, id)
	var shop Shop

	err := s.cacheClient.QueryWithMutex(
		ctx, key, &shop,
		func() (any, error) {
			return s.repo.GetShopByID(ctx, id)
		},
	)

	if err != nil {
		if errors.Is(err, cache.ErrDataNotFound) {
			return nil, &errmsg.ErrShopNotFound
		}
		return nil, err
	}
	return shopToResponse(&shop), nil
}

func (s *Service) GetShopByIDWithLogicalExpire(ctx context.Context, id uint64) (*QueryShopResp, error) {
	key := fmt.Sprintf("%s%d", CacheShopKey, id)
	var shop Shop

	err := s.cacheClient.QueryWithLogicalExpire(
		ctx, key, &shop,
		LogicalShopTTL,
		func() (any, error) {
			return s.repo.GetShopByID(ctx, id)
		},
	)

	if err != nil {
		if errors.Is(err, cache.ErrDataNotFound) {
			return nil, &errmsg.ErrShopNotFound
		}
		return nil, err
	}
	fmt.Printf("商户信息: %+v\n", shop)
	return shopToResponse(&shop), nil
}

func (s *Service) Update(ctx context.Context, id uint64, req *UpdateShopReq) error {
	shop, err := s.repo.GetShopByID(ctx, id)
	if err != nil {
		return err
	}
	if shop == nil {
		return &errmsg.ErrShopNotFound
	}
	shop.Name = req.Name
	shop.TypeID = req.TypeID
	shop.Images = req.Images
	shop.Area = req.Area
	shop.Address = req.Address
	shop.OpenTime = req.OpenTime
	if err := s.repo.UpdateShop(ctx, shop); err != nil {
		return err
	}

	_ = s.cacheClient.Del(ctx, fmt.Sprintf("%s%d", CacheShopKey, id))
	return nil
}

// GetShopsByType 根据商户类型分页查询商户列表，支持根据坐标进行附近商户查询和排序
func (s *Service) GetShopsByType(ctx context.Context, typeID uint64, current int, x, y *float64) ([]QueryShopResp, error) {
	// 无坐标信息，按数据库查询并分页
	if x == nil || y == nil {
		offset := (current - 1) * MaxPageSize
		shops, err := s.repo.GetShopsByType(ctx, typeID, offset, MaxPageSize)
		if err != nil {
			return nil, err
		}
		return batchShopToResponse(shops), nil
	}

	// 有坐标信息，用Redis GEO 进行附近商户查询并分页
	from := (current - 1) * MaxPageSize
	to := current * MaxPageSize

	geoKey := fmt.Sprintf("%s%d", CacheShopGeoKey, typeID)
	results, err := s.cacheClient.GeoSearch(ctx, geoKey, *x, *y, GeoSearchRadius, to)
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		offset := (current - 1) * MaxPageSize
		shops, err := s.repo.GetShopsByType(ctx, typeID, offset, MaxPageSize)
		if err != nil {
			return nil, err
		}
		return batchShopToResponse(shops), nil
	}
	if len(results) <= from {
		return []QueryShopResp{}, nil
	}

	// 截取当前页的结果，解析商户ID和距离
	pageResults := results[from:]
	ids := make([]uint64, 0, len(pageResults))
	distanceMap := make(map[uint64]float64, len(pageResults))
	for _, res := range pageResults {
		id, _ := strconv.ParseUint(res.Name, 10, 64)
		ids = append(ids, id)
		distanceMap[id] = res.Distance
	}

	shops, err := s.repo.GetShopsByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}

	// 根据 Redis 返回的 ID 顺序排序商户列表
	idOrder := make(map[uint64]int, len(ids))
	for i, id := range ids {
		idOrder[id] = i
	}
	sort.Slice(shops, func(i, j int) bool {
		return idOrder[shops[i].ID] < idOrder[shops[j].ID]
	})
	resp := make([]QueryShopResp, len(shops))
	for i, shop := range shops {
		r := shopToResponse(&shop)
		r.Distance = distanceMap[shop.ID]
		resp[i] = *r
	}
	return resp, nil
}

// GetShopsByName 根据商户名称模糊分页查询商户列表
func (s *Service) GetShopsByName(ctx context.Context, name string, current int) ([]QueryShopResp, error) {
	offset := (current - 1) * MaxPageSize
	shops, err := s.repo.GetShopsByName(ctx, name, offset, MaxPageSize)
	if err != nil {
		return nil, err
	}
	return batchShopToResponse(shops), nil
}

func batchShopToResponse(shops []Shop) []QueryShopResp {
	resps := make([]QueryShopResp, len(shops))
	for i, shop := range shops {
		resps[i] = *shopToResponse(&shop)
	}
	return resps
}

func shopToResponse(shop *Shop) *QueryShopResp {
	if shop == nil {
		return nil
	}
	return &QueryShopResp{
		ID:        shop.ID,
		Name:      shop.Name,
		TypeID:    shop.TypeID,
		Images:    shop.Images,
		Area:      shop.Area,
		Address:   shop.Address,
		Longitude: shop.Longitude,
		Latitude:  shop.Latitude,
		AvgPrice:  shop.AvgPrice,
		Sold:      shop.Sold,
		Comments:  shop.Comments,
		Score:     shop.Score,
		OpenTime:  shop.OpenTime,
		Distance:  shop.Distance,
	}
}
