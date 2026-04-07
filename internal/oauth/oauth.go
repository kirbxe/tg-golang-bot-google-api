package oauth

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/alexl/tgBotGoogle/internal/calendar"
	"github.com/alexl/tgBotGoogle/internal/storage"
)

type OAuth2Server struct {
	port        string
	storage     *storage.Storage
	calendarCfg calendar.Config
	logger      *slog.Logger
	pendingAuth map[int64]chan AuthResult
	mu          sync.Mutex
}

type AuthResult struct {
	Token *calendar.Token
	Error error
}

type Config struct {
	Port        string
	Storage     *storage.Storage
	CalendarCfg calendar.Config
	Logger      *slog.Logger
}

func New(cfg Config) *OAuth2Server {
	return &OAuth2Server{
		port:        cfg.Port,
		storage:     cfg.Storage,
		calendarCfg: cfg.CalendarCfg,
		logger:      cfg.Logger,
		pendingAuth: make(map[int64]chan AuthResult),
	}
}

func (s *OAuth2Server) Start(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/callback", s.handleCallback)
	mux.HandleFunc("/health", s.handleHealth)

	server := &http.Server{
		Addr:         ":" + s.port,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		s.logger.Info("OAuth2 callback сервер запущен", "port", s.port)

		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.Error("Ошибка OAuth2 сервера", "error", err)
		}
	}()

	go func() {
		<-ctx.Done()
		s.logger.Info("Остановка OAuth2 сервера...")

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := server.Shutdown(shutdownCtx); err != nil {
			s.logger.Error("Ошибка при остановке OAuth2 сервера", "error", err)
		}
	}()

	return nil
}

func (s *OAuth2Server) RegisterAuth(telegramID int64) chan AuthResult {
	s.mu.Lock()
	defer s.mu.Unlock()

	ch := make(chan AuthResult, 1)
	s.pendingAuth[telegramID] = ch

	s.logger.Debug("Зарегистрирована ожидающая авторизация", "telegram_id", telegramID)
	return ch
}

func (s *OAuth2Server) handleCallback(w http.ResponseWriter, r *http.Request) {
	s.logger.Info("Получен OAuth2 callback", "remote_addr", r.RemoteAddr)

	code := r.URL.Query().Get("code")
	if code == "" {
		s.logger.Warn("OAuth2 callback без кода")
		http.Error(w, "Код авторизации не найден", http.StatusBadRequest)
		return
	}

	state := r.URL.Query().Get("state")
	s.logger.Debug("OAuth2 state", "state", state)

	ctx := context.Background()
	token, err := calendar.ExchangeToken(ctx, s.calendarCfg, code)
	if err != nil {
		s.logger.Error("Ошибка обмена кода на токен", "error", err)
		http.Error(w, "Ошибка авторизации: "+err.Error(), http.StatusInternalServerError)
		return
	}

	s.logger.Info("Токен успешно получен", "has_refresh", token.RefreshToken != "")

	authToken := &calendar.Token{
		AccessToken:  token.AccessToken,
		RefreshToken: token.RefreshToken,
		Expiry:       token.Expiry,
	}

	s.mu.Lock()
	for telegramID, ch := range s.pendingAuth {
		select {
		case ch <- AuthResult{Token: authToken, Error: nil}:
			s.logger.Info("Токен отправлен боту", "telegram_id", telegramID)
			delete(s.pendingAuth, telegramID)
		default:
		}
	}
	s.mu.Unlock()

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, `
		<!DOCTYPE html>
		<html>
		<head><title>Авторизация успешна</title></head>
		<body style="font-family: Arial, sans-serif; text-align: center; padding: 50px;">
			<h1>✅ Авторизация успешна!</h1>
			<p>Google Calendar подключён.</p>
			<p>Вернитесь в Telegram бота.</p>
		</body>
		</html>
	`)
}

func (s *OAuth2Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "ok",
		"time":   time.Now().Format(time.RFC3339),
	})
}

func SerializeToken(token *calendar.Token) ([]byte, error) {
	data, err := json.Marshal(token)
	if err != nil {
		return nil, fmt.Errorf("ошибка сериализации токена: %w", err)
	}
	return data, nil
}

func DeserializeToken(data []byte) (*calendar.Token, error) {
	var token calendar.Token
	if err := json.Unmarshal(data, &token); err != nil {
		return nil, fmt.Errorf("ошибка десериализации токена: %w", err)
	}
	return &token, nil
}
