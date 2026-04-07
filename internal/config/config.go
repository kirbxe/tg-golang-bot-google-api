package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	Telegram struct {
		BotToken string
	}
	Google struct {
		ClientID     string
		ClientSecret string
		RedirectURL  string
	}
	Encryption struct {
		Key string
	}
	Database struct {
		Path string
	}
	Server struct {
		Port int
	}
	Scheduler struct {
		Interval time.Duration
	}
	Log struct {
		File  string
		Level string
	}
}

func Load() (*Config, error) {
	viper.SetConfigFile(".env")
	viper.SetConfigType("env")

	if err := viper.ReadInConfig(); err != nil {
		fmt.Println("Файл .env не найден, используем переменные окружения")
	}

	cfg := &Config{}

	cfg.Telegram.BotToken = viper.GetString("TELEGRAM_BOT_TOKEN")
	if cfg.Telegram.BotToken == "" {
		return nil, fmt.Errorf("TELEGRAM_BOT_TOKEN не установлен")
	}

	cfg.Google.ClientID = viper.GetString("GOOGLE_CLIENT_ID")
	cfg.Google.ClientSecret = viper.GetString("GOOGLE_CLIENT_SECRET")

	cfg.Server.Port = viper.GetInt("OAUTH_CALLBACK_PORT")
	if cfg.Server.Port == 0 {
		cfg.Server.Port = 8080
	}

	cfg.Google.RedirectURL = fmt.Sprintf("http://localhost:%d/callback", cfg.Server.Port)

	cfg.Encryption.Key = viper.GetString("ENCRYPTION_KEY")
	if len(cfg.Encryption.Key) != 32 {
		return nil, fmt.Errorf("ENCRYPTION_KEY должен быть 32 символа (AES-256)")
	}

	cfg.Database.Path = viper.GetString("DB_PATH")
	if cfg.Database.Path == "" {
		cfg.Database.Path = "./data/bot.db"
	}

	interval := viper.GetInt("CHECK_INTERVAL_MINUTES")
	if interval == 0 {
		interval = 5
	}
	cfg.Scheduler.Interval = time.Duration(interval) * time.Minute

	cfg.Log.File = viper.GetString("LOG_FILE")
	if cfg.Log.File == "" {
		cfg.Log.File = "./logs/bot.log"
	}
	cfg.Log.Level = viper.GetString("LOG_LEVEL")
	if cfg.Log.Level == "" {
		cfg.Log.Level = "INFO"
	}

	return cfg, nil
}

func EnsureDirs(cfg *Config) error {
	dbDir := filepath.Dir(cfg.Database.Path)
	if err := os.MkdirAll(dbDir, 0755); err != nil {
		return fmt.Errorf("ошибка создания директории БД: %w", err)
	}

	logDir := filepath.Dir(cfg.Log.File)
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return fmt.Errorf("ошибка создания директории логов: %w", err)
	}

	return nil
}
