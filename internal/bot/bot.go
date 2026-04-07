package bot

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/alexl/tgBotGoogle/internal/calendar"
	"github.com/alexl/tgBotGoogle/internal/config"
	"github.com/alexl/tgBotGoogle/internal/oauth"
	"github.com/alexl/tgBotGoogle/internal/scheduler"
	"github.com/alexl/tgBotGoogle/internal/storage"

	"golang.org/x/oauth2"
	"gopkg.in/telebot.v3"
)

type Bot struct {
	tb          *telebot.Bot
	storage     *storage.Storage
	config      *config.Config
	oauthServer *oauth.OAuth2Server
	calendarCfg calendar.Config
	logger      *slog.Logger
	schedulers  map[int64]*scheduler.Scheduler
	schedMu     sync.Mutex
}

type Config struct {
	Storage     *storage.Storage
	TgConfig    *config.Config
	CalendarCfg calendar.Config
	OAuthServer *oauth.OAuth2Server
	Logger      *slog.Logger
}

func New(cfg Config) (*Bot, error) {
	settings := telebot.Settings{
		Token:  cfg.TgConfig.Telegram.BotToken,
		Poller: &telebot.LongPoller{Timeout: 10},
	}

	tb, err := telebot.NewBot(settings)
	if err != nil {
		return nil, fmt.Errorf("ошибка создания бота: %w", err)
	}

	bot := &Bot{
		tb:          tb,
		storage:     cfg.Storage,
		config:      cfg.TgConfig,
		oauthServer: cfg.OAuthServer,
		calendarCfg: cfg.CalendarCfg,
		logger:      cfg.Logger,
		schedulers:  make(map[int64]*scheduler.Scheduler),
	}

	bot.registerHandlers()

	return bot, nil
}

func (b *Bot) registerHandlers() {
	b.tb.Handle("/start", b.handleStart)
	b.tb.Handle("/help", b.handleHelp)
	b.tb.Handle("/auth", b.handleAuth)
	b.tb.Handle("/set_reminder", b.handleSetReminder)
	b.tb.Handle("/next", b.handleNext)
	b.tb.Handle("/history", b.handleHistory)
	b.tb.Handle("/stop", b.handleStop)
	b.tb.Handle("/reauth", b.handleReauth)
	b.tb.Handle("/check_calendar", b.handleCheckCalendar)
	b.tb.Handle("callback", b.handleCallback)
}

func (b *Bot) handleStart(c telebot.Context) error {
	b.logger.Info("Команда /start", "user_id", c.Sender().ID)

	user, err := b.storage.CreateUser(c.Sender().ID, c.Sender().Username)
	if err != nil {
		b.logger.Error("Ошибка создания пользователя", "error", err)
		return c.Reply("Ошибка при регистрации. Попробуйте позже.")
	}

	message := "Привет! Я бот для уведомлений о встречах из Google Calendar\n\n"
	message += "Я умею:\n"
	message += "- Отслеживать ваши встречи\n"
	message += "- Присылать напоминания за заданное время\n"
	message += "- Показывать историю встреч\n\n"
	message += "Команды:\n"
	message += "/auth - Подключить Google Calendar\n"
	message += "/set_reminder - Установить время напоминания\n"
	message += "/next - Следующая встреча\n"
	message += "/history - Последние 10 встреч\n"
	message += "/help - Справка\n"
	message += "/stop - Отключить уведомления\n\n"
	message += "/check_calendar - Проверить свои ближайшие встречи\n\n"

	if user.GoogleToken != nil && len(user.GoogleToken) > 0 {
		message += "Google Calendar подключён"
	} else {
		message += "Google Calendar не подключён. Используйте /auth"
	}

	return c.Reply(message)
}

func (b *Bot) handleHelp(c telebot.Context) error {
	b.logger.Info("Команда /help", "user_id", c.Sender().ID)

	message := "Справка по командам\n\n"
	message += "/start - Начать работу с ботом\n"
	message += "/auth - Авторизоваться в Google Calendar\n"
	message += "/set_reminder - Установить напоминание (например: /set_reminder 15)\n"
	message += "/next - Показать следующую встречу\n"
	message += "/history - Показать последние 10 встреч\n"
	message += "/stop - Отключить уведомления\n\n"
	message += "Примеры:\n"
	message += "• /set_reminder 5 - напоминать за 5 минут\n"
	message += "• /set_reminder 30 - напоминать за 30 минут\n"
	message += "• /check_calendar - Увидеть ближайшие встречи\n"

	return c.Reply(message)
}

