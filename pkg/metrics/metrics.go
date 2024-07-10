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
	"time"

	"github.com/sethvargo/go-envconfig"

	"github.com/abcxyz/pkg/logging"
)

const (
	installIDFileName     = "id.json"
	maxErrorResponseBytes = 2048

	// metricsKey points to the value in the context where the client is stored.
	metricsKey = contextKey("metricsClient")
)

// Assert client implements MetricWriter.
var _ MetricWriter = (*client)(nil)

// contextKey is a private string type to prevent collisions in the context map.
type contextKey string

type metricsConfig struct {
	ServerURL string `env:"METRICS_URL, default=https://abc-updater-metrics.tycho.joonix.net"`
	NoMetrics bool   `env:"NO_METRICS"`
}

type options struct {
	httpClient *http.Client
	lookuper   envconfig.Lookuper
	// Optional override for install id file location. Mostly intended for testing.
	// If empty uses default location.
	installIDFileOverride string
}

// Option is the MetricWriter option type.
type Option func(*options) *options

// WithHTTPClient instructs the MetricWriter to use given http.Client when
// making calls.
func WithHTTPClient(client *http.Client) Option {
	return func(o *options) *options {
		o.httpClient = client
		return o
	}
}

// WithLookuper instructs the MetricWriter to use given envconfig.Lookuper when
// loading configuration.
func WithLookuper(lookuper envconfig.Lookuper) Option {
	return func(o *options) *options {
		o.lookuper = lookuper
		return o
	}
}

// WithInstallIDFileOverride overrides the path where install ID file is stored.
func WithInstallIDFileOverride(path string) Option {
	return func(o *options) *options {
		o.installIDFileOverride = path
		return o
	}
}

// MetricWriter is a client for reporting metrics about an application's usage.
type MetricWriter interface {
	WriteMetric(ctx context.Context, name string, count int64) error
	WriteMetricAsync(ctx context.Context, name string, count int64) func() error
}

type client struct {
	AppID      string
	AppVersion string
	InstallID  string
	HTTPClient *http.Client
	OptOut     bool
	Config     *metricsConfig
}

// New provides a MetricWriter based on provided values and options.
// Upon error recommended to use NoopWriter().
func New(ctx context.Context, appID, version string, opt ...Option) (MetricWriter, error) {
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

	storedID, err := loadInstallID(appID, opts.installIDFileOverride)
	var installID string
	if err != nil || storedID == nil {
		installID, err = generateInstallID()
		if err != nil {
			return nil, err
		}

		if err = storeInstallID(appID, opts.installIDFileOverride, &InstallIDData{
			InstallID: installID,
		}); err != nil {
			logging.FromContext(ctx).DebugContext(ctx, "error storing InstallID", "error", err.Error())
		}
	} else {
		installID = storedID.InstallID
	}

	return &client{
		AppID:      appID,
		AppVersion: version,
		InstallID:  installID,
		HTTPClient: opts.httpClient,
		Config:     &c,
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

	// InstallID. Expected to be a random base64 value.
	InstallID string `json:"installId"`
}

// WriteMetric sends information about application usage, blocking until
// completion. It accepts a context for cancellation, or will time out after 5
// seconds, whatever is sooner. It is a noop if metrics are opted out.
func (c *client) WriteMetric(ctx context.Context, name string, count int64) error {
	if c.OptOut {
		return nil
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(&SendMetricRequest{
		AppID:      c.AppID,
		AppVersion: c.AppVersion,
		Metrics:    map[string]int64{name: count},
		InstallID:  c.InstallID,
	}); err != nil {
		return fmt.Errorf("failed to marshal metrics as json: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf(c.Config.ServerURL+"/sendMetrics"), &buf)
	if err != nil {
		return fmt.Errorf("failed to create http request: %w", err)
	}
	req.Header.Set("User-Agent", "github.com/abcxyz/abc-updater")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.HTTPClient.Do(req)
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

// WriteMetricAsync is like [WriteMetric], but it sends the metric in the
// background in a goroutine. The resulting closure can be deferred to ensure
// the metric finishes writing before process termination. For example:
//
//	done := client.WriteMetricsAsync(ctx, "foo", 1)
//	defer done()
//
// Or with error handling:
//
//	done := client.WriteMetricsAsync(ctx, "foo", 1)
//	defer func() {
//	  if err := done(); err != nil {
//	    // handle error
//	  }
//	}()
func (c *client) WriteMetricAsync(ctx context.Context, name string, count int64) func() error {
	errCh := make(chan error, 1)
	go func() {
		defer close(errCh)
		errCh <- c.WriteMetric(ctx, name, count)
	}()

	return func() error {
		return <-errCh
	}
}

// NoopWriter returns a MetricWriter which is opted-out and will not send metrics.
func NoopWriter() MetricWriter {
	return &client{OptOut: true}
}

// WithClient creates a new context with the provided client attached.
func WithClient(ctx context.Context, client *client) context.Context {
	return context.WithValue(ctx, metricsKey, client)
}

// FromContext returns the metrics client stored in the context.
// If no such client exists, a default noop client is returned.
func FromContext(ctx context.Context) MetricWriter {
	if client, ok := ctx.Value(metricsKey).(MetricWriter); ok {
		return client
	}
	return NoopWriter()
}
