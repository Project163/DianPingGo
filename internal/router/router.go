package router

import (
	"dianping/internal/middleware"
	"dianping/internal/module/blog"
	"dianping/internal/module/follow"
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

	followRepo := follow.NewRepository(db)

	blogRepo := blog.NewRepository(db)
	blogSrv := blog.NewService(blogRepo, rdb, userSrv, followRepo)
	blogHandler := blog.NewHandler(blogSrv)

	api := r.Group("/api")
	{
		api.POST("/login/password", userHandler.Login)
		api.POST("/login/code", userHandler.CodeLogin)
		api.POST("/code", userHandler.SendCode)

		shop := api.Group("/shops")
		{
			shop.POST("", shopHandler.CreateShop)
			shop.GET("/:id", shopHandler.GetShopByID)
			shop.PUT("/:id", shopHandler.UpdateShop)
			shop.GET("/type/:type_id", shopHandler.GetShopsByType)
			shop.GET("/name/:name", shopHandler.GetShopsByName)
		}

		voucher := api.Group("/voucher")
		{
			voucher.POST("/normal", voucherHandler.CreateVoucher)
			voucher.POST("/seckill", voucherHandler.CreateSeckillVoucher)
			voucher.GET("/shop/:shopid", voucherHandler.GetVoucherByShopID)
		}

		upload := api.Group("/upload")
		{
			upload.POST("/blog", uploadHandler.UploadImage)
			upload.DELETE("/blog", uploadHandler.DeleteImage)
		}

		api.GET("/blog/hot", blogHandler.GetBlogsHot)
		api.GET("/blog/likes/:id", blogHandler.GetBlogLikesByID)

		auth := api.Group("")
		auth.Use(middleware.AuthMiddleware(rdb))
		{
			auth.POST("/seckill/:voucherId", voucherOrderHandler.SeckillVoucher)

			blog := auth.Group("/blog")
			{
				blog.POST("", blogHandler.CreateBlog)
				api.GET("/blog/:id", blogHandler.GetBlogByID)
				blog.GET("/of/me", blogHandler.GetBlogSelf)
				blog.PUT("/like/:id", blogHandler.LikeBlog)
				blog.GET("/of/user/:id", blogHandler.GetBlogByUserID)
				blog.GET("/of/follow", blogHandler.GetBlogOfFollow)
			}
		}
	}

	r.GET("/ping", func(ctx *gin.Context) {
		ctx.JSON(http.StatusOK, gin.H{"message": "pong"})
	})

	return r
}
