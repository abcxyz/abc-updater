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

package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"

	"github.com/abcxyz/pkg/logging"
)

const (
	manifestURLFormat     = "%s/manifest.json"
	appMetricsURLFormat   = "%s/%s/metrics.json"
	maxErrorResponseBytes = 2048
)

// Assert MetricsDB satisfies MetricsLookuper.
var _ MetricsLookuper = (*MetricsDB)(nil)

// ManifestResponse is the json file served to list all apps which have metrics.
type ManifestResponse struct {
	MetricsApps []string `json:"metricsApps"`
}

// AllowedMetricsResponse is the per-app metrics.json file which lists the metrics
// which can be recorded.
type AllowedMetricsResponse struct {
	Metrics []string `json:"metrics"`
}

type MetricsLookuper interface {
	Update(ctx context.Context, params *MetricsLoadParams) error
	GetAllowedMetrics(appID string) (*AppMetrics, error)
}

type MetricsDB struct {
	apps map[string]*AppMetrics
	mu   sync.RWMutex
}

func (db *MetricsDB) Update(ctx context.Context, params *MetricsLoadParams) error {
	manifest, err := getManifest(ctx, params)
	if err != nil {
		return fmt.Errorf("could not load manifest: %w", err)
	}

	newDefs := make(map[string]*AppMetrics, len(manifest.MetricsApps))

	// Could do these in parallel if performance is ever a concern.
	for _, app := range manifest.MetricsApps {
		def, err := getMetricsDefinition(ctx, app, params)
		if err != nil {
			logger := logging.FromContext(ctx)
			logger.WarnContext(ctx, "Error looking up metrics definitions for application in manifest. Will use cached definition if available.",
				"app_id", app,
				"cause", err.Error())
			// Technically a race as we could squash changes created by another update
			// but not a big deal if that happens.
			if metrics, err := db.GetAllowedMetrics(app); err != nil {
				logger.WarnContext(ctx, "No cached definition available for application metrics definition.",
					"app_id", app,
					"cause", err.Error())
			} else {
				newDefs[app] = metrics
			}
			continue
		} else {
			metricSet := make(map[string]interface{}, len(def.Metrics))
			for _, v := range def.Metrics {
				metricSet[v] = struct{}{}
			}
			newDefs[app] = &AppMetrics{
				AppID:   app,
				Allowed: metricSet,
			}
		}
	}
	db.mu.Lock()
	defer db.mu.Unlock()
	oldDefs := db.apps
	db.apps = newDefs
	diffApps(ctx, oldDefs, newDefs)
	return nil
}

// Log any changes in application lists. Individual metric names changes not
// currently logged. Logging is called in a goroutine to reduce blocking when
// holding write lock. Must only be called by a function that already
// holds lock.
func diffApps(ctx context.Context, oldDefs, newDefs map[string]*AppMetrics) {
	if oldDefs == nil {
		oldDefs = make(map[string]*AppMetrics)
	}
	logger := logging.FromContext(ctx)
	for k := range newDefs {
		k := k
		if _, ok := oldDefs[k]; !ok {
			go logger.InfoContext(ctx, "Loaded new application for metrics.", "app_id", k)
		}
	}
	for k := range oldDefs {
		k := k
		if _, ok := newDefs[k]; !ok {
			go logger.InfoContext(ctx, "Removed application for metrics.", "app_id", k)
		}
	}
}

// GetAllowedMetrics returns a struct containing metrics for a given appID.
// An error is returned if that appID is not defined in the backend for metrics.
func (db *MetricsDB) GetAllowedMetrics(appID string) (*AppMetrics, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()
	if db.apps == nil {
		return nil, fmt.Errorf("no metric definition found for app %s", appID)
	}
	v, ok := db.apps[appID]
	if !ok {
		return nil, fmt.Errorf("no metric definition found for app %s", appID)
	}
	return v, nil
}

// MetricsLoadParams are the parameters for looking up metrics information.
// TODO: load from config and parse/validate url on startup.
type MetricsLoadParams struct {
	ServerURL string
	Client    *http.Client
}

// getManifest fetches manifest definition from remote server.
func getManifest(ctx context.Context, params *MetricsLoadParams) (*ManifestResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf(manifestURLFormat, params.ServerURL), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create manifest request: %w", err)
	}
	resp, err := params.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make manifest request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, err := io.ReadAll(io.LimitReader(resp.Body, maxErrorResponseBytes))
		if err != nil {
			return nil, fmt.Errorf("unable to read response body")
		}
		// TODO: would be nice to alert on 4xx as it likely is not temporary failure.
		return nil, fmt.Errorf("not a 200 response: %s", string(b))
	}

	var m ManifestResponse
	if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
		return nil, fmt.Errorf("failed to decode response body: %w", err)
	}
	return &m, nil
}

// getMetricsDefinition fetches metrics definitions for a particular app from remote server.
func getMetricsDefinition(ctx context.Context, appID string, params *MetricsLoadParams) (*AllowedMetricsResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf(appMetricsURLFormat, params.ServerURL, appID), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create metric lookup request: %w", err)
	}
	resp, err := params.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make metric lookup request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, err := io.ReadAll(io.LimitReader(resp.Body, maxErrorResponseBytes))
		if err != nil {
			return nil, fmt.Errorf("unable to read response body")
		}
		// TODO: would be nice to alert on 4xx as it likely is not temporary failure.
		return nil, fmt.Errorf("not a 200 response: %s", string(b))
	}

	var m AllowedMetricsResponse
	if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
		return nil, fmt.Errorf("failed to decode response body: %w", err)
	}
	return &m, nil
}

type AppMetrics struct {
	AppID   string
	Allowed map[string]interface{}
}

// MetricAllowed is a helper for looking up a particular metric for an app.
func (m *AppMetrics) MetricAllowed(metric string) bool {
	if m != nil && m.Allowed != nil {
		_, ok := m.Allowed[metric]
		return ok
	}
	return false
}
