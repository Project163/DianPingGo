package shop

import "time"

const (
	CacheShopKey = "cache:shop:"    // 商户缓存键前缀
	CacheShopTTL = 30 * time.Minute // 商户缓存过期时间
	CacheNullTTL = 2 * time.Minute  // 空值缓存过期时间，避免缓存穿透

	CacheShopLockKey = "lock:shop:"     // 商户互斥锁键前缀
	CacheShopLockTTL = 10 * time.Second // 商户互斥锁过期时间

	LogicalShopTTL = 30 * time.Minute // 商户逻辑过期时间

	CacheShopGeoKey = "shop:geo:" // 商户地理位置键前缀
	MaxPageSize     = 5           // 最大分页大小
	GeoSearchRadius = 5000        // 地理位置搜索半径，单位：米
)
