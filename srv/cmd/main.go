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
	"github.com/abcxyz/abc-updater/srv/pkg"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/abcxyz/pkg/logging"
	"github.com/abcxyz/pkg/renderer"
	"github.com/abcxyz/pkg/serving"
)

const defaultPort = "8080"

var port = flag.String("port", defaultPort, "Specifies server port to listen on.")

func handleHello(h *renderer.Renderer) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logger := logging.FromContext(r.Context())
		logger.InfoContext(r.Context(), "handling request")
		h.RenderJSON(w, http.StatusOK, map[string]string{"message": "hello world"})
	})
}

// TODO: figure out how to make modules so this doesn't get re-defined multiple places
type SendMetricRequest struct {
	// The ID of the application to check.
	AppID string `json:"appID"`

	// The version of the app to check for updates.
	// Should be of form vMAJOR[.MINOR[.PATCH[-PRERELEASE][+BUILD]]] (e.g., v1.0.1)
	AppVersion string `json:"appVersion"`

	// TODO: this is a bit different from design doc, is it ok?
	Metrics map[string]int `json:"metrics"`

	// InstallID. Expected to be a hex 8-4-4-4-12 formatted v4 UUID.
	InstallID string `json:"installId"`
}

func handleMetric(h *renderer.Renderer) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logger := logging.FromContext(r.Context())
		metric_logger := logger.WithGroup("metric")
		logger.InfoContext(r.Context(), "handling request")

		metrics, err := pkg.DecodeRequest[SendMetricRequest](r.Context(), w, r, h)
		if err != nil {
			// Error response already handled by pkg.DecodeRequest.
			return
		}

		// TODO: Validate metrics & remove unknown

		for name, count := range metrics.Metrics {
			// TODO: does this leak sensitive information? Is default logger preferred.
			metric_logger.InfoContext(r.Context(), "metric received", "appID", metrics.AppID, "appVersion", metrics.AppVersion, "installId", metrics.InstallID, "name", name, "count", count)
		}

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

	mux := http.NewServeMux()
	mux.Handle("/", handleHello(h))
	mux.Handle("POST /sendMetrics", handleMetric(h))

	httpServer := &http.Server{
		Addr:              *port,
		Handler:           mux,
		ReadHeaderTimeout: 2 * time.Second,
	}

	logger.InfoContext(ctx, "starting server", "port", *port)
	server, err := serving.New(*port)
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
	logger := logging.FromContext(ctx)

	flag.Parse()
	if err := realMain(logging.WithLogger(ctx, logger)); err != nil {
		done()
		logger.ErrorContext(ctx, err.Error())
		os.Exit(1)
	}
	logger.InfoContext(ctx, "completed")
}
