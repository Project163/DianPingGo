package config

import (
	"fmt"
	"log"

	"github.com/spf13/viper"
)

var GlobalConfig *Config

// Config 核心配置结构体，包含服务器、MySQL和Redis
type Config struct {
	Server ServerConfig `mapstructure:"server"`
	MySql  MySqlConfig  `mapstructure:"mysql"`
	Redis  RedisConfig  `mapstructure:"redis"`
	Upload UploadConfig `mapstructure:"upload"`
}

// ServerConfig 服务器配置结构体，包含端口和模式
type ServerConfig struct {
	Port int    `mapstructure:"port"`
	Mode string `mapstructure:"mode"`
}

// MySqlConfig MySQL配置结构体，包含主机、端口、用户、密码和数据库名称
type MySqlConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	User     string `mapstructure:"user"`
	Password string `mapstructure:"password"`
	DBName   string `mapstructure:"dbname"`
}

// RedisConfig Redis配置结构体，包含地址、密码和数据库索引
type RedisConfig struct {
	Addr     string `mapstructure:"addr"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
}

// UploadConfig 上传配置结构体，包含上传目录、最大文件大小和允许的文件类型
type UploadConfig struct {
	Dir          string   `mapstructure:"dir"`
	MaxSize      int64    `mapstructure:"max_size"`
	AllowedTypes []string `mapstructure:"allowed_types"`
}

// InitConfig 初始化配置函数，接受配置文件路径作为参数，返回配置对象和错误
func InitConfig(filePath string) (*Config, error) {
	if filePath != "" {
		viper.SetConfigFile(filePath)
	} else {
		viper.AddConfigPath("configs")
		viper.SetConfigName("config")
		viper.SetConfigType("yaml")
	}

	if err := viper.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("读取配置文件失败：%w", err)
	}

	var config Config
	if err := viper.Unmarshal(&config); err != nil {
		return nil, fmt.Errorf("解析配置文件失败：%w", err)
	}

	GlobalConfig = &config
	log.Printf("配置加载成功!")
	return GlobalConfig, nil
}
