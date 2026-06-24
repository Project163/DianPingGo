package voucher

import "time"

const (
	CacheVoucherKey     = "cache:voucher:"      // 存储优惠券信息的Redis key模板
	CacheVoucherTTL     = 30 * time.Minute      // 优惠券信息的过期时间
	CacheNullTTL        = 2 * time.Minute       // 缓存空结果的过期时间，防止缓存穿透
	CacheShopVoucherKey = "cache:shop:voucher:" // 存储店铺优惠券列表的Redis key模板
	CacheShopVoucherTTL = 30 * time.Minute      // 店铺优惠券列表的过期时间

	SeckillStockKey = "seckill:stock:" // 存储秒杀库存的Redis key模板
)
