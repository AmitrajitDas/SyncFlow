package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// Config holds all configuration for the sync engine
type Config struct {
	Environment string           `mapstructure:"environment"`
	Server      ServerConfig     `mapstructure:"server"`
	MongoDB     MongoDBConfig    `mapstructure:"mongodb"`
	Kafka       KafkaConfig      `mapstructure:"kafka"`
	Redis       RedisConfig      `mapstructure:"redis"`
	Checkpoint  CheckpointConfig `mapstructure:"checkpoint"`
	WebSocket   WebSocketConfig  `mapstructure:"websocket"`
	Log         LogConfig        `mapstructure:"log"`
	viper       *viper.Viper
}

// ServerConfig holds server configuration
type ServerConfig struct {
	HTTPPort string `mapstructure:"http_port"`
	WSPort   string `mapstructure:"ws_port"`
}

// MongoDBConfig holds MongoDB configuration
type MongoDBConfig struct {
	URI      string `mapstructure:"uri"`
	Database string `mapstructure:"database"`
}

// KafkaConfig holds Kafka configuration
type KafkaConfig struct {
	BootstrapServers []string `mapstructure:"bootstrap_servers"`
	ConsumerGroup    string   `mapstructure:"consumer_group"`
}

// RedisConfig holds Redis configuration
type RedisConfig struct {
	Host     string `mapstructure:"host"`
	Port     string `mapstructure:"port"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
}

// CheckpointConfig holds checkpoint configuration
type CheckpointConfig struct {
	Dir      string        `mapstructure:"dir"`
	Interval time.Duration `mapstructure:"interval"`
}

// WebSocketConfig holds WebSocket configuration
type WebSocketConfig struct {
	ReadBufferSize  int           `mapstructure:"read_buffer_size"`
	WriteBufferSize int           `mapstructure:"write_buffer_size"`
	PingInterval    time.Duration `mapstructure:"ping_interval"`
	PongWait        time.Duration `mapstructure:"pong_wait"`
	WriteWait       time.Duration `mapstructure:"write_wait"`
}

// LogConfig holds logging configuration
type LogConfig struct {
	Level string `mapstructure:"level"`
}

// Load reads configuration from environment variables
func Load() (*Config, error) {
	v := viper.New()

	// Set defaults
	setDefaults(v)

	// Enable environment variable override
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// Bind specific env vars for docker-compose compatibility
	bindEnvVars(v)

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	cfg.viper = v
	return &cfg, nil
}

// LoadConfig is an alias for Load for backwards compatibility
func LoadConfig() (*Config, error) {
	return Load()
}

// GetString returns a config value as string (for dynamic lookups)
func (c *Config) GetString(key string) string {
	if c.viper == nil {
		return ""
	}
	return c.viper.GetString(key)
}

// setDefaults sets default configuration values
func setDefaults(v *viper.Viper) {
	// Environment
	v.SetDefault("environment", "development")

	// Server defaults
	v.SetDefault("server.http_port", "8082")
	v.SetDefault("server.ws_port", "8083")
	v.SetDefault("server.http_addr", ":8082")

	// MongoDB defaults
	v.SetDefault("mongodb.uri", "mongodb://admin:admin123@localhost:27017/syncflow?authSource=admin&replicaSet=rs0")
	v.SetDefault("mongodb.database", "syncflow")

	// Kafka defaults
	v.SetDefault("kafka.bootstrap_servers", []string{"localhost:9092"})
	v.SetDefault("kafka.consumer_group", "sync-engine-group")

	// Redis defaults
	v.SetDefault("redis.host", "localhost")
	v.SetDefault("redis.port", "6379")
	v.SetDefault("redis.password", "redis123")
	v.SetDefault("redis.db", 0)

	// Checkpoint defaults
	v.SetDefault("checkpoint.dir", "/tmp/syncflow/checkpoints")
	v.SetDefault("checkpoint.interval", "5m")

	// WebSocket defaults
	v.SetDefault("websocket.read_buffer_size", 1024)
	v.SetDefault("websocket.write_buffer_size", 1024)
	v.SetDefault("websocket.ping_interval", "30s")
	v.SetDefault("websocket.pong_wait", "60s")
	v.SetDefault("websocket.write_wait", "10s")

	// Log defaults
	v.SetDefault("log.level", "INFO")
}

// bindEnvVars binds environment variables for docker-compose compatibility
func bindEnvVars(v *viper.Viper) {
	// Direct env var mappings (without SYNC_ prefix)
	v.BindEnv("environment", "ENVIRONMENT")
	v.BindEnv("server.http_port", "HTTP_PORT")
	v.BindEnv("server.ws_port", "WS_PORT")
	v.BindEnv("server.http_addr", "HTTP_ADDR")
	v.BindEnv("mongodb.uri", "MONGODB_URI")
	v.BindEnv("mongodb.database", "MONGODB_DATABASE")
	v.BindEnv("kafka.bootstrap_servers", "KAFKA_BOOTSTRAP_SERVERS")
	v.BindEnv("kafka.consumer_group", "KAFKA_CONSUMER_GROUP")
	v.BindEnv("redis.host", "REDIS_HOST")
	v.BindEnv("redis.port", "REDIS_PORT")
	v.BindEnv("redis.password", "REDIS_PASSWORD")
	v.BindEnv("redis.db", "REDIS_DB")
	v.BindEnv("checkpoint.dir", "CHECKPOINT_DIR")
	v.BindEnv("checkpoint.interval", "CHECKPOINT_INTERVAL")
	v.BindEnv("log.level", "LOG_LEVEL")
}

// RedisAddr returns the full Redis address
func (c *Config) RedisAddr() string {
	return c.Redis.Host + ":" + c.Redis.Port
}

// GetKafkaServers returns Kafka bootstrap servers as a string slice
func (c *Config) GetKafkaServers() []string {
	return c.Kafka.BootstrapServers
}
