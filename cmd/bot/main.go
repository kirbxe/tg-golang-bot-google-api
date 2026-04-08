package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/alexl/tgBotGoogle/internal/bot"
	"github.com/alexl/tgBotGoogle/internal/calendar"
	"github.com/alexl/tgBotGoogle/internal/config"
	"github.com/alexl/tgBotGoogle/internal/oauth"
	"github.com/alexl/tgBotGoogle/internal/storage"
)

func main() {
	fmt.Println("🚀 Запуск Telegram бота для Google Calendar...")
	var wg sync.WaitGroup
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
		Port:    fmt.Sprintf("%d", cfg.Server.Port),
		Storage: db,
		CalendarCfg: calendar.Config{
			ClientID:     cfg.Google.ClientID,
			ClientSecret: cfg.Google.ClientSecret,
			RedirectURL:  cfg.Google.RedirectURL,
		},
		Logger: logger,
	})

	oauthCtx := context.Background()
	if err := oauthServer.Start(oauthCtx); err != nil {
		logger.Error("Ошибка запуска OAuth сервера", "error", err)
		os.Exit(1)
	}
	fmt.Printf("✅ OAuth2 сервер запущен на порту %d\n", cfg.Server.Port)

	tgBot, err := bot.New(bot.Config{
		Storage:  db,
		TgConfig: cfg,
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

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	fmt.Println("\n✅ Бот готов к работе!")
	fmt.Println("📍 Нажмите Ctrl+C для остановки")
	fmt.Println("📋 Команды: /start, /auth, /set_reminder, /next, /history, /stop")

	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := tgBot.Start(ctx); err != nil {
			logger.Error("Ошибка работы бота", "error", err)
		}
	}()

	<-sigChan
	fmt.Println("\n🛑 Получен сигнал остановки...")

	cancel()
	oauthServer.Wait()
	wg.Wait()
	logger.Info("Приложение остановлено")
	fmt.Println("✅ Бот остановлен корректно")
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
