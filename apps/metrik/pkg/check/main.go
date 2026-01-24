package check

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"syscall"
	"time"

	"github.com/cenkalti/backoff/v5"
	"go.uber.org/zap"
)

type (
	APIResponse struct {
		Message string `json:"message"`
		Status  int    `json:"status"`
		Reason  string `json:"reason,omitempty"`
	}
	Result struct {
		Duration time.Duration `json:"time"`
		Code     int           `json:"code"`
		Body     *APIResponse  `json:"body"`
	}
	Check struct {
		Name           string        `yaml:"name"`
		Endpoint       string        `yaml:"endpoint"`
		AlertThreshold time.Duration `yaml:"alertThreshold"`
		MaxTries       uint          `yaml:"maxTries"`
		RequestTimeout time.Duration `yaml:"requestTimeout,omitempty"`
	}
)

func NewCheck(name, endpoint string, alertThreshold time.Duration, maxTries uint) *Check {
	return &Check{
		Name:           name,
		Endpoint:       endpoint,
		AlertThreshold: alertThreshold,
		MaxTries:       maxTries,
	}
}

func (c *Check) Run(ctx context.Context, logger *zap.Logger, client *http.Client) (*Result, error) {
	r := &Result{}
	// Prepare the HTTP Request
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.Endpoint, http.NoBody)
	if err != nil {
		logger.Error("unable to create request: %s\n", zap.Error(err))
		return nil, err
	}

	// Wrap request in retry logic
	operation := func() (*Check, error) {
		responseTime := time.Now()
		response, err := client.Do(req.WithContext(ctx))
		if errors.Is(err, syscall.ECONNREFUSED) {
			logger.Error("connection was refused", zap.Error(err))
			return c, err
		} else if err != nil {
			logger.Error("unable to perform check", zap.Error(err))
			return c, err
		}
		defer response.Body.Close()

		// Set results
		r.Duration = time.Since(responseTime)
		r.Code = response.StatusCode

		// Handle Rate Limiting
		if response.StatusCode == http.StatusTooManyRequests {
			seconds, err := strconv.ParseInt(response.Header.Get("Retry-After"), 10, 64)
			if err == nil {
				return c, backoff.RetryAfter(int(seconds))
			}
		}

		// Handle client errors
		if response.StatusCode >= http.StatusBadRequest && response.StatusCode < http.StatusInternalServerError {
			return c, backoff.Permanent(errors.New("client error"))
		}

		// Parse Request Body
		if err := json.NewDecoder(response.Body).Decode(&r.Body); err != nil {
			logger.Error("unable to decode response", zap.Error(err))
			return c, err
		}

		return c, nil
	}

	_, err = backoff.Retry(
		ctx,
		operation,
		backoff.WithBackOff(backoff.NewExponentialBackOff()),
		backoff.WithMaxTries(c.MaxTries),
		backoff.WithNotify(func(err error, duration time.Duration) {
			logger.Info("retrying ...",
				zap.Error(err),
				zap.Duration("duration", duration),
			)
		}),
	)
	if err != nil {
		return r, err
	}

	return r, nil
}
