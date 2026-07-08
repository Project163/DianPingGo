package app

import (
	"context"
	"dianping/internal/cache"
	"dianping/internal/config"
	"dianping/internal/infra"
	"dianping/internal/module/blog"
	"dianping/internal/module/follow"
	"dianping/internal/module/seckillvoucher"
	"dianping/internal/module/shop"
	"dianping/internal/module/shoptype"
	"dianping/internal/module/upload"
	"dianping/internal/module/user"
	"dianping/internal/module/userinfo"
	"dianping/internal/module/voucher"
	"dianping/internal/module/voucherorder"
	"dianping/internal/router"
	"dianping/internal/tx"
	"dianping/pkg/idgen"
	"dianping/pkg/validator"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

type App struct {
	cfg *config.Config

	db  *gorm.DB
	rdb *redis.Client

	voucherOrderSrv *voucherorder.Service
}

// NewApp 创建一个新的App实例，接受配置文件路径作为参数，返回App对象和错误
func NewApp(configPath string) (*App, error) {
	cfg, err := config.InitConfig(configPath)
	if err != nil {
		return nil, fmt.Errorf("初始化配置失败：%w", err)
	}

	db, err := infra.InitMySQL(cfg.MySql, cfg.Server.Mode)
	if err != nil {
		return nil, fmt.Errorf("初始化MySQL失败：%w", err)
	}

	rdb, err := infra.InitRedis(cfg.Redis)
	if err != nil {
		return nil, fmt.Errorf("初始化Redis失败：%w", err)
	}

	return &App{cfg: cfg, db: db, rdb: rdb}, nil
}

// Start 启动HTTP服务器，监听指定端口，并处理系统中断信号以关闭服务器
func (a *App) Start() error {
	validator.InitValidator()
	txManager := tx.NewGormManager(a.db)
	refreshPool := cache.NewRefreshPool(a.rdb, 10, 20, 0.2)

	userInfoRepo := userinfo.NewRepository(a.db)
	userInfoSrv := userinfo.NewService(userInfoRepo)

	userRepo := user.NewRepository(a.db)
	userSrv := user.NewService(userRepo, a.rdb, refreshPool)
	userHandler := user.NewHandler(userSrv, userInfoSrv)

	shopRepo := shop.NewRepository(a.db)
	shopSrv := shop.NewService(shopRepo, a.rdb, refreshPool)
	shopHandler := shop.NewHandler(shopSrv)

	shopTypeRepo := shoptype.NewRepository(a.db)
	shopTypeSrv := shoptype.NewService(shopTypeRepo, a.rdb, refreshPool)
	shopTypeHandler := shoptype.NewHandler(shopTypeSrv)

	voucherRepo := voucher.NewRepository(a.db)
	seckillVoucherRepo := seckillvoucher.NewRepository(a.db)
	voucherSrv := voucher.NewService(voucherRepo, a.rdb, refreshPool)
	voucherHandler := voucher.NewHandler(voucherSrv)

	voucherOrderRepo := voucherorder.NewRepository(a.db)
	a.voucherOrderSrv = voucherorder.NewService(voucherOrderRepo, seckillVoucherRepo, a.rdb, idgen.NewRedisIDWorker(a.rdb), txManager)
	voucherOrderHandler := voucherorder.NewHandler(a.voucherOrderSrv)

	uploadSrv := upload.NewService()
	uploadHandler := upload.NewHandler(uploadSrv)

	followRepo := follow.NewRepository(a.db)
	followSrv := follow.NewService(followRepo, userSrv, a.rdb)
	followHandler := follow.NewHandler(followSrv)

	blogRepo := blog.NewRepository(a.db)
	blogSrv := blog.NewService(blogRepo, infra.RedisClient, userSrv, followSrv)
	blogHandler := blog.NewHandler(blogSrv)

	a.voucherOrderSrv.Start()

	r := router.NewRouter(
		a.cfg.Server.Mode,
		a.db, a.rdb,
		userHandler,
		shopHandler,
		shopTypeHandler,
		voucherHandler,
		voucherOrderHandler,
		uploadHandler,
		followHandler,
		blogHandler,
	)

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
	refreshPool.Shutdown()
	log.Println("服务器已成功关闭")

	if a.voucherOrderSrv != nil {
		a.voucherOrderSrv.Stop()
	}

	defer infra.CloseMySQL()
	defer infra.CloseRedis()

	return nil
}

// TODO: 对象生命周期管理
