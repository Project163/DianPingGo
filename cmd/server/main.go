package main

import (
	"dianping/internal/app"
	"flag"
	"log"
)

func main() {
	configPath := flag.String("config", "configs/config.yaml", "配置文件路径")
	flag.Parse()

	application, err := app.NewApp(*configPath)
	if err != nil {
		log.Fatalf("应用初始化失败：%v", err)
	}

	if err := application.Start(); err != nil {
		log.Fatalf("应用启动失败：%v", err)
	}
}
