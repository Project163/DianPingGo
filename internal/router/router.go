package router

import (
	"dianping/internal/middleware"
	"dianping/internal/module/seckillvoucher"
	"dianping/internal/module/shop"
	"dianping/internal/module/upload"
	"dianping/internal/module/user"
	"dianping/internal/module/voucher"
	"dianping/internal/module/voucherorder"
	"dianping/pkg/idgen"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

func NewRouter(mode string, db *gorm.DB, rdb redis.Cmdable) *gin.Engine {
	if mode == "release" {
		gin.SetMode(gin.ReleaseMode)
	} else {
		gin.SetMode(gin.DebugMode)
	}

	r := gin.Default()

	// jwtSecret := os.Getenv("JWT_SECRET")
	// if jwtSecret == "" {
	// 	jwtSecret = "default_secret"
	// }

	userRepo := user.NewRepository(db)
	userSrv := user.NewService(userRepo, rdb)
	userHandler := user.NewHandler(userSrv)

	shopRepo := shop.NewRepository(db)
	shopSrv := shop.NewService(shopRepo, rdb)
	shopHandler := shop.NewHandler(shopSrv)

	voucherRepo := voucher.NewRepository(db)
	seckillVoucherRepo := seckillvoucher.NewRepository(db)
	voucherSrv := voucher.NewService(voucherRepo, rdb)
	voucherHandler := voucher.NewHandler(voucherSrv)
	voucherOrderRepo := voucherorder.NewRepository(db)
	voucherOrderSrv := voucherorder.NewService(voucherOrderRepo, seckillVoucherRepo, rdb, idgen.NewRedisIDWorker(rdb.(*redis.Client)))
	voucherOrderSrv.Start()
	voucherOrderHandler := voucherorder.NewHandler(voucherOrderSrv)

	uploadSrv := upload.NewService()
	uploadHandler := upload.NewHandler(uploadSrv)

	api := r.Group("/api")
	{
		api.POST("/login/password", userHandler.Login)
		api.POST("/login/code", userHandler.CodeLogin)
		api.POST("/code", userHandler.SendCode)

		api.GET("/shops/:id", shopHandler.GetShopByID)
		api.PUT("/shops/:id", shopHandler.UpdateShop)
		api.GET("/shops/type/:type_id", shopHandler.GetShopsByType)
		api.GET("/shops/name/:name", shopHandler.GetShopsByName)
		api.POST("/shops", shopHandler.CreateShop)

		api.POST("/voucher/normal", voucherHandler.CreateVoucher)
		api.POST("/voucher/seckill", voucherHandler.CreateSeckillVoucher)
		api.GET("/voucher/shop/:shopid", voucherHandler.GetVoucherByShopID)

		api.POST("/upload/blog", uploadHandler.UploadImage)
		api.DELETE("/upload/blog", uploadHandler.DeleteImage)

		auth := api.Group("")
		auth.Use(middleware.AuthMiddleware(rdb))
		{
			auth.POST("/seckill/:voucherId", voucherOrderHandler.SeckillVoucher)
		}

	}

	r.GET("/ping", func(ctx *gin.Context) {
		ctx.JSON(http.StatusOK, gin.H{"message": "pong"})
	})

	return r
}