func (b *Bot) handleAuth(c telebot.Context) error {
	b.logger.Info("Команда /auth", "user_id", c.Sender().ID)

	user, err := b.storage.GetUserByTelegramID(c.Sender().ID)
	if err != nil {
		b.logger.Error("Ошибка поиска пользователя", "error", err)
		return c.Reply("Ошибка. Попробуйте позже.")
	}

	if user == nil {
		return c.Reply("Сначала нажмите /start для регистрации")
	}

	if user.GoogleToken != nil && len(user.GoogleToken) > 0 {
		return c.Reply("Google Calendar уже подключён.\nДля переподключения: /reauth")
	}

	state := fmt.Sprintf("state_%d_%d", c.Sender().ID, time.Now().Unix())

	authURL := calendar.GetAuthURL(calendar.Config{
		ClientID:     b.calendarCfg.ClientID,
		ClientSecret: b.calendarCfg.ClientSecret,
		RedirectURL:  b.calendarCfg.RedirectURL,
	}, state)

	authChan := b.oauthServer.RegisterAuth(c.Sender().ID)

	message := "Авторизация в Google Calendar\n\n"
	message += "Перейдите по ссылке для авторизации:\n"
	message += authURL + "\n\n"
	message += "После авторизации вы увидите страницу успеха.\n"
	message += "Бот автоматически получит токен."

	if err := c.Reply(message); err != nil {
		b.logger.Error("Ошибка отправки сообщения", "error", err)
		return err
	}

	b.tb.Notify(c.Sender(), telebot.Typing)

	go b.waitForAuthResult(c.Sender().ID, authChan, user.ID)

	return nil
}

func (b *Bot) waitForAuthResult(telegramID int64, authChan chan oauth.AuthResult, userID int64) {
	recipient := &telebot.User{ID: telegramID}

	select {
	case result := <-authChan:
		if result.Error != nil {
			b.logger.Error("Ошибка авторизации", "error", result.Error)
			b.tb.Notify(recipient, telebot.Typing)
			b.tb.Send(recipient, "Ошибка авторизации: "+result.Error.Error())
			return
		}

		tokenData, err := oauth.SerializeToken(result.Token)
		if err != nil {
			b.logger.Error("Ошибка сериализации токена", "error", err)
			b.tb.Send(recipient, "Ошибка обработки токена.")
			return
		}

		if err := b.storage.UpdateGoogleToken(userID, tokenData); err != nil {
			b.logger.Error("Ошибка сохранения токена", "error", err)
			b.tb.Send(recipient, "Ошибка сохранения токена.")
			return
		}

		b.logger.Info("Google Calendar подключён", "telegram_id", telegramID)
		b.tb.Notify(recipient, telebot.Typing)
		b.tb.Send(recipient, "Google Calendar успешно подключён!\nТеперь бот будет отслеживать ваши встречи.")

		b.StartSchedulerForUser(userID, telegramID, tokenData)

	case <-time.After(5 * time.Minute):
		b.logger.Warn("Таймаут авторизации", "telegram_id", telegramID)
		b.tb.Send(recipient, "Время авторизации истекло.\nПопробуйте снова: /auth")
	}
}

func (b *Bot) handleSetReminder(c telebot.Context) error {
	b.logger.Info("Команда /set_reminder", "user_id", c.Sender().ID)

	user, err := b.storage.GetUserByTelegramID(c.Sender().ID)
	if err != nil {
		b.logger.Error("Ошибка поиска пользователя", "error", err)
		return c.Reply("Ошибка. Попробуйте позже.")
	}

	if user == nil {
		return c.Reply("Сначала нажмите /start для регистрации")
	}

	args := strings.Split(c.Message().Payload, " ")
	if len(args) < 1 {
		return c.Reply("Укажите время в минутах.\nПример: /set_reminder 15")
	}

	minutes, err := strconv.Atoi(args[0])
	if err != nil {
		return c.Reply("Время должно быть числом.\nПример: /set_reminder 15")
	}

	if minutes < 1 || minutes > 1440 {
		return c.Reply("Время должно быть от 1 до 1440 минут (24 часа)")
	}

	if err := b.storage.SetReminderMinutes(user.ID, minutes); err != nil {
		b.logger.Error("Ошибка обновления напоминания", "error", err)
		return c.Reply("Ошибка при сохранении. Попробуйте позже.")
	}

	return c.Reply(fmt.Sprintf("Напоминание установлено за %d минут до встречи", minutes))
}

