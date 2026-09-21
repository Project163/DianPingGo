package app

import (
	"context"
	"dianping/internal/cache"
	"dianping/internal/config"
	"dianping/internal/infra"
	"dianping/internal/middleware"
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
	"dianping/internal/session"
	"dianping/internal/tx"
	"dianping/pkg/idgen"
	"dianping/pkg/validator"
	"fmt"
	"log"
	"log/slog"
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
	defer infra.CloseMySQL()
	defer infra.CloseRedis()
	validator.InitValidator()
	txManager := tx.NewGormManager(a.db)
	refreshPool := cache.NewRefreshPool(
		a.rdb,
		10,  // workers
		100, // queue size，当前 20 可能偏小
		0.2,
	)

	metrics := cache.NoopMetrics{} // Prometheus/OTel 实现
	logger := slog.Default()

	breaker := cache.NewRedisBreaker(cache.BreakerConfig{
		Window:          10 * time.Second,
		MinimumRequests: 20,
		FailureRatio:    0.5,
		OpenDuration:    3 * time.Second,
	})

	readRuntime := cache.NewReadRuntime(
		refreshPool,
		breaker,
		logger,
		metrics,
		cache.ReadPolicy{
			RedisReadTimeout:  100 * time.Millisecond,
			RedisWriteTimeout: 200 * time.Millisecond,
			LoaderTimeout:     2 * time.Second,
			DBAcquireTimeout:  300 * time.Millisecond,
			MaxDBConcurrency:  20,
		},
	)

	cacheClient := cache.NewCacheClient(
		a.rdb,
		refreshPool,
		readRuntime,
	)

	cfg := session.DefaultConfig()
	sessionStore, err := session.NewStore(a.rdb, cfg)
	if err != nil {
		log.Fatalf("%v", err)
	}
	authMiddleware := middleware.AuthMiddleware(sessionStore, nil)

	userInfoRepo := userinfo.NewRepository(a.db)
	userInfoSrv := userinfo.NewService(userInfoRepo)

	userRepo := user.NewRepository(a.db)
	userSrv := user.NewService(userRepo, a.rdb, cacheClient)
	userHandler := user.NewHandler(userSrv, userInfoSrv)

	shopRepo := shop.NewRepository(a.db)
	shopSrv := shop.NewService(shopRepo, cacheClient)
	shopHandler := shop.NewHandler(shopSrv)

	shopTypeRepo := shoptype.NewRepository(a.db)
	shopTypeSrv := shoptype.NewService(shopTypeRepo, cacheClient)
	shopTypeHandler := shoptype.NewHandler(shopTypeSrv)

	voucherRepo := voucher.NewRepository(a.db)
	seckillVoucherRepo := seckillvoucher.NewRepository(a.db)
	voucherSrv := voucher.NewService(voucherRepo, a.rdb, cacheClient)
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
	initTaskRepo := seckillvoucher.NewInitTaskRepository(a.db)
	initService := seckillvoucher.NewInitService(
		initTaskRepo,
		seckillVoucherRepo,
		a.rdb,
		txManager,
		logger,
	)
	initWorker := seckillvoucher.NewInitWorker(initService, logger)

	initCtx, stopInit := context.WithCancel(context.Background())
	initDone := make(chan struct{})

	go func() {
		defer close(initDone)
		initWorker.Run(initCtx)
	}()

	// 注册晚于数据库/Redis的关闭 defer，所以执行更早。
	defer func() {
		stopInit()
		<-initDone
	}()

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
		authMiddleware,
	)

	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", a.cfg.Server.Port),
		Handler: r,
	}

	serverErr := make(chan error, 1)
	go func() {
		log.Printf("服务器正在监听端口 %d...", a.cfg.Server.Port)
		serverErr <- server.ListenAndServe()
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(quit)

	var runErr error

	select {
	case <-quit:
		log.Println("收到关闭信号")
	case err := <-serverErr:
		if err != nil && err != http.ErrServerClosed {
			runErr = err
		}
	}

	// 停止初始化任务的后续领取。
	// 等待退出由上面注册的 defer 完成。
	stopInit()

	shutdownCtx, cancel := context.WithTimeout(
		context.Background(), 5*time.Second,
	)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("HTTP 优雅关闭失败：%v", err)
		_ = server.Close()
		if runErr == nil {
			runErr = err
		}
	}

	refreshPool.Shutdown()

	if a.voucherOrderSrv != nil {
		a.voucherOrderSrv.Stop()
	}

	return runErr
}

// TODO: 对象生命周期管理
