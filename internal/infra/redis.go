package infra

import (
	"context"
	"dianping/internal/config"
	"fmt"
	"log"
	"time"

	"github.com/redis/go-redis/v9"
)

var RedisClient *redis.Client

// InitRedis 初始化Redis连接
func InitRedis(cfg config.RedisConfig) (*redis.Client, error) {
	rdb := redis.NewClient(&redis.Options{
		Addr:            cfg.Addr,
		Password:        cfg.Password,
		DB:              cfg.DB,
		PoolSize:        cfg.PoolSize,
		MinIdleConns:    cfg.MinIdleConns,
		ConnMaxLifetime: time.Duration(cfg.ConnMaxLifetime) * time.Second,
		ConnMaxIdleTime: time.Duration(cfg.ConnMaxIdleTime) * time.Second,

		DialTimeout:           time.Duration(cfg.DialTimeout) * time.Second,
		ReadTimeout:           time.Duration(cfg.ReadTimeout) * time.Second,
		WriteTimeout:          time.Duration(cfg.WriteTimeout) * time.Second,
		PoolTimeout:           time.Duration(cfg.PoolTimeout) * time.Second,
		ContextTimeoutEnabled: true,
		MaxRetries:            cfg.MaxRetries,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := rdb.Ping(ctx).Result()
	if err != nil {
		return nil, fmt.Errorf("连接Redis失败：%w", err)
	}

	RedisClient = rdb
	log.Printf("Redis连接成功!")
	return RedisClient, nil
}

func CloseRedis() {
	if RedisClient != nil {
		_, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := RedisClient.Close(); err != nil {
			log.Printf("关闭Redis连接失败：%v", err)
		} else {
			log.Println("Redis连接已关闭")
		}
	}
}