func (b *Bot) handleNext(c telebot.Context) error {
	b.logger.Info("Команда /next", "user_id", c.Sender().ID)

	user, err := b.storage.GetUserByTelegramID(c.Sender().ID)
	if err != nil {
		b.logger.Error("Ошибка поиска пользователя", "error", err)
		return c.Reply("Ошибка. Попробуйте позже.")
	}

	if user == nil {
		return c.Reply("Сначала нажмите /start для регистрации")
	}

	allEvents, err := b.storage.GetAllEvents(user.ID)
	if err != nil {
		b.logger.Error("Ошибка поиска событий", "error", err, "user_id", user.ID)
		return c.Reply(fmt.Sprintf("Ошибка при поиске встреч: %v", err))
	}

	if len(allEvents) == 0 {
		return c.Reply("В базе данных нет встреч. Отправьте /check_calendar для обновления.")
	}

	now := time.Now()
	var nextEvent *storage.Event
	for _, e := range allEvents {
		if e.StartTime.After(now) {
			nextEvent = e
			break
		}
	}

	if nextEvent == nil {
		lastEvent := allEvents[0]
		timeStr := lastEvent.StartTime.Format("02.01.2006 15:04")
		message := fmt.Sprintf("Все встречи уже прошли.\n\nПоследняя: %s\nВремя: %s", lastEvent.Title, timeStr)
		return c.Reply(message)
	}

	timeStr := nextEvent.StartTime.Format("02.01.2006 15:04")
	message := fmt.Sprintf("Следующая встреча:\n\n%s\nВремя: %s\n", nextEvent.Title, timeStr)

	if nextEvent.Description != "" {
		message += fmt.Sprintf("Описание: %s\n", nextEvent.Description)
	}

	if nextEvent.Link != "" {
		message += fmt.Sprintf("Ссылка: %s\n", nextEvent.Link)
	}

	return c.Reply(message)
}

func (b *Bot) handleHistory(c telebot.Context) error {
	b.logger.Info("Команда /history", "user_id", c.Sender().ID)

	user, err := b.storage.GetUserByTelegramID(c.Sender().ID)
	if err != nil {
		b.logger.Error("Ошибка поиска пользователя", "error", err)
		return c.Reply("Ошибка. Попробуйте позже.")
	}

	if user == nil {
		return c.Reply("Сначала нажмите /start для регистрации")
	}

	events, err := b.storage.GetPastEvents(user.ID, 10)
	if err != nil {
		b.logger.Error("Ошибка получения истории", "error", err)
		return c.Reply(" Ошибка при получении истории.")
	}

	if len(events) == 0 {
		return c.Reply("История встреч пуста")
	}

	message := "Последние 10 встреч:\n\n"
	for i, event := range events {
		timeStr := event.StartTime.Format("02.01.2006 15:04")
		message += fmt.Sprintf("%d. [%s] %s\n", i+1, timeStr, event.Title)
	}

	return c.Reply(message)
}

func (b *Bot) handleStop(c telebot.Context) error {
	b.logger.Info("Команда /stop", "user_id", c.Sender().ID)

	user, err := b.storage.GetUserByTelegramID(c.Sender().ID)
	if err != nil {
		b.logger.Error("Ошибка поиска пользователя", "error", err)
		return c.Reply(" Ошибка. Попробуйте позже.")
	}

	if user == nil {
		return c.Reply(" Сначала нажмите /start для регистрации")
	}

	if err := b.storage.SetReminderMinutes(user.ID, 0); err != nil {
		b.logger.Error("Ошибка отключения уведомлений", "error", err)
		return c.Reply("Ошибка при отключении уведомлений.")
	}

	return c.Reply("Уведомления отключены.\nВключить: /set_reminder 15")
}

func (b *Bot) handleReauth(c telebot.Context) error {
	b.logger.Info("Команда /reauth", "user_id", c.Sender().ID)

	user, err := b.storage.GetUserByTelegramID(c.Sender().ID)
	if err != nil {
		b.logger.Error("Ошибка поиска пользователя", "error", err)
		return c.Reply("Ошибка. Попробуйте позже.")
	}

	if user == nil {
		return c.Reply("Сначала нажмите /start для регистрации")
	}

	if err := b.storage.UpdateGoogleToken(user.ID, nil); err != nil {
		b.logger.Error("Ошибка сброса токена", "error", err)
		return c.Reply("Ошибка при переподключении.")
	}

	b.logger.Info("Токен сброшен", "telegram_id", c.Sender().ID)

	return b.handleAuth(c)
}

