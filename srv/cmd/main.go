// Copyright 2023 The Authors (see AUTHORS file)
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

// Package main implements a simple HTTP/JSON REST example.
package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/sethvargo/go-envconfig"

	"github.com/abcxyz/abc-updater/srv/pkg"

	"github.com/abcxyz/pkg/logging"
	"github.com/abcxyz/pkg/renderer"
	"github.com/abcxyz/pkg/serving"
)

// TODO: figure out how to make modules so this doesn't get re-defined multiple places
type SendMetricRequest struct {
	// The ID of the application to check.
	AppID string `json:"appId"`

	// The version of the app to check for updates.
	// Should be of form vMAJOR[.MINOR[.PATCH[-PRERELEASE][+BUILD]]] (e.g., v1.0.1)
	AppVersion string `json:"appVersion"`

	// TODO: this is a bit different from design doc, is it ok?
	Metrics map[string]int64 `json:"metrics"`

	// InstallID. Expected to be a hex 8-4-4-4-12 formatted v4 UUID.
	InstallID string `json:"installId"`
}

type metricsServerConfig struct {
	ServerURL               string        `env:"ABC_UPDATER_METRICS_METADATA_URL, default=https://abc-updater.tycho.joonix.net"`
	MetadataUpdateFrequency time.Duration `env:"ABC_UPDATER_METRICS_METADATA_UPDATE_FREQUENCY, default=1m"`
	Port                    string        `env:"ABC_UPDATER_METRICS_SERVER_PORT, default=8080"`
}

func handleMetric(h *renderer.Renderer, db *pkg.MetricsDB) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logger := logging.FromContext(r.Context())
		metricLogger := logger.WithGroup("metric")
		logger.InfoContext(r.Context(), "handling request")

		metrics, err := pkg.DecodeRequest[SendMetricRequest](r.Context(), w, r, h)
		if err != nil {
			// Error response already handled by pkg.DecodeRequest.
			return
		}

		allowedMetrics, err := db.GetAllowedMetrics(metrics.AppID)
		if err != nil {
			h.RenderJSON(w, http.StatusNotFound, err)
			logger.WarnContext(r.Context(), "received metric request for unknown app")
			return
		}

		// Currently we only expose an API for a single metric on the client,
		// but I suspect multiple metrics will be added later on, and effort is
		// about the same to support both.
		for name, count := range metrics.Metrics {
			if allowedMetrics.MetricAllowed(name) {
				// TODO: does this leak sensitive information? Is default logger preferred.
				metricLogger.InfoContext(r.Context(), "metric received", "appID", metrics.AppID, "appVersion", metrics.AppVersion, "installId", metrics.InstallID, "name", name, "count", count)
			} else {
				// TODO: do we want to return a warning to client or fail silently?
				logger.WarnContext(r.Context(), "received unknown metric for app", "appID", metrics.AppID)
			}
		}

		// Client does not currently read body, future changes are acceptable.
		h.RenderJSON(w, http.StatusAccepted, map[string]string{"message": "ok"})
	})
}

// realMain creates an example backend HTTP server.
// This server supports graceful stopping and cancellation.
func realMain(ctx context.Context) error {
	logger := logging.FromContext(ctx)
	// Make a new renderer for rendering json.
	// Don't provide filesystem as we don't have templates to render.
	h, err := renderer.New(ctx, nil,
		renderer.WithOnError(func(err error) {
			logger.ErrorContext(ctx, "failed to render", "error", err)
		}))
	if err != nil {
		return fmt.Errorf("failed to create renderer for main server: %w", err)
	}

	var c metricsServerConfig
	if err := envconfig.ProcessWith(ctx, &envconfig.Config{
		Target:   &c,
		Lookuper: envconfig.OsLookuper(),
	}); err != nil {
		return fmt.Errorf("failed to process envconfig: %w", err)
	}
	if c.MetadataUpdateFrequency.Milliseconds() < 100 {
		return fmt.Errorf("invalid config: METADATA_UPDATE_FREQUENCY must be at least 100ms")
	}

	dbUpdateParams := &pkg.MetricsLoadParams{
		ServerURL: c.ServerURL,
		Client:    &http.Client{Timeout: 2 * time.Second},
	}

	db := &pkg.MetricsDB{}
	if err := db.Update(ctx, dbUpdateParams); err != nil {
		return fmt.Errorf("failed to load metrics definitions on startup: %w", err)
	}

	// Fetch new metadata for DB occasionally.
	done := make(chan bool)
	ticker := time.NewTicker(c.MetadataUpdateFrequency)
	defer ticker.Stop()
	defer func() { done <- true }()
	go func() {
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				logger.DebugContext(ctx, "Updating metrics definitions.")
				// Error logged by db.
				_ = db.Update(ctx, dbUpdateParams)
			}
		}
	}()

	mux := http.NewServeMux()
	mux.Handle("POST /sendMetrics", handleMetric(h, db))

	httpServer := &http.Server{
		Addr:              c.Port,
		Handler:           mux,
		ReadHeaderTimeout: 2 * time.Second,
	}

	logger.InfoContext(ctx, "starting server", "port", c.Port)
	server, err := serving.New(c.Port)
	if err != nil {
		return fmt.Errorf("error creating server: %w", err)
	}

	// This will block until the provided context is cancelled.
	if err := server.StartHTTP(ctx, httpServer); err != nil {
		return fmt.Errorf("error starting server: %w", err)
	}
	return nil
}

func main() {
	// creates a context that exits on interrupt signal.
	ctx, done := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer done()
	ctx = logging.WithLogger(ctx, logging.NewFromEnv("ABC_UPDATER_METRICS_"))
	logger := logging.FromContext(ctx)

	flag.Parse()
	if err := realMain(logging.WithLogger(ctx, logger)); err != nil {
		done()
		logger.ErrorContext(ctx, err.Error())
		os.Exit(1)
	}
	logger.InfoContext(ctx, "completed")
}
