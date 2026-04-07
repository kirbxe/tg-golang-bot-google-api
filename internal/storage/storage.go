package storage

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

type Storage struct {
	db *sql.DB
}

type User struct {
	ID              int64
	TelegramID      int64
	Username        string
	GoogleToken     []byte
	ReminderMinutes int
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type Event struct {
	ID          string
	UserID      int64
	Title       string
	Description string
	StartTime   time.Time
	EndTime     time.Time
	Link        string
	Notified    bool
	CreatedAt   time.Time
}

func New(dbPath string) (*Storage, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("ошибка подключения к БД: %w", err)
	}

	storage := &Storage{db: db}

	if err := storage.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ошибка миграции: %w", err)
	}

	return storage, nil
}

func (s *Storage) migrate() error {
	createUsersTable := `
	CREATE TABLE IF NOT EXISTS users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		telegram_id INTEGER UNIQUE NOT NULL,
		username TEXT,
		google_token BLOB,
		reminder_minutes INTEGER DEFAULT 15,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	`

	createEventsTable := `
	CREATE TABLE IF NOT EXISTS events (
		id TEXT PRIMARY KEY,
		user_id INTEGER NOT NULL,
		title TEXT NOT NULL,
		description TEXT,
		start_time DATETIME NOT NULL,
		end_time DATETIME NOT NULL,
		link TEXT,
		notified BOOLEAN DEFAULT FALSE,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
	);
	`

	createIndex := `
	CREATE INDEX IF NOT EXISTS idx_events_user_id ON events(user_id);
	`

	createTimeIndex := `
	CREATE INDEX IF NOT EXISTS idx_events_start_time ON events(start_time);
	`

	queries := []string{
		createUsersTable,
		createEventsTable,
		createIndex,
		createTimeIndex,
	}

	for _, query := range queries {
		if _, err := s.db.Exec(query); err != nil {
			return fmt.Errorf("ошибка выполнения SQL: %w", err)
		}
	}

	return nil
}

func (s *Storage) CreateUser(telegramID int64, username string) (*User, error) {
	query := `
	INSERT INTO users (telegram_id, username, reminder_minutes)
	VALUES (?, ?, 15)
	ON CONFLICT(telegram_id) DO UPDATE SET
		username = excluded.username,
		updated_at = CURRENT_TIMESTAMP
	RETURNING id, telegram_id, username, google_token, reminder_minutes, created_at, updated_at
	`

	user := &User{}
	err := s.db.QueryRow(query, telegramID, username).Scan(
		&user.ID,
		&user.TelegramID,
		&user.Username,
		&user.GoogleToken,
		&user.ReminderMinutes,
		&user.CreatedAt,
		&user.UpdatedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("ошибка создания пользователя: %w", err)
	}

	return user, nil
}

func (s *Storage) GetUserByTelegramID(telegramID int64) (*User, error) {
	query := `
	SELECT id, telegram_id, username, google_token, reminder_minutes, created_at, updated_at
	FROM users
	WHERE telegram_id = ?
	`

	user := &User{}
	err := s.db.QueryRow(query, telegramID).Scan(
		&user.ID,
		&user.TelegramID,
		&user.Username,
		&user.GoogleToken,
		&user.ReminderMinutes,
		&user.CreatedAt,
		&user.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}

	if err != nil {
		return nil, fmt.Errorf("ошибка поиска пользователя: %w", err)
	}

	return user, nil
}

func (s *Storage) UpdateGoogleToken(userID int64, tokenData []byte) error {
	query := `
	UPDATE users
	SET google_token = ?, updated_at = CURRENT_TIMESTAMP
	WHERE id = ?
	`

	result, err := s.db.Exec(query, tokenData, userID)
	if err != nil {
		return fmt.Errorf("ошибка обновления токена: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("ошибка получения количества строк: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("пользователь не найден")
	}

	return nil
}

func (s *Storage) GetGoogleToken(userID int64) (map[string]interface{}, error) {
	query := `SELECT google_token FROM users WHERE id = ?`

	var tokenData []byte
	err := s.db.QueryRow(query, userID).Scan(&tokenData)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("ошибка получения токена: %w", err)
	}

	if tokenData == nil || len(tokenData) == 0 {
		return nil, nil
	}

	var token map[string]interface{}
	if err := json.Unmarshal(tokenData, &token); err != nil {
		return nil, fmt.Errorf("ошибка десериализации токена: %w", err)
	}

	return token, nil
}

