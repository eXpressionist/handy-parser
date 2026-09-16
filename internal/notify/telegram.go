package notify

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

var ErrNotConfigured = errors.New("telegram is not configured")

type DeliveryError struct {
	message     string
	permanent   bool
	rateLimited bool
}

func (e *DeliveryError) Error() string { return e.message }

func IsPermanent(err error) bool {
	var deliveryErr *DeliveryError
	return errors.As(err, &deliveryErr) && deliveryErr.permanent
}

func IsRateLimited(err error) bool {
	var deliveryErr *DeliveryError
	return errors.As(err, &deliveryErr) && deliveryErr.rateLimited
}

type Telegram struct {
	token    string
	chatID   string
	endpoint string
	client   *http.Client
}

func NewTelegram(token, chatID string, timeout time.Duration) *Telegram {
	token = strings.TrimSpace(token)
	return &Telegram{token: token, chatID: strings.TrimSpace(chatID), endpoint: "https://api.telegram.org/bot" + token + "/sendMessage", client: &http.Client{Timeout: timeout}}
}

func (t *Telegram) Configured() bool { return t.token != "" && t.chatID != "" }

func (t *Telegram) Send(ctx context.Context, message string) (time.Duration, error) {
	if !t.Configured() {
		return time.Hour, ErrNotConfigured
	}
	message = strings.TrimSpace(message)
	if len([]rune(message)) > 4096 {
		message = string([]rune(message)[:4090]) + "…"
	}
	form := url.Values{"chat_id": {t.chatID}, "text": {message}, "disable_web_page_preview": {"true"}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return time.Minute, fmt.Errorf("create Telegram request")
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := t.client.Do(req)
	if err != nil {
		return time.Minute, fmt.Errorf("Telegram request failed")
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	var result struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
		Parameters  struct {
			RetryAfter int `json:"retry_after"`
		} `json:"parameters"`
	}
	_ = json.Unmarshal(body, &result)
	if resp.StatusCode >= 200 && resp.StatusCode < 300 && result.OK {
		return 0, nil
	}
	retry := time.Minute
	if result.Parameters.RetryAfter > 0 {
		retry = time.Duration(result.Parameters.RetryAfter) * time.Second
	}
	description := strings.TrimSpace(result.Description)
	if description == "" {
		description = "HTTP " + strconv.Itoa(resp.StatusCode)
	}
	return retry, &DeliveryError{
		message:     "Telegram rejected message: " + description,
		permanent:   resp.StatusCode >= 400 && resp.StatusCode < 500 && resp.StatusCode != http.StatusRequestTimeout && resp.StatusCode != http.StatusTooManyRequests,
		rateLimited: resp.StatusCode == http.StatusTooManyRequests,
	}
}
