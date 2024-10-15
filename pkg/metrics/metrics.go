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
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/sethvargo/go-envconfig"

	"github.com/abcxyz/abc-updater/pkg/localstore"
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
var noopWriterOnce = sync.OnceValue(func() *Client {
	return &Client{optOut: true}
})

// contextKey is a private string type to prevent collisions in the context map.
type contextKey string

// MetricsConfig is the metrics configuration as parsed from the environment.
type MetricsConfig struct {
	ServerURL string `env:"METRICS_URL, default=https://abc-metrics.tycho.joonix.net"`
	NoMetrics bool   `env:"NO_METRICS"`
}

// Validate performs error validation and checking on the config.
func (c *MetricsConfig) Validate(ctx context.Context) error {
	if c.NoMetrics {
		return nil
	}

	var merr error

	// Use ParseRequestURI over Parse because Parse validation is more loose and will accept
	// things such as relative paths without a host.
	if _, err := url.ParseRequestURI(c.ServerURL); err != nil {
		merr = errors.Join(fmt.Errorf("failed to parse server URL: %w", err))
	}

	return merr
}

// Option is the Client option type.
type Option func(*Client) *Client

// WithHTTPClient instructs the Client to use given http.Client when
// making calls.
func WithHTTPClient(client *http.Client) Option {
	return func(c *Client) *Client {
		c.httpClient = client
		return c
	}
}

// WithLookuper instructs the Client to use given envconfig.Lookuper when
// loading configuration.
func WithLookuper(lookuper envconfig.Lookuper) Option {
	return func(c *Client) *Client {
		c.lookuper = lookuper
		return c
	}
}

// WithInstallInfoFilePath sets the path where install time file is stored.
func WithInstallInfoFilePath(path string) Option {
	return func(c *Client) *Client {
		c.installInfoFilePath = path
		return c
	}
}

// withNowOverride overrides the current time for testing purposes.
func withNowOverride(nowFunc func() time.Time) Option {
	return func(c *Client) *Client {
		c.nowFunc = nowFunc
		return c
	}
}

type Client struct {
	// optOut is a boolean that disables the client from sending any metrics.
	optOut bool

	// appID, appVersion, and identifier are the identifies for a given
	// installation.
	appID      string
	appVersion string
	identifier string

	// httpClient is the cached HTTP client to use for metrics.
	httpClient *http.Client

	// serverURL is the URL endpoint for the server.
	serverURL string

	// lookuper is the lookuper to use for processing the metrics environment
	// configuration.
	lookuper envconfig.Lookuper

	// installInfoFilePath is the path on disk to the file that contains the
	// install info. The default value is computed from the appID, but it can be
	// overridden for testing.
	installInfoFilePath string

	// nowFunc is a function that returns the current time. By default it uses
	// [time.Now], but can be overridden in tests.
	nowFunc func() time.Time
}

// New provides a Client based on provided values and options.
// Upon error recommended to use NoopWriter().
func New(ctx context.Context, appID, version string, opt ...Option) (*Client, error) {
	if len(appID) == 0 {
		return nil, fmt.Errorf("appID cannot be empty")
	}

	// Create a client with the defaults.
	client := &Client{
		appID:      appID,
		appVersion: version,
		httpClient: &http.Client{
			Timeout: 1 * time.Second,
		},
		lookuper: envconfig.PrefixLookuper(strings.ToUpper(appID)+"_", envconfig.OsLookuper()),
		nowFunc:  time.Now,
	}

	// Process overrides.
	for _, o := range opt {
		client = o(client)
	}

	// After applying all options, check if the install file was given. If not,
	// dynamically compute it.
	if client.installInfoFilePath == "" {
		dir, err := localstore.DefaultDir(appID)
		if err != nil {
			return nil, fmt.Errorf("could not calculate install time path: %w", err)
		}
		client.installInfoFilePath = filepath.Join(dir, installTimeFileName)
	}

	// Process the metrics config from the environment and set any configuration
	// on the client.
	var metricsConfig MetricsConfig
	if err := envconfig.ProcessWith(ctx, &envconfig.Config{
		Target:   &metricsConfig,
		Lookuper: client.lookuper,
	}); err != nil {
		return nil, fmt.Errorf("failed to process metrics configuration: %w", err)
	}
	if err := metricsConfig.Validate(ctx); err != nil {
		return nil, fmt.Errorf("failed to validate metrics configuration: %w", err)
	}
	client.serverURL = metricsConfig.ServerURL

	// Short Circuit if user opted out of metrics.
	if metricsConfig.NoMetrics {
		return NoopWriter(), nil
	}

	// Get or create the installation identifier.
	installInfo, err := loadInstallInfo(client.installInfoFilePath)
	if err != nil {
		client.identifier = client.nowFunc().
			UTC().
			Truncate(installTimeResolution).
			Format(time.RFC3339Nano)

		if err := storeInstallInfo(client.installInfoFilePath, &InstallInfo{
			InstallTime: client.identifier,
		}); err != nil {
			logging.FromContext(ctx).DebugContext(ctx, "failed to store new install time", "error", err.Error())
		}
	} else {
		client.identifier = installInfo.InstallTime
	}

	return client, nil
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
		InstallTime: c.identifier,
	}); err != nil {
		return fmt.Errorf("failed to marshal metrics as json: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.serverURL+"/sendMetrics", &buf)
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