func (s *Storage) SetReminderMinutes(userID int64, minutes int) error {
	query := `
	UPDATE users
	SET reminder_minutes = ?, updated_at = CURRENT_TIMESTAMP
	WHERE id = ?
	`

	result, err := s.db.Exec(query, minutes, userID)
	if err != nil {
		return fmt.Errorf("ошибка обновления напоминания: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("ошибка получения количества строк: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("пользователь не найден")
	}

	return nil
}

func (s *Storage) SaveEvent(event *Event) error {
	startTimeStr := event.StartTime.Format("2006-01-02 15:04:05")
	endTimeStr := event.EndTime.Format("2006-01-02 15:04:05")

	query := `
	INSERT INTO events (id, user_id, title, description, start_time, end_time, link, notified)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(id) DO UPDATE SET
		title = excluded.title,
		description = excluded.description,
		start_time = excluded.start_time,
		end_time = excluded.end_time,
		link = excluded.link,
		notified = excluded.notified
	`

	_, err := s.db.Exec(query,
		event.ID,
		event.UserID,
		event.Title,
		event.Description,
		startTimeStr,
		endTimeStr,
		event.Link,
		event.Notified,
	)

	if err != nil {
		return fmt.Errorf("ошибка сохранения события: %w", err)
	}

	return nil
}

func (s *Storage) GetUpcomingEvents(userID int64, minutes int) ([]*Event, error) {
	query := `
	SELECT id, user_id, title, description, start_time, end_time, link, notified, created_at
	FROM events
	WHERE user_id = ?
	  AND notified = false
	  AND start_time BETWEEN ? AND ?
	ORDER BY start_time
	`

	now := time.Now().Format("2006-01-02 15:04:05")
	windowEnd := time.Now().Add(time.Duration(minutes) * time.Minute).Format("2006-01-02 15:04:05")

	rows, err := s.db.Query(query, userID, now, windowEnd)
	if err != nil {
		return nil, fmt.Errorf("ошибка поиска событий: %w", err)
	}
	defer rows.Close()

	var events []*Event
	for rows.Next() {
		event := &Event{}
		var startTimeStr, endTimeStr string
		err := rows.Scan(
			&event.ID,
			&event.UserID,
			&event.Title,
			&event.Description,
			&startTimeStr,
			&endTimeStr,
			&event.Link,
			&event.Notified,
			&event.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("ошибка чтения строки: %w", err)
		}

		event.StartTime, _ = parseTime(startTimeStr)
		event.EndTime, _ = parseTime(endTimeStr)

		events = append(events, event)
	}

	return events, nil
}

func parseTime(s string) (time.Time, error) {
	formats := []string{
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05Z",
		"2006-01-02T15:04:05-07:00",
		time.RFC3339,
	}

	for _, format := range formats {
		t, err := time.Parse(format, s)
		if err == nil {
			return t, nil
		}
	}

	return time.Time{}, fmt.Errorf("не удалось распарсить время '%s'", s)
}

func (s *Storage) GetAllEvents(userID int64) ([]*Event, error) {
	query := `
	SELECT id, user_id, title, description, start_time, end_time, link, notified, created_at
	FROM events
	WHERE user_id = ?
	ORDER BY start_time
	`

	rows, err := s.db.Query(query, userID)
	if err != nil {
		return nil, fmt.Errorf("ошибка поиска событий: %w", err)
	}
	defer rows.Close()

	var events []*Event
	for rows.Next() {
		event := &Event{}
		var startTimeStr, endTimeStr string
		err := rows.Scan(
			&event.ID,
			&event.UserID,
			&event.Title,
			&event.Description,
			&startTimeStr,
			&endTimeStr,
			&event.Link,
			&event.Notified,
			&event.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("ошибка чтения строки: %w", err)
		}

		event.StartTime, err = parseTime(startTimeStr)
		if err != nil {
			return nil, fmt.Errorf("ошибка парсинга start_time '%s': %w", startTimeStr, err)
		}
		event.EndTime, err = parseTime(endTimeStr)
		if err != nil {
			return nil, fmt.Errorf("ошибка парсинга end_time '%s': %w", endTimeStr, err)
		}

		events = append(events, event)
	}

	return events, nil
}

func (s *Storage) GetPastEvents(userID int64, limit int) ([]*Event, error) {
	query := `
	SELECT id, user_id, title, description, start_time, end_time, link, notified, created_at
	FROM events
	WHERE user_id = ?
	  AND end_time < ?
	ORDER BY end_time DESC
	LIMIT ?
	`

	nowStr := time.Now().Format("2006-01-02 15:04:05")

	rows, err := s.db.Query(query, userID, nowStr, limit)
	if err != nil {
		return nil, fmt.Errorf("ошибка поиска прошлых событий: %w", err)
	}
	defer rows.Close()

	var events []*Event
	for rows.Next() {
		event := &Event{}
		var startTimeStr, endTimeStr string
		err := rows.Scan(
			&event.ID,
			&event.UserID,
			&event.Title,
			&event.Description,
			&startTimeStr,
			&endTimeStr,
			&event.Link,
			&event.Notified,
			&event.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("ошибка чтения строки: %w", err)
		}

		event.StartTime, _ = parseTime(startTimeStr)
		event.EndTime, _ = parseTime(endTimeStr)

		events = append(events, event)
	}

	return events, nil
}

func (s *Storage) MarkEventAsNotified(eventID string) error {
	query := `UPDATE events SET notified = true WHERE id = ?`

	result, err := s.db.Exec(query, eventID)
	if err != nil {
		return fmt.Errorf("ошибка обновления события: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("ошибка получения количества строк: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("событие не найдено")
	}

	return nil
}

func (s *Storage) Close() error {
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}
