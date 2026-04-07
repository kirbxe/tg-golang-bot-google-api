package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/alexl/tgBotGoogle/internal/calendar"
	"github.com/alexl/tgBotGoogle/internal/storage"
)


type Scheduler struct {
	storage    *storage.Storage
	calendar   *calendar.Calendar
	interval   time.Duration
	logger     *slog.Logger
	userID     int64
	telegramID int64
}

type Config struct {
	Storage    *storage.Storage
	Calendar   *calendar.Calendar
	Interval   time.Duration
	Logger     *slog.Logger
	UserID     int64
	TelegramID int64
}

func New(cfg Config) *Scheduler {
	return &Scheduler{
		storage:    cfg.Storage,
		calendar:   cfg.Calendar,
		interval:   cfg.Interval,
		logger:     cfg.Logger,
		userID:     cfg.UserID,
		telegramID: cfg.TelegramID,
	}
}

func (s *Scheduler) Start(ctx context.Context, notifyFunc func(int64, string)) {
	s.logger.Info("Запуск планировщика",
		"interval", s.interval.String(),
		"user_id", s.userID)

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	s.checkAndNotify(ctx, notifyFunc)

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("Планировщик остановлен", "user_id", s.userID)
			return

		case <-ticker.C:
			s.checkAndNotify(ctx, notifyFunc)
		}
	}
}

func (s *Scheduler) checkAndNotify(ctx context.Context, notifyFunc func(int64, string)) {
	s.logger.Debug("Проверка календаря", "user_id", s.userID)

	user, err := s.storage.GetUserByTelegramID(s.telegramID)
	if err != nil {
		s.logger.Error("Ошибка получения пользователя", "error", err)
		return
	}

	if user == nil {
		s.logger.Warn("Пользователь не найден", "telegram_id", s.telegramID)
		return
	}

	gEvents, err := s.calendar.GetUpcomingEvents(ctx, 50)
	if err != nil {
		s.logger.Error("Ошибка получения событий из календаря", "error", err)
		return
	}

	s.logger.Debug("Получено событий из календаря",
		"count", len(gEvents),
		"user_id", s.userID)

	for _, ge := range gEvents {
		event := &storage.Event{
			ID:          ge.ID,
			UserID:      s.userID,
			Title:       ge.Title,
			Description: ge.Description,
			StartTime:   ge.StartTime,
			EndTime:     ge.EndTime,
			Link:        ge.Link,
			Notified:    false,
		}

		if err := s.storage.SaveEvent(event); err != nil {
			s.logger.Error("Ошибка сохранения события",
				"event_id", ge.ID,
				"error", err)
		}
	}

	upcoming, err := s.storage.GetUpcomingEvents(s.userID, user.ReminderMinutes)
	if err != nil {
		s.logger.Error("Ошибка получения предстоящих событий", "error", err)
		return
	}

	s.logger.Debug("Найдено предстоящих событий для уведомления",
		"count", len(upcoming),
		"user_id", s.userID)

	for _, event := range upcoming {
		message := s.formatNotification(event)

		notifyFunc(s.telegramID, message)

		if err := s.storage.MarkEventAsNotified(event.ID); err != nil {
			s.logger.Error("Ошибка обновления статуса события",
				"event_id", event.ID,
				"error", err)
		}

		s.logger.Info("Отправлено уведомление",
			"event_id", event.ID,
			"title", event.Title)
	}
}

func (s *Scheduler) formatNotification(event *storage.Event) string {
	timeStr := event.StartTime.Format("02 января 2006, 15:04")

	loc := event.StartTime.Location()
	timeStr += fmt.Sprintf(" (%s)", loc.String())

	message := "*Напоминание!*\n\n"
	message += fmt.Sprintf("*Встреча:* %s\n", event.Title)
	message += fmt.Sprintf("*Время:* %s\n", timeStr)

	if event.Description != "" {
		desc := event.Description
		if len(desc) > 200 {
			desc = desc[:200] + "..."
		}
		message += fmt.Sprintf("*Описание:* %s\n", desc)
	}

	if event.Link != "" {
		message += fmt.Sprintf("*Ссылка:* %s\n", event.Link)
	}

	return message
}
