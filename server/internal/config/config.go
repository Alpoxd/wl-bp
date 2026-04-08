package config

import (
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	VKBotToken          string
	VKBotGroupID        int
	AdminVKUserIDs      []int64
	MaxConcurrentWorkers int
	WorkerResourceMode  string
	LoadBalanceStrategy string
	AccountCooldownMin  int
	AccountMaxFailCount int
	DBPath              string
	GRPCPort            int
	MetricsPort         int
	ChromedpHeadless    bool
	ChromePath          string
	LogLevel            string
}

func LoadConfig() *Config {
	_ = godotenv.Load() // Ignore error if .env not found

	cfg := &Config{
		VKBotToken:          getEnv("VK_BOT_TOKEN", ""),
		VKBotGroupID:        getEnvInt("VK_BOT_GROUP_ID", 0),
		AdminVKUserIDs:      getEnvInt64Slice("ADMIN_VK_USER_IDS"),
		MaxConcurrentWorkers: getEnvInt("MAX_CONCURRENT_WORKERS", 10),
		WorkerResourceMode:  getEnv("WORKER_RESOURCE_MODE", "moderate"),
		LoadBalanceStrategy: getEnv("LOAD_BALANCE_STRATEGY", "lru"),
		AccountCooldownMin:  getEnvInt("ACCOUNT_COOLDOWN_MINUTES", 30),
		AccountMaxFailCount: getEnvInt("ACCOUNT_MAX_FAIL_COUNT", 3),
		DBPath:              getEnv("DB_PATH", "./data/wlbp.db"),
		GRPCPort:            getEnvInt("GRPC_PORT", 8080),
		MetricsPort:         getEnvInt("METRICS_PORT", 9090),
		ChromedpHeadless:    getEnvBool("CHROMEDP_HEADLESS", true),
		ChromePath:          getEnv("CHROME_PATH", ""),
		LogLevel:            getEnv("LOG_LEVEL", "info"),
	}

	if cfg.VKBotToken == "" {
		log.Fatal("VK_BOT_TOKEN is required")
	}
	if cfg.VKBotGroupID == 0 {
		log.Fatal("VK_BOT_GROUP_ID is required")
	}

	return cfg
}

func getEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	str := getEnv(key, "")
	if val, err := strconv.Atoi(str); err == nil {
		return val
	}
	return fallback
}

func getEnvBool(key string, fallback bool) bool {
	str := getEnv(key, "")
	if strings.ToLower(str) == "true" || str == "1" {
		return true
	}
	if strings.ToLower(str) == "false" || str == "0" {
		return false
	}
	return fallback
}

func getEnvInt64Slice(key string) []int64 {
	str := getEnv(key, "")
	if str == "" {
		return nil
	}
	parts := strings.Split(str, ",")
	var result []int64
	for _, p := range parts {
		if val, err := strconv.ParseInt(strings.TrimSpace(p), 10, 64); err == nil {
			result = append(result, val)
		}
	}
	return result
}
