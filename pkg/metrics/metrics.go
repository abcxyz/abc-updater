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

	"github.com/abcxyz/abc-updater/pkg/optout"
	"github.com/abcxyz/pkg/logging"
)

const (
	installIDFileName     = "id.json"
	maxErrorResponseBytes = 2048
)

type metricsConfig struct {
	ServerURL string `env:"ABC_METRICS_URL, default=https://abc-updater-metrics.tycho.joonix.net"`
}

type options struct {
	httpClient *http.Client
	lookuper   envconfig.Lookuper
	// Optional override for install id file location. Mostly intended for testing.
	// If empty uses default location.
	installIDFileOverride string
}

// Option is the metrics Client option type.
type Option func(*options) *options

// WithHTTPClient instructs the Client to use given http.Client when making calls.
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

// For testing. Can expose externally if a need arises in the future.
func withInstallIDFileOverride(path string) Option { //nolint:unused
	return func(o *options) *options {
		o.installIDFileOverride = path
		return o
	}
}

// TODO: should Client be an interface so we can have a noop client returned for caller to use in case of error?
type Client struct {
	appID      string
	version    string
	installID  string
	httpClient *http.Client
	optOut     *optout.OptOutSettings
	config     *metricsConfig
}

// New provides a Client based on provided values and options.
func New(ctx context.Context, appID, version string, opt ...Option) (*Client, error) {
	opts := &options{}

	for _, o := range opt {
		opts = o(opts)
	}

	// Default to the environment loader.
	if opts.lookuper == nil {
		opts.lookuper = envconfig.OsLookuper()
	}

	optOut, err := optout.LoadOptOutSettings(ctx, opts.lookuper, appID)
	if err != nil {
		return nil, fmt.Errorf("failed to load opt out settings: %w", err)
	}

	// Short Circuit if user opted out of metrics.
	if optOut.NoMetrics {
		return &Client{optOut: optOut}, nil
	}

	// Default to 1 second timeout httpClient.
	if opts.httpClient == nil {
		opts.httpClient = &http.Client{Timeout: 1 * time.Second}
	}

	var c metricsConfig
	prefixLookuper := envconfig.PrefixLookuper(strings.ToUpper(appID)+"_", opts.lookuper)
	if err := envconfig.ProcessWith(ctx, &envconfig.Config{
		Target:   &c,
		Lookuper: prefixLookuper,
	}); err != nil {
		return nil, fmt.Errorf("failed to process envconfig: %w", err)
	}

	// Use ParseRequestURI over Parse because Parse validation is more loose and will accept
	// things such as relative paths without a host.
	if _, err := url.ParseRequestURI(c.ServerURL); err != nil {
		return nil, fmt.Errorf("failed to parse server url: %w", err)
	}

	storedID, err := loadInstallID(appID, opts.installIDFileOverride)
	var installID string
	if err != nil || storedID == nil {
		installID, err = generateInstallID()
		if err != nil {
			// TODO: should we just have a preset ID for this case that we don't save?
			return nil, err
		}
		err = storeInstallID(appID, opts.installIDFileOverride, &InstallIDData{
			IDCreatedTimestamp: time.Now().Unix(),
			InstallID:          installID,
		})
		if err != nil {
			logging.FromContext(ctx).DebugContext(ctx, "error storing installID", "error", err.Error())
		}
	} else {
		installID = storedID.InstallID
	}

	return &Client{
		appID:      appID,
		version:    version,
		installID:  installID,
		httpClient: opts.httpClient,
		optOut:     optOut,
		config:     &c,
	}, nil
}

type SendMetricRequest struct {
	// The ID of the application to check.
	AppID string `json:"appId"`

	// The version of the app to check for updates.
	// Should be of form vMAJOR[.MINOR[.PATCH[-PRERELEASE][+BUILD]]] (e.g., v1.0.1)
	Version string `json:"version"`

	Metrics map[string]int `json:"metrics"`

	// InstallID. Expected to be a hex 8-4-4-4-12 formatted v4 UUID.
	InstallID string `json:"installId"`
}

// SendSync sends information about application usage. Noop if metrics
// are opted out.
// Accepts a context for cancellation.
func (c *Client) SendSync(ctx context.Context, metrics map[string]int) error {
	if c.optOut.NoMetrics {
		return nil
	}

	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(&SendMetricRequest{
		AppID:     c.appID,
		Version:   c.version,
		Metrics:   metrics,
		InstallID: c.installID,
	}); err != nil {
		return fmt.Errorf("failed to marshal metrics as json: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf(c.config.ServerURL+"/sendMetrics"), &buf)
	if err != nil {
		return fmt.Errorf("failed to create http request: %w", err)
	}
	req.Header.Set("User-Agent", "Go-http-client/1.1 github.com/abcxyz/abc-updater")
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
