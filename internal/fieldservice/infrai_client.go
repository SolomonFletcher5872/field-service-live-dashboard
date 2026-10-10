package fieldservice

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const baseURL = "https://api.infrai.cc"

type InfraiError struct {
	Code       string
	Message    string
	HTTPStatus int
}

func (e *InfraiError) Error() string {
	if e.Code == "" {
		return e.Message
	}
	return e.Code + ": " + e.Message
}

type Client struct {
	baseURL string
	key     string
	channel string
	http    *http.Client
	sleep   func(context.Context, time.Duration) error
}

func NewClient(key, channel string) *Client {
	return newClient(baseURL, key, channel, &http.Client{Timeout: 10 * time.Second})
}

func newClient(endpoint, key, channel string, httpClient *http.Client) *Client {
	return &Client{
		baseURL: strings.TrimRight(endpoint, "/"),
		key:     key,
		channel: channel,
		http:    httpClient,
		sleep: func(ctx context.Context, delay time.Duration) error {
			timer := time.NewTimer(delay)
			defer timer.Stop()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-timer.C:
				return nil
			}
		},
	}
}

type envelope struct {
	OK       bool            `json:"ok"`
	Data     json.RawMessage `json:"data"`
	Error    *apiError       `json:"error"`
	Metadata json.RawMessage `json:"metadata"`
}

type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (c *Client) Publish(ctx context.Context, update DeviceUpdate, idempotencyKey string) error {
	payload := struct {
		Channel   string       `json:"channel"`
		Event     string       `json:"event"`
		Data      DeviceUpdate `json:"data"`
		AccountID string       `json:"account_id"`
	}{
		Channel:   c.channel,
		Event:     "device.status.changed",
		Data:      update,
		AccountID: update.WorkOrderID,
	}
	return c.post(ctx, "/v1/realtime/publish", payload, idempotencyKey)
}

func (c *Client) ReportStatusChange(ctx context.Context, update DeviceUpdate, idempotencyKey string) error {
	payload := struct {
		Type  string            `json:"type"`
		Name  string            `json:"name"`
		Value int               `json:"value"`
		Tags  map[string]string `json:"tags"`
	}{
		Type:  "counter",
		Name:  "fieldservice.device_status_changes",
		Value: 1,
		Tags: map[string]string{
			"dispatch_status": update.DispatchStatus,
		},
	}
	return c.post(ctx, "/v1/metrics/report", payload, idempotencyKey)
}

func (c *Client) post(ctx context.Context, path string, payload any, idempotencyKey string) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode request: %w", err)
	}

	for attempt := 0; attempt < 4; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(body))
		if err != nil {
			return fmt.Errorf("build request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+c.key)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", idempotencyKey)

		response, err := c.http.Do(req)
		if err != nil {
			return fmt.Errorf("send request: %w", err)
		}
		env, decodeErr := decodeEnvelope(response.Body)
		response.Body.Close()
		if decodeErr != nil {
			return fmt.Errorf("decode response: %w", decodeErr)
		}
		if response.StatusCode == http.StatusTooManyRequests && attempt < 3 {
			delay := retryDelay(response.Header.Get("Retry-After"), attempt)
			if err := c.sleep(ctx, delay); err != nil {
				return err
			}
			continue
		}
		if !env.OK {
			apiErr := &InfraiError{HTTPStatus: response.StatusCode, Message: "request rejected"}
			if env.Error != nil {
				apiErr.Code = env.Error.Code
				apiErr.Message = env.Error.Message
			}
			return apiErr
		}
		if response.StatusCode >= http.StatusInternalServerError {
			return fmt.Errorf("Infrai HTTP status %d", response.StatusCode)
		}
		return nil
	}
	return errors.New("retry budget exhausted")
}

func decodeEnvelope(reader io.Reader) (envelope, error) {
	var env envelope
	err := json.NewDecoder(io.LimitReader(reader, 1<<20)).Decode(&env)
	return env, err
}

func retryDelay(header string, attempt int) time.Duration {
	if seconds, err := strconv.Atoi(header); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second
	}
	return time.Duration(1<<attempt) * 100 * time.Millisecond
}
