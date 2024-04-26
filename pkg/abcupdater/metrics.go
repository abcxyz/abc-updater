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

package abcupdater

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"time"

	"github.com/google/uuid"
	"github.com/sethvargo/go-envconfig"

	"github.com/abcxyz/abc-updater/pkg/abcupdater/localstore"
	"github.com/abcxyz/pkg/logging"
)

const (
	installIDFileName = "id.json"
)

var regExUUID = regexp.MustCompile("^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$")

type metricsConfig struct {
	ServerURL string `env:"ABC_UPDATER_METRICS_URL,default=https://abc-updater-metrics.tycho.joonix.net"`
}

// MetricsInfo are the parameters for sending metrics.
type MetricsInfo struct {
	// The ID of the application to check.
	AppID string

	// The version of the app to check for updates.
	// Should be of form vMAJOR[.MINOR[.PATCH[-PRERELEASE][+BUILD]]] (e.g., v1.0.1)
	Version string

	// An optional Lookuper to load envconfig structs. Will default to os environment variables.
	Lookuper envconfig.Lookuper

	Metrics map[string]int

	// Optional override for install id file location. Mostly intended for testing.
	// If empty uses default location.
	InstallIDFileOverride string
}

// InstallIDData defines the json file that defines installation id.
type InstallIDData struct {
	// Time ID was created, in UTC epoch seconds.
	IDCreatedTimestamp int64 `json:"idCreatedTimestamp"`
	// InstallID. Expected to be a hex 8-4-4-4-12 formatted v4 UUID.
	InstallID string `json:"installId"`
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

// Stricter than uuid.Parse() which isn't meant for validating strings according
// to documentation.
func validInstallId(id string) bool {
	return regExUUID.MatchString(id)
}

// SendMetricsSync sends information about application usage. Users can opt out
// by setting an env variable defined in opt_out.go.
// Accepts a context for cancellation.
func SendMetricsSync(ctx context.Context, info *MetricsInfo) error {
	lookuper := info.Lookuper
	if lookuper == nil {
		lookuper = envconfig.OsLookuper()
	}

	optOutSettings, err := loadOptOutSettings(ctx, lookuper, info.AppID)
	if err != nil {
		return fmt.Errorf("failed to load opt out settings: %w", err)
	}

	if optOutSettings.NoMetrics {
		return nil
	}

	generateNewID := true
	storedID, err := loadInstallID(info)
	if err == nil && storedID != nil {
		oneDayAgo := time.Now().Add(-24 * time.Hour)
		// Defensively check for case ID Created is in future.
		generateNewID = oneDayAgo.Unix() >= storedID.IDCreatedTimestamp ||
			time.Now().Unix() < storedID.IDCreatedTimestamp
	}
	var installID string
	if generateNewID {
		installUUID, err := uuid.NewRandom()
		if err != nil {
			return fmt.Errorf("could not generate id for metrics: %w")
		}
		installID = installUUID.String()
		err = storeInstallID(info, &InstallIDData{
			IDCreatedTimestamp: time.Now().Unix(),
			InstallID:          installID,
		})
		if err != nil {
			logging.FromContext(ctx).DebugContext(ctx, "error storing installID", "error", err.Error())
		}
	} else {
		installID = storedID.InstallID
	}

	var c metricsConfig
	if err := envconfig.ProcessWith(ctx, &envconfig.Config{
		Target:   &c,
		Lookuper: lookuper,
	}); err != nil {
		return fmt.Errorf("failed to process envconfig: %w", err)
	}

	// Use ParseRequestURI over Parse because Parse validation is more loose and will accept
	// things such as relative paths without a host.
	if _, err := url.ParseRequestURI(c.ServerURL); err != nil {
		return fmt.Errorf("failed to parse server url: %w", err)
	}

	client := &http.Client{}

	buf := bytes.Buffer{}
	if err := json.NewEncoder(&buf).Encode(SendMetricRequest{
		AppID:     info.AppID,
		Version:   info.Version,
		Metrics:   info.Metrics,
		InstallID: installID,
	}); err != nil {
		return fmt.Errorf("failed to marshal metric json: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf(c.ServerURL+"/sendMetrics"), &buf)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to make request: %w", err)
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

// A per-application install id is randomly generated. It gets rotated every
// 24 hours. This is to avoid a single client polluting metrics.
func loadInstallID(c *MetricsInfo) (*InstallIDData, error) {
	path := c.InstallIDFileOverride
	if path == "" {
		dir, err := localstore.DefaultDir(c.AppID)
		if err != nil {
			return nil, fmt.Errorf("could not calculate install ID path: %w", err)
		}
		path = filepath.Join(dir, installIDFileName)
	}
	var stored InstallIDData
	err := localstore.LoadJSONFile(path, &stored)
	if err != nil {
		return nil, fmt.Errorf("could not load install id: %w", err)
	}
	// Validate InstallID
	if !validInstallId(stored.InstallID) {
		return nil, fmt.Errorf("invalid install id")
	}

	return &stored, nil
}

func storeInstallID(c *MetricsInfo, data *InstallIDData) error {
	path := c.InstallIDFileOverride
	if path == "" {
		dir, err := localstore.DefaultDir(c.AppID)
		if err != nil {
			return fmt.Errorf("could not calculate install ID path: %w", err)
		}
		path = filepath.Join(dir, installIDFileName)
	}
	if err := localstore.StoreJSONFile(path, data); err != nil {
		return fmt.Errorf("could not store install id: %w", err)
	}
	return nil
}
