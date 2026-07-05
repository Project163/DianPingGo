package router

import (
	"dianping/internal/config"
	"dianping/internal/middleware"
	"dianping/internal/module/blog"
	"dianping/internal/module/follow"
	"dianping/internal/module/shop"
	"dianping/internal/module/shoptype"
	"dianping/internal/module/upload"
	"dianping/internal/module/user"
	"dianping/internal/module/voucher"
	"dianping/internal/module/voucherorder"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

func NewRouter(mode string, db *gorm.DB, rdb redis.Cmdable,
	userHandler *user.Handler,
	shopHandler *shop.Handler,
	shopTypeHandler *shoptype.Handler,
	voucherHandler *voucher.Handler,
	voucherOrderHandler *voucherorder.Handler,
	uploadHandler *upload.Handler,
	followHandler *follow.Handler,
	blogHandler *blog.Handler,
) *gin.Engine {
	if mode == "release" {
		gin.SetMode(gin.ReleaseMode)
	} else {
		gin.SetMode(gin.DebugMode)
	}

	r := gin.Default()

	r.Static("/blogs", config.GlobalConfig.Upload.Dir+"/blogs")

	// jwtSecret := os.Getenv("JWT_SECRET")
	// if jwtSecret == "" {
	// 	jwtSecret = "default_secret"
	// }

	api := r.Group("/api")
	{
		user := api.Group("/user")
		{
			user.POST("/login/password", userHandler.Login)
			user.POST("/login/code", userHandler.CodeLogin)
			user.POST("/code", userHandler.SendCode)
			user.GET("/:id", userHandler.GetUserByID)
			user.GET("/info/:id", userHandler.GetUserInfoByID)
		}

		shop := api.Group("/shops")
		{
			shop.POST("", shopHandler.CreateShop)
			shop.GET("/:id", shopHandler.GetShopByID)
			shop.PUT("/:id", shopHandler.UpdateShop)
			shop.GET("/of/type", shopHandler.GetShopsByType)
			shop.GET("/of/name", shopHandler.GetShopsByName)
		}

		shoptype := api.Group("/shop-type")
		{
			shoptype.GET("/:id", shopTypeHandler.GetShopTypeByID)
			shoptype.GET("/list", shopTypeHandler.GetShopTypeAll)
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

		blog := api.Group("/blog")
		{
			blog.GET("/blog/hot", blogHandler.GetBlogsHot)
			blog.GET("/blog/likes/:id", blogHandler.GetBlogLikesByID)
			blog.GET("/:id", blogHandler.GetBlogByID)
		}

		auth := api.Group("")
		auth.Use(middleware.AuthMiddleware(rdb))
		{
			auth.POST("/seckill/:voucherId", voucherOrderHandler.SeckillVoucher)

			auth.GET("/user/me", userHandler.GetSelf)

			user := auth.Group("/user")
			{
				user.GET("/sign/count", userHandler.SignCount)
				user.PUT("/sign", userHandler.Sign)
			}

			blog := auth.Group("/blog")
			{
				blog.POST("", blogHandler.CreateBlog)
				blog.GET("/of/me", blogHandler.GetBlogSelf)
				blog.PUT("/like/:id", blogHandler.LikeBlog)
				blog.GET("/of/user/:id", blogHandler.GetBlogByUserID)
				blog.GET("/of/follow", blogHandler.GetBlogOfFollow)
			}
			follow := auth.Group("/follow")
			{
				follow.POST("/:id/:isFollow", followHandler.Follow)
				follow.GET("/or/not/:id", followHandler.IsFollowed)
				follow.GET("/common/:id", followHandler.FollowCommon)
				follow.GET("/followed", followHandler.ListFollowedUserIDs)
				follow.GET("/follower", followHandler.ListFollowerUserIDs)
			}
		}
	}

	r.GET("/ping", func(ctx *gin.Context) {
		ctx.JSON(http.StatusOK, gin.H{"message": "pong"})
	})

	return r
}
