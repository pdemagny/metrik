package check

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
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
		// RequestTimeout is nil to inherit the global value; when set it must be positive.
		RequestTimeout *time.Duration `yaml:"requestTimeout,omitempty"`
		// FailureThreshold is nil to inherit the global value; 0 disables the circuit breaker.
		FailureThreshold *int          `yaml:"failureThreshold,omitempty"`
		Cooldown         time.Duration `yaml:"cooldown,omitempty"`
	}
)

// NewClient returns the HTTP client probes use: TLS 1.2 or newer, redirects
// not followed (the 3xx is the result), and timeout as the per-request limit.
func NewClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				MinVersion: tls.VersionTLS12,
			},
			MaxIdleConns:        10,
			MaxIdleConnsPerHost: 2,
		},
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
		Timeout: timeout,
	}
}

// ID identifies a check by name and endpoint, so a changed endpoint is a new check.
func (c *Check) ID() string { return c.Name + "\x00" + c.Endpoint }

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
		response, err := client.Do(req)
		if err != nil {
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

		// Handle server errors, which are transient and retried
		if response.StatusCode >= http.StatusInternalServerError {
			return c, errors.New("server error")
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
	return r, err
}
