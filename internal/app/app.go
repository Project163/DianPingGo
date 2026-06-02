package app

import (
	"context"
	"dianping/internal/config"
	"dianping/internal/infra"
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

func (a *App) Start() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/ping", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("pong"))
	})

	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", a.cfg.Server.Port),
		Handler: mux,
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
