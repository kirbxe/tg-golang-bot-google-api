package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/alexl/tgBotGoogle/internal/bot"
	"github.com/alexl/tgBotGoogle/internal/calendar"
	"github.com/alexl/tgBotGoogle/internal/config"
	"github.com/alexl/tgBotGoogle/internal/oauth"
	"github.com/alexl/tgBotGoogle/internal/scheduler"
	"github.com/alexl/tgBotGoogle/internal/storage"

	"golang.org/x/oauth2"
)

func main() {
	fmt.Println("🚀 Запуск Telegram бота для Google Calendar...")

	cfg, err := config.Load()
	if err != nil {
		fmt.Printf("❌ Ошибка загрузки конфигурации: %v\n", err)
		fmt.Println("\n💡 Совет: Скопируйте .env.example в .env и заполните значения")
		os.Exit(1)
	}
	fmt.Println("✅ Конфигурация загружена")

	if err := config.EnsureDirs(cfg); err != nil {
		fmt.Printf("❌ Ошибка создания директорий: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("✅ Директории созданы")

	logFile, err := os.OpenFile(cfg.Log.File, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		fmt.Printf("❌ Ошибка создания файла логов: %v\n", err)
		os.Exit(1)
	}
	defer logFile.Close()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: parseLogLevel(cfg.Log.Level),
	}))
	fmt.Println("✅ Логирование настроено")

	logger.Info("Запуск приложения", "version", "1.0.0")

	db, err := storage.New(cfg.Database.Path)
	if err != nil {
		logger.Error("Ошибка подключения к БД", "error", err)
		os.Exit(1)
	}
	defer db.Close()
	fmt.Println("✅ База данных подключена")

	oauthServer := oauth.New(oauth.Config{
		Port: fmt.Sprintf("%d", cfg.Server.Port),
		Storage: db,
		CalendarCfg: calendar.Config{
			ClientID:     cfg.Google.ClientID,
			ClientSecret: cfg.Google.ClientSecret,
			RedirectURL:  cfg.Google.RedirectURL,
		},
		Logger: logger,
	})

	ctx := context.Background()
	if err := oauthServer.Start(ctx); err != nil {
		logger.Error("Ошибка запуска OAuth сервера", "error", err)
		os.Exit(1)
	}
	fmt.Printf("✅ OAuth2 сервер запущен на порту %d\n", cfg.Server.Port)

	tgBot, err := bot.New(bot.Config{
		Storage:     db,
		TgConfig:    cfg,
		CalendarCfg: calendar.Config{
			ClientID:     cfg.Google.ClientID,
			ClientSecret: cfg.Google.ClientSecret,
			RedirectURL:  cfg.Google.RedirectURL,
		},
		OAuthServer: oauthServer,
		Logger:      logger,
	})
	if err != nil {
		logger.Error("Ошибка создания бота", "error", err)
		os.Exit(1)
	}
	fmt.Println("✅ Telegram бот создан")

	go startAllSchedulers(ctx, db, cfg, logger)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	fmt.Println("\n✅ Бот готов к работе!")
	fmt.Println("📍 Нажмите Ctrl+C для остановки")
	fmt.Println("📋 Команды: /start, /auth, /set_reminder, /next, /history, /stop")

	go func() {
		if err := tgBot.Start(ctx); err != nil {
			logger.Error("Ошибка работы бота", "error", err)
		}
	}()

	<-sigChan
	fmt.Println("\n🛑 Получен сигнал остановки...")

	cancel()

	logger.Info("Приложение остановлено")
	fmt.Println("✅ Бот остановлен корректно")
}

func startAllSchedulers(ctx context.Context, db *storage.Storage, cfg *config.Config, logger *slog.Logger) {
	logger.Info("Запуск планировщиков...")

	go func() {
		logger.Debug("Планировщики будут запускаться динамически при подключении пользователей")
	}()
}

func startSchedulerForUser(ctx context.Context, db *storage.Storage, cfg *config.Config, logger *slog.Logger, userID int64, telegramID int64, tokenData []byte) {
	token, err := oauth.DeserializeToken(tokenData)
	if err != nil {
		logger.Error("Ошибка десериализации токена", "error", err)
		return
	}

	oauthToken := &oauth2.Token{
		AccessToken:  token.AccessToken,
		RefreshToken: token.RefreshToken,
		Expiry:       token.Expiry,
	}

	cal, err := calendar.New(ctx, oauthToken, calendar.Config{
		ClientID:     cfg.Google.ClientID,
		ClientSecret: cfg.Google.ClientSecret,
		RedirectURL:  cfg.Google.RedirectURL,
	})
	if err != nil {
		logger.Error("Ошибка создания Calendar сервиса", "error", err)
		return
	}

	sched := scheduler.New(scheduler.Config{
		Storage:      db,
		Calendar:     cal,
		Interval:     cfg.Scheduler.Interval,
		Logger:       logger,
		UserID:       userID,
		TelegramID:   telegramID,
	})

	notifyFunc := func(telegramID int64, message string) {
		logger.Info("Уведомление", "telegram_id", telegramID, "message", message)
	}

	sched.Start(ctx, notifyFunc)
}

func parseLogLevel(level string) slog.Level {
	switch level {
	case "DEBUG":
		return slog.LevelDebug
	case "INFO":
		return slog.LevelInfo
	case "WARN", "WARNING":
		return slog.LevelWarn
	case "ERROR":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