func (b *Bot) handleCheckCalendar(c telebot.Context) error {
	b.logger.Info("Команда /check_calendar", "user_id", c.Sender().ID)

	user, err := b.storage.GetUserByTelegramID(c.Sender().ID)
	if err != nil {
		b.logger.Error("Ошибка поиска пользователя", "error", err)
		return c.Reply("Ошибка. Попробуйте позже.")
	}

	if user == nil {
		return c.Reply("Сначала нажмите /start для регистрации")
	}

	if user.GoogleToken == nil || len(user.GoogleToken) == 0 {
		return c.Reply("Google Calendar не подключён. Используйте /auth")
	}

	token, err := oauth.DeserializeToken(user.GoogleToken)
	if err != nil {
		b.logger.Error("Ошибка десериализации токена", "error", err)
		return c.Reply("Ошибка токена. Попробуйте /reauth")
	}

	oauthToken := &oauth2.Token{
		AccessToken:  token.AccessToken,
		RefreshToken: token.RefreshToken,
		Expiry:       token.Expiry,
	}

	ctx := context.Background()
	cal, err := calendar.New(ctx, oauthToken, calendar.Config{
		ClientID:     b.calendarCfg.ClientID,
		ClientSecret: b.calendarCfg.ClientSecret,
		RedirectURL:  b.calendarCfg.RedirectURL,
	})
	if err != nil {
		b.logger.Error("Ошибка создания Calendar сервиса", "error", err)
		return c.Reply("Ошибка подключения к календарю.")
	}

	events, err := cal.GetUpcomingEvents(ctx, 50)
	if err != nil {
		b.logger.Error("Ошибка получения событий", "error", err)
		return c.Reply("Ошибка чтения календаря: " + err.Error())
	}

	if len(events) == 0 {
		return c.Reply("В календаре нет предстоящих событий")
	}

	saved := 0
	for _, ge := range events {
		event := &storage.Event{
			ID:          ge.ID,
			UserID:      user.ID,
			Title:       ge.Title,
			Description: ge.Description,
			StartTime:   ge.StartTime,
			EndTime:     ge.EndTime,
			Link:        ge.Link,
			Notified:    false,
		}

		if err := b.storage.SaveEvent(event); err != nil {
			b.logger.Error("Ошибка сохранения события", "error", err)
			continue
		}
		saved++
	}

	message := fmt.Sprintf("Найдено событий: %d\nСохранено: %d\n\n", len(events), saved)
	message += "Ближайшие встречи:\n\n"

	for i, ge := range events {
		if i >= 5 {
			break
		}
		timeStr := ge.StartTime.Format("02.01.2006 15:04")
		message += fmt.Sprintf("%d. %s - %s\n", i+1, timeStr, ge.Title)
	}

	return c.Reply(message)
}

func (b *Bot) handleCallback(c telebot.Context) error {
	b.logger.Debug("Callback получен", "data", c.Data())
	return nil
}

func (b *Bot) Start(ctx context.Context) error {
	b.logger.Info("Запуск Telegram бота")

	go b.tb.Start()

	<-ctx.Done()

	b.tb.Stop()
	b.logger.Info("Telegram бот остановлен")

	return nil
}

func (b *Bot) StartSchedulerForUser(userID int64, telegramID int64, tokenData []byte) {
	b.schedMu.Lock()
	defer b.schedMu.Unlock()

	if _, exists := b.schedulers[userID]; exists {
		b.logger.Info("Планировщик уже запущен", "user_id", userID)
		return
	}

	token, err := oauth.DeserializeToken(tokenData)
	if err != nil {
		b.logger.Error("Ошибка десериализации токена", "error", err)
		return
	}

	oauthToken := &oauth2.Token{
		AccessToken:  token.AccessToken,
		RefreshToken: token.RefreshToken,
		Expiry:       token.Expiry,
	}

	ctx := context.Background()
	cal, err := calendar.New(ctx, oauthToken, calendar.Config{
		ClientID:     b.calendarCfg.ClientID,
		ClientSecret: b.calendarCfg.ClientSecret,
		RedirectURL:  b.calendarCfg.RedirectURL,
	})
	if err != nil {
		b.logger.Error("Ошибка создания Calendar сервиса", "error", err)
		return
	}

	sched := scheduler.New(scheduler.Config{
		Storage:    b.storage,
		Calendar:   cal,
		Interval:   b.config.Scheduler.Interval,
		Logger:     b.logger,
		UserID:     userID,
		TelegramID: telegramID,
	})

	b.schedulers[userID] = sched

	go sched.Start(ctx, func(telegramID int64, message string) {
		recipient := &telebot.User{ID: telegramID}
		if _, err := b.tb.Send(recipient, message); err != nil {
			b.logger.Error("Ошибка отправки уведомления", "error", err)
		}
	})

	b.logger.Info("Планировщик запущен", "user_id", userID, "interval", b.config.Scheduler.Interval)
}

func generateState(userID int64) string {
	return fmt.Sprintf("state_%d", userID)
}
