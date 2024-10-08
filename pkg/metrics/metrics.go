// Copyright 2024 The Authors (see AUTHORS file)
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package metrics includes code specific to sending usage metrics.
package metrics

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/sethvargo/go-envconfig"

	"github.com/abcxyz/pkg/logging"
)

const (
	installTimeFileName   = "id.json"
	installTimeResolution = time.Minute // Internal Use: Consult PWG before shortening.
	maxErrorResponseBytes = 2048

	// metricsKey points to the value in the context where the Client is stored.
	metricsKey = contextKey("metricsClient")
)

// noopWriterOnce returns a function that returns the noop Client, which
// is a Client with optOut = true.
//
// It is initialized once when called the first time.
var noopWriterOnce = sync.OnceValue[*Client](func() *Client {
	return &Client{optOut: true}
})

// contextKey is a private string type to prevent collisions in the context map.
type contextKey string

type metricsConfig struct {
	ServerURL string `env:"METRICS_URL, default=https://abc-metrics.tycho.joonix.net"`
	NoMetrics bool   `env:"NO_METRICS"`
}

type options struct {
	httpClient *http.Client
	lookuper   envconfig.Lookuper
	// Optional override for install time file location. Mostly intended for
	// testing. If empty uses default location.
	installInfoFileOverride string
	// Optional override for time for testing.
	nowFn func() time.Time
}

// Option is the Client option type.
type Option func(*options) *options

// WithHTTPClient instructs the Client to use given http.Client when
// making calls.
func WithHTTPClient(client *http.Client) Option {
	return func(o *options) *options {
		o.httpClient = client
		return o
	}
}

// WithLookuper instructs the Client to use given envconfig.Lookuper when
// loading configuration.
func WithLookuper(lookuper envconfig.Lookuper) Option {
	return func(o *options) *options {
		o.lookuper = lookuper
		return o
	}
}

// WithInstallInfoFileOverride overrides the path where install time file is stored.
func WithInstallInfoFileOverride(path string) Option {
	return func(o *options) *options {
		o.installInfoFileOverride = path
		return o
	}
}

// withNowOverride overrides the current time for testing purposes.
func withNowOverride(nowFn func() time.Time) Option {
	return func(o *options) *options {
		o.nowFn = nowFn
		return o
	}
}

type Client struct {
	appID       string
	appVersion  string
	installTime string
	httpClient  *http.Client
	optOut      bool
	config      *metricsConfig
}

// New provides a Client based on provided values and options.
// Upon error recommended to use NoopWriter().
func New(ctx context.Context, appID, version string, opt ...Option) (*Client, error) {
	if len(appID) == 0 {
		return nil, fmt.Errorf("appID cannot be empty")
	}

	opts := &options{}

	for _, o := range opt {
		opts = o(opts)
	}

	// Default to the environment loader.
	if opts.lookuper == nil {
		opts.lookuper = envconfig.OsLookuper()
		opts.lookuper = envconfig.PrefixLookuper(strings.ToUpper(appID)+"_", opts.lookuper)
	}

	var c metricsConfig
	if err := envconfig.ProcessWith(ctx, &envconfig.Config{
		Target:   &c,
		Lookuper: opts.lookuper,
	}); err != nil {
		return nil, fmt.Errorf("failed to process envconfig: %w", err)
	}

	// Short Circuit if user opted out of metrics.
	if c.NoMetrics {
		return NoopWriter(), nil
	}

	// Default to 1 second timeout httpClient.
	if opts.httpClient == nil {
		opts.httpClient = &http.Client{Timeout: 1 * time.Second}
	}

	// Use ParseRequestURI over Parse because Parse validation is more loose and will accept
	// things such as relative paths without a host.
	if _, err := url.ParseRequestURI(c.ServerURL); err != nil {
		return nil, fmt.Errorf("failed to parse server URL: %w", err)
	}

	storedTime, err := loadInstallTime(appID, opts.installInfoFileOverride)
	var installTime string

	if err != nil {
		var now time.Time
		if opts.nowFn != nil {
			now = opts.nowFn()
		} else {
			now = time.Now()
		}
		if installTimeBuf, err := now.UTC().Truncate(installTimeResolution).MarshalText(); err != nil {
			return nil, fmt.Errorf("time.Now() could not be converted to RFC3339, check system clock: %w", err)
		} else {
			installTime = string(installTimeBuf)
		}

		if err = storeInstallTime(appID, opts.installInfoFileOverride, &installInfo{
			InstallTime: installTime,
		}); err != nil {
			logging.FromContext(ctx).DebugContext(ctx, "error storing InstallTime", "error", err.Error())
		}
	} else {
		installTime = storedTime.InstallTime
	}

	return &Client{
		appID:       appID,
		appVersion:  version,
		installTime: installTime,
		httpClient:  opts.httpClient,
		config:      &c,
	}, nil
}

type SendMetricRequest struct {
	// The ID of the application to check.
	AppID string `json:"appId"`

	// The version of the app to check for updates.
	// Should be of form vMAJOR[.MINOR[.PATCH[-PRERELEASE][+BUILD]]] (e.g., v1.0.1)
	AppVersion string `json:"appVersion"`

	// Only single item is used now, map used for flexibility in the future.
	Metrics map[string]int64 `json:"metrics"`

	// InstallTime. Time of install in UTC. String in rfc3339 format.
	InstallTime string `json:"installTime"`
}

// WriteMetric sends information about application usage, blocking until
// completion. It accepts a context for cancellation, or will time out after 5
// seconds, whatever is sooner. It is a noop if metrics are opted out.
func (c *Client) WriteMetric(ctx context.Context, name string, count int64) error {
	if c.optOut {
		return nil
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(&SendMetricRequest{
		AppID:       c.appID,
		AppVersion:  c.appVersion,
		Metrics:     map[string]int64{name: count},
		InstallTime: c.installTime,
	}); err != nil {
		return fmt.Errorf("failed to marshal metrics as json: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf(c.config.ServerURL+"/sendMetrics"), &buf)
	if err != nil {
		return fmt.Errorf("failed to create http request: %w", err)
	}
	req.Header.Set("User-Agent", "github.com/abcxyz/abc-updater")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to make http request: %w", err)
	}
	defer resp.Body.Close()

	// Future releases may be more strict.
	if resp.StatusCode >= 300 || resp.StatusCode <= 199 {
		b, err := io.ReadAll(io.LimitReader(resp.Body, maxErrorResponseBytes))
		if err != nil {
			return fmt.Errorf("received %d response, unable to read response body", resp.StatusCode)
		}
		return fmt.Errorf("received %d response: %s", resp.StatusCode, string(b))
	}

	// For now, ignore response body for happy responses.
	// Future versions may parse warnings for debug logging.
	return nil
}

// NoopWriter returns a Client which is opted-out and will not send
// metrics.
func NoopWriter() *Client {
	return noopWriterOnce()
}

// WithClient creates a new context with the provided Client attached.
func WithClient(ctx context.Context, client *Client) context.Context {
	return context.WithValue(ctx, metricsKey, client)
}

// FromContext returns the metrics Client stored in the context.
// If no such Client exists, a default noop Client is returned.
func FromContext(ctx context.Context) *Client {
	if client, ok := ctx.Value(metricsKey).(*Client); ok {
		return client
	}
	return NoopWriter()
}
