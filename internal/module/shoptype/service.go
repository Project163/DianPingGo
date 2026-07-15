package shoptype

import (
	"context"
	"dianping/internal/cache"

	"github.com/redis/go-redis/v9"
)

type ShopTypeRepository interface {
	CreateShopType(ctx context.Context, shopType *ShopType) error
	UpdateShopType(ctx context.Context, shopType *ShopType) error
	GetShopTypeByID(ctx context.Context, shoptypeId uint64) (*ShopType, error)
	GetShopTypeAll(ctx context.Context) ([]ShopType, error)
}

type Service struct {
	repo  ShopTypeRepository
	rdb   redis.Cmdable
	cache *cache.CacheClient
}

func NewService(repo ShopTypeRepository, rdb redis.Cmdable, pool *cache.RefreshPool) *Service {
	return &Service{
		repo:  repo,
		rdb:   rdb,
		cache: cache.NewCacheClient(rdb, pool),
	}
}

func (s *Service) CreateShopType(ctx context.Context, shopType *ShopType) error {
	err := s.repo.CreateShopType(ctx, shopType)
	if err != nil {
		return err
	}
	return nil
}

func (s *Service) UpdateShopType(ctx context.Context, shopType *ShopType) error {
	if err := s.repo.UpdateShopType(ctx, shopType); err != nil {
		return err
	}
	_ = s.cache.Del(ctx, BizShopTypeKey)
	return nil
}

func (s *Service) GetShopTypeByID(ctx context.Context, shopTypeId uint64) (*ShopType, error) {
	shopType, err := s.repo.GetShopTypeByID(ctx, shopTypeId)
	if err != nil {
		return nil, err
	}
	if shopType == nil {
		return nil, nil
	}
	return shopType, nil
}

func (s *Service) GetShopTypeAll(ctx context.Context) ([]ShopType, error) {
	types, found, err := cache.GetOrLoad(
		ctx, s.cache, BizShopTypeKey, BizShopTypeTTL, BizShopTypeNullTTL,
		func(ctx context.Context) ([]ShopType, bool, error) {
			result, err := s.repo.GetShopTypeAll(ctx)
			if err != nil {
				return nil, false, err
			}
			if result == nil {
				result = make([]ShopType, 0)
			}
			return result, true, nil
		},
	)
	if err != nil {
		return nil, err
	}
	if !found {
		return []ShopType{}, nil
	}
	return types, nil
}
