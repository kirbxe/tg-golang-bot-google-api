package calendar

import (
	"context"
	"fmt"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/option"
)

type Token = oauth2.Token

type Calendar struct {
	svc *calendar.Service
}

type Config struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string
}

func New(ctx context.Context, token *oauth2.Token, cfg Config) (*Calendar, error) {
	oauthConfig := &oauth2.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		RedirectURL:  cfg.RedirectURL,
		Scopes:       []string{calendar.CalendarReadonlyScope},
		Endpoint:     google.Endpoint,
	}

	httpClient := oauthConfig.Client(ctx, token)

	svc, err := calendar.NewService(ctx, option.WithHTTPClient(httpClient))
	if err != nil {
		return nil, fmt.Errorf("ошибка создания сервиса Calendar: %w", err)
	}

	return &Calendar{svc: svc}, nil
}

func GetAuthURL(cfg Config, state string) string {
	oauthConfig := &oauth2.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		RedirectURL:  cfg.RedirectURL,
		Scopes:       []string{calendar.CalendarReadonlyScope},
		Endpoint:     google.Endpoint,
	}

	return oauthConfig.AuthCodeURL(state, oauth2.AccessTypeOffline)
}

func ExchangeToken(ctx context.Context, cfg Config, code string) (*oauth2.Token, error) {
	oauthConfig := &oauth2.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		RedirectURL:  cfg.RedirectURL,
		Scopes:       []string{calendar.CalendarReadonlyScope},
		Endpoint:     google.Endpoint,
	}

	token, err := oauthConfig.Exchange(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("ошибка обмена кода на токен: %w", err)
	}

	return token, nil
}

type Event struct {
	ID          string
	Title       string
	Description string
	StartTime   time.Time
	EndTime     time.Time
	Link        string
}

func (c *Calendar) GetUpcomingEvents(ctx context.Context, maxResults int64) ([]*Event, error) {
	events, err := c.svc.Events.List("primary").
		ShowDeleted(false).
		SingleEvents(true).
		TimeMin(time.Now().Format(time.RFC3339)).
		MaxResults(maxResults).
		OrderBy("startTime").
		Do()
	if err != nil {
		return nil, fmt.Errorf("ошибка получения событий: %w", err)
	}

	var result []*Event
	for _, item := range events.Items {
		start := item.Start.DateTime
		if start == "" {
			start = item.Start.Date
		}
		startTime, err := time.Parse(time.RFC3339, start)
		if err != nil {
			startTime, err = time.Parse("2006-01-02", start)
			if err != nil {
				continue
			}
		}

		end := item.End.DateTime
		if end == "" {
			end = item.End.Date
		}
		endTime, err := time.Parse(time.RFC3339, end)
		if err != nil {
			endTime, err = time.Parse("2006-01-02", end)
			if err != nil {
				continue
			}
		}

		link := item.HangoutLink
		if link == "" {
			link = item.HtmlLink
		}

		event := &Event{
			ID:          item.Id,
			Title:       item.Summary,
			Description: item.Description,
			StartTime:   startTime,
			EndTime:     endTime,
			Link:        link,
		}

		result = append(result, event)
	}

	return result, nil
}

func (c *Calendar) GetCalendars(ctx context.Context) ([]string, error) {
	calendars, err := c.svc.CalendarList.List().Do()
	if err != nil {
		return nil, fmt.Errorf("ошибка получения списка календарей: %w", err)
	}

	var result []string
	for _, cal := range calendars.Items {
		result = append(result, fmt.Sprintf("%s (%s)", cal.Summary, cal.Id))
	}

	return result, nil
}
