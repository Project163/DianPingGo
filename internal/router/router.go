package router

import (
	"dianping/internal/module/user"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
)

func NewRouter(mode string) *gin.Engine {
	if mode == "release" {
		gin.SetMode(gin.ReleaseMode)
	} else {
		gin.SetMode(gin.DebugMode)
	}

	r := gin.Default()

	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		jwtSecret = "default_secret"
	}

	userRepo := user.NewRepository()
	userSrv := user.NewService(userRepo, jwtSecret)
	userHandler := user.NewHandler(userSrv)

	api := r.Group("/api")
	{
		api.POST("/login/password", userHandler.Login)
		api.POST("/login/code", userHandler.CodeLogin)
		api.POST("/code", userHandler.SendCode)
	}

	r.GET("/ping", func(ctx *gin.Context) {
		ctx.JSON(http.StatusOK, gin.H{"message": "pong"})
	})

	return r
}
