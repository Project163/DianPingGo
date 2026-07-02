package app

import (
	"context"
	"dianping/internal/config"
	"dianping/internal/infra"
	"dianping/internal/module/blog"
	"dianping/internal/module/follow"
	"dianping/internal/module/seckillvoucher"
	"dianping/internal/module/shop"
	"dianping/internal/module/upload"
	"dianping/internal/module/user"
	"dianping/internal/module/userinfo"
	"dianping/internal/module/voucher"
	"dianping/internal/module/voucherorder"
	"dianping/internal/router"
	"dianping/pkg/idgen"
	"dianping/pkg/validator"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

type App struct {
	cfg *config.Config

	voucherOrderSrv *voucherorder.Service
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

	userInfoRepo := userinfo.NewRepository(infra.DB)
	userInfoSrv := userinfo.NewService(userInfoRepo)

	userRepo := user.NewRepository(infra.DB)
	userSrv := user.NewService(userRepo, infra.RedisClient)
	userHandler := user.NewHandler(userSrv, *userInfoSrv)

	shopRepo := shop.NewRepository(infra.DB)
	shopSrv := shop.NewService(shopRepo, infra.RedisClient)
	shopHandler := shop.NewHandler(shopSrv)

	voucherRepo := voucher.NewRepository(infra.DB)
	seckillVoucherRepo := seckillvoucher.NewRepository(infra.DB)
	voucherSrv := voucher.NewService(voucherRepo, infra.RedisClient)
	voucherHandler := voucher.NewHandler(voucherSrv)

	voucherOrderRepo := voucherorder.NewRepository(infra.DB)
	a.voucherOrderSrv = voucherorder.NewService(voucherOrderRepo, seckillVoucherRepo, infra.RedisClient, idgen.NewRedisIDWorker(infra.RedisClient))
	voucherOrderHandler := voucherorder.NewHandler(a.voucherOrderSrv)

	uploadSrv := upload.NewService()
	uploadHandler := upload.NewHandler(uploadSrv)

	followRepo := follow.NewRepository(infra.DB)
	followSrv := follow.NewService(followRepo, userSrv, infra.RedisClient)
	followHandler := follow.NewHandler(followSrv)

	blogRepo := blog.NewRepository(infra.DB)
	blogSrv := blog.NewService(blogRepo, infra.RedisClient, userSrv, followSrv)
	blogHandler := blog.NewHandler(blogSrv)

	a.voucherOrderSrv.Start()

	r := router.NewRouter(a.cfg.Server.Mode, infra.DB, infra.RedisClient, userHandler, shopHandler, voucherHandler, voucherOrderHandler, uploadHandler, followHandler, blogHandler)

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
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("收到中断信号，正在关闭服务器...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Fatalf("服务器关闭失败：%v", err)
	}
	log.Println("服务器已成功关闭")

	if a.voucherOrderSrv != nil {
		a.voucherOrderSrv.Stop()
	}

	infra.CloseMySQL()
	infra.CloseRedis()

	return nil
}

// TODO: 对象生命周期管理
