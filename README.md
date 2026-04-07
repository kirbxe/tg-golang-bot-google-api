### 1. Получение Telegram токена

1. Откройте (https://t.me/BotFather) в Telegram
2. Отправьте команду `/newbot`
3. Придумайте имя и username для бота 
4. Сохраните полученный токен 

### 2. Настройка Google Cloud Project

#### Создание проекта

1. Перейдите в https://console.cloud.google.com/
2. Создайте новый проект: **Select a project** → **New Project**
3. Назовите его (например, "tg-calendar-bot")

#### Включение Google Calendar API

1. В меню выберите **APIs & Services** → **Library**
2. Найдите "Google Calendar API"
3. Нажмите **Enable**

#### Создание OAuth2 credentials

1. Перейдите в **APIs & Services** → **Credentials**
2. Нажмите **Create Credentials** → **OAuth client ID**
3. Если спрашивают - настройте **OAuth consent screen**:
   - User Type: **External**
   - App name: "Telegram Calendar Bot"
   - Сохраните
4. Создайте **OAuth client ID**:
   - Application type: **Web application**
   - Name: "Telegram Bot"
   - **Authorized redirect URIs**: добавьте `http://localhost:8080/callback`
5. Нажмите **Create**
6. Сохраните **Client ID** и **Client Secret**
7. Далее нужно добавить Test Users, т.к. для полного доступа к продукту нужна верификация от Google

### 3. Настройка проекта

#### Создание .env файла

Скопируйте пример и заполните значения:

```bash
copy .env.example .env
```

Откройте `.env` и заполните:

```env
# Токен от @BotFather (обязательно!)
TELEGRAM_BOT_TOKEN=1234567890:ABCdefGHIjklMNOpqrsTUVwxyz

# Из Google Cloud Console (обязательно!)
GOOGLE_CLIENT_ID=123456789-abc123def456.apps.googleusercontent.com
GOOGLE_CLIENT_SECRET=GOCSPX-abcdefghijklmnop

# Ключ шифрования (ровно 32 символа для AES-256)
# Сгенерировать можно так: openssl rand -hex 32
# Или просто придумайте 32 символа
ENCRYPTION_KEY=0123456789abcdef0123456789abcdef

# Путь к базе данных (можно оставить как есть)
DB_PATH=./data/bot.db

# Порт для OAuth callback (должен совпадать с Google Cloud)
OAUTH_CALLBACK_PORT=8080

# Интервал проверки календаря в минутах
CHECK_INTERVAL_MINUTES=5

# Лог файл
LOG_FILE=./logs/bot.log
LOG_LEVEL=INFO
```

## Команды бота


`/start` Начать работу, регистрация 
`/help` Справка по командам 
`/auth` Авторизация в Google Calendar 
`/reauth` Переподключить Google Calendar 
`/set_reminder <мин>`  Установить напоминание (например: `/set_reminder 15`) 
`/next` Следующая встреча 
`/history` Последние 10 встреч 
`/stop` Отключить уведомления 
`/check_calendar` Получить ближайшие встречи


1. Пользователь отправляет `/auth`
2. Бот генерирует OAuth URL и показывает ссылку
3. Пользователь переходит по ссылке и авторизуется в Google
4. Google перенаправляет на `http://localhost:8080/callback?code=XXX`
5. OAuth сервер получает код и обменивает его на refresh token
6. Токен сохраняется в БД (в реальном проекте - зашифрованный)
7. Бот подтверждает успешное подключение



