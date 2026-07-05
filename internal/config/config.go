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

// MySqlConfig MySQL配置结构体，包含主机、端口、用户、密码、数据库名和连接池配置
type MySqlConfig struct {
	Host            string `mapstructure:"host"`
	Port            int    `mapstructure:"port"`
	User            string `mapstructure:"user"`
	Password        string `mapstructure:"password"`
	DBName          string `mapstructure:"dbname"`
	MaxOpenConns    int    `mapstructure:"max_open_conns"`
	MaxIdleConns    int    `mapstructure:"max_idle_conns"`
	ConnMaxLifetime int    `mapstructure:"conn_max_lifetime"`
	ConnMaxIdleTime int    `mapstructure:"conn_max_idle_time"`
}

// RedisConfig Redis配置结构体，包含地址、密码和数据库索引
type RedisConfig struct {
	Addr            string `mapstructure:"addr"`
	Password        string `mapstructure:"password"`
	DB              int    `mapstructure:"db"`
	PoolSize        int    `mapstructure:"pool_size"`
	MinIdleConns    int    `mapstructure:"min_idle_conns"`
	ConnMaxLifetime int    `mapstructure:"conn_max_lifetime"`
	ConnMaxIdleTime int    `mapstructure:"conn_max_idle_time"`
	DialTimeout     int    `mapstructure:"dial_timeout"`
	ReadTimeout     int    `mapstructure:"read_timeout"`
	WriteTimeout    int    `mapstructure:"write_timeout"`
	PoolTimeout     int    `mapstructure:"pool_timeout"`
	MaxRetries      int    `mapstructure:"max_retries"`
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
	viper.SetDefault("mysql.max_open_conns", 100)
	viper.SetDefault("mysql.max_idle_conns", 20)
	viper.SetDefault("mysql.conn_max_lifetime", 3600)
	viper.SetDefault("mysql.conn_max_idle_time", 300)

	viper.SetDefault("redis.pool_size", 20)
	viper.SetDefault("redis.min_idle_conns", 5)
	viper.SetDefault("redis.conn_max_lifetime", 3600)
	viper.SetDefault("redis.conn_max_idle_time", 300)
	viper.SetDefault("redis.dial_timeout", 5)
	viper.SetDefault("redis.read_timeout", 3)
	viper.SetDefault("redis.write_timeout", 3)
	viper.SetDefault("redis.pool_timeout", 4)
	viper.SetDefault("redis.max_retries", 3)

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
