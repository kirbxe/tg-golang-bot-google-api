-- Миграция 001: Начальная схема базы данных
-- Создаёт таблицы для пользователей и событий

-- Таблица пользователей
-- Хранит информацию о пользователях бота и их настройки
CREATE TABLE IF NOT EXISTS users (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    telegram_id INTEGER UNIQUE NOT NULL,
    username TEXT,
    google_token BLOB,
    reminder_minutes INTEGER DEFAULT 15,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Таблица событий
-- Хранит информацию о встречах из Google Calendar
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

-- Индекс для быстрого поиска событий по пользователю
CREATE INDEX IF NOT EXISTS idx_events_user_id ON events(user_id);

-- Индекс для быстрого поиска по времени начала
CREATE INDEX IF NOT EXISTS idx_events_start_time ON events(start_time);
