package app

import (
	"context"
	"dianping/internal/config"
	"dianping/internal/infra"
	"dianping/internal/router"
	"dianping/pkg/validator"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"
)

type App struct {
	cfg *config.Config
}

// NewApp 创建一个新的App实例，接受配置文件路径作为参数，返回App对象和错误
func NewApp(configPath string) (*App, error) {
	cfg, err := config.InitConfig(configPath)
	if err != nil {
		return nil, fmt.Errorf("初始化配置失败：%w", err)
	}

	_, err = infra.InitMySQL(cfg.MySql, cfg.Server.Mode)
	if err != nil {
		return nil, fmt.Errorf("初始化MySQL失败：%w", err)
	}

	_, err = infra.InitRedis(cfg.Redis)
	if err != nil {
		return nil, fmt.Errorf("初始化Redis失败：%w", err)
	}

	return &App{cfg: cfg}, nil
}

// Start 启动HTTP服务器，监听指定端口，并处理系统中断信号以关闭服务器
func (a *App) Start() error {
	validator.InitValidator()

	r := router.NewRouter(a.cfg.Server.Mode, infra.DB, infra.RedisClient)

	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", a.cfg.Server.Port),
		Handler: r,
	}

	go func() {
		log.Printf("服务器正在监听端口 %d...", a.cfg.Server.Port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("服务器启动失败：%v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt)
	<-quit
	log.Println("收到中断信号，正在关闭服务器...")
	if err := server.Close(); err != nil {
		log.Fatalf("关闭服务器失败：%v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Fatalf("服务器关闭失败：%v", err)
	}
	log.Println("服务器已成功关闭")
	return nil
}

// TODO: 对象生命周期管理
