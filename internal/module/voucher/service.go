package voucher

import (
	"context"
	"dianping/internal/cache"
	"dianping/internal/module/seckillvoucher"
	"fmt"

	"github.com/redis/go-redis/v9"
)

// VoucherRepository 定义了优惠券仓库接口，包含创建优惠券、根据ID查询优惠券、根据商户ID查询优惠券列表和创建秒杀优惠券的方法
type VoucherRepository interface {
	CreateVoucher(ctx context.Context, voucher *Voucher) error
	GetVoucherByID(ctx context.Context, id uint64) (*Voucher, error)
	GetByShopID(ctx context.Context, shopID uint64) ([]Voucher, error)
	CreateSeckillVoucher(ctx context.Context, v *Voucher, sv *seckillvoucher.SeckillVoucher) error
}

// Service 提供优惠券相关的业务逻辑
type Service struct {
	repo  VoucherRepository
	cache *cache.CacheClient
	rdb   redis.Cmdable
}

func NewService(repo VoucherRepository, rdb redis.Cmdable) *Service {
	return &Service{
		repo:  repo,
		cache: cache.NewCacheClient(rdb),
		rdb:   rdb,
	}
}

// CreateVoucher 创建普通券
func (s *Service) CreateVoucher(ctx context.Context, req *CreateVoucherReq) (uint64, error) {
	// 创建普通券
	voucher := &Voucher{
		ShopID:      req.ShopID,
		Type:        req.Type,
		Title:       req.Title,
		SubTitle:    req.SubTitle,
		Rules:       req.Rules,
		PayValue:    req.PayValue,
		ActualValue: req.ActualValue,
		Status:      1,
		Stock:       req.Stock,
		BeginTime:   req.BeginTime,
		EndTime:     req.EndTime,
	}
	if err := s.repo.CreateVoucher(ctx, voucher); err != nil {
		return 0, err
	}
	return voucher.ID, nil
}

// CreateSeckillVoucher 创建秒杀券
func (s *Service) CreateSeckillVoucher(ctx context.Context, req *CreateVoucherReq) (uint64, error) {
	svoucher := &Voucher{
		ShopID:      req.ShopID,
		Type:        1, // 秒杀券
		Title:       req.Title,
		SubTitle:    req.SubTitle,
		Rules:       req.Rules,
		PayValue:    req.PayValue,
		ActualValue: req.ActualValue,
		Status:      1,
		Stock:       req.Stock,
		BeginTime:   req.BeginTime,
		EndTime:     req.EndTime,
	}
	err := s.repo.CreateSeckillVoucher(ctx, svoucher, &seckillvoucher.SeckillVoucher{
		VoucherID: svoucher.ID,
		Stock:     req.Stock,
		BeginTime: req.BeginTime,
		EndTime:   req.EndTime,
	})
	if err != nil {
		return 0, err
	}
	stockKey := fmt.Sprintf("%s%d", SeckillStockKey, svoucher.ID)

	if err := s.rdb.Set(ctx, stockKey, req.Stock, 0).Err(); err != nil {
		return 0, err
	}
	return svoucher.ID, nil
}

// GetVoucherByID 根据优惠券ID查询优惠券
func (s *Service) GetVoucherByShopID(ctx context.Context, shopID uint64) ([]VoucherResp, error) {
	key := fmt.Sprintf("%s%d", CacheShopVoucherKey, shopID)
	var vouchers []Voucher

	// 使用缓存控制策略查询优惠券列表
	err := s.cache.QueryWithPassThrough(ctx, key, &vouchers, CacheShopVoucherTTL, CacheNullTTL,
		func() (any, error) {
			return s.repo.GetByShopID(ctx, shopID)
		})

	if err != nil {
		if err == cache.ErrDataNotFound {
			return []VoucherResp{}, nil
		}
		return nil, err
	}
	return toVoucherRespList(vouchers), nil
}

// toVoucherRespList 将 Voucher 切片转换为 VoucherResp 切片
// 坚持不直接返回 Voucher 对象，而是返回 VoucherResp 对象，以便在响应中隐藏不必要的字段
func toVoucherRespList(vouchers []Voucher) []VoucherResp {
	respList := make([]VoucherResp, len(vouchers))
	for i, v := range vouchers {
		respList[i] = VoucherResp{
			ID:          v.ID,
			ShopID:      v.ShopID,
			Title:       v.Title,
			SubTitle:    v.SubTitle,
			Rules:       v.Rules,
			PayValue:    v.PayValue,
			ActualValue: v.ActualValue,
			Type:        v.Type,
			Status:      v.Status,
			Stock:       v.Stock,
			BeginTime:   v.BeginTime,
			EndTime:     v.EndTime,
		}
	}
	return respList
}
