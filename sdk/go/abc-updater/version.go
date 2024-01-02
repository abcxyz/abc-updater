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

package abcupdater

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/sethvargo/go-envconfig"
	"golang.org/x/mod/semver"
)

// CheckVersionParams are the parameters for checking for application updates.
type CheckVersionParams struct {
	// The ID of the application to check.
	AppID string

	// The version of the app to check for updates.
	// Should be of form vMAJOR[.MINOR[.PATCH[-PRERELEASE][+BUILD]]] (e.g., v1.0.1)
	Version string

	// The writer where the update info will be written to.
	Writer io.Writer

	// An optional configLookuper to supply config values. Will default to os environment variables.
	ConfigLookuper envconfig.Lookuper

	// Optional optOutSettings to override settings set by environment variables.
	OptOutSettings *optOutSettings
}

// AppResponse is the response object for an app version request.
// It contains information about the most recent version for a given app.
type AppResponse struct {
	AppID          string `json:"app_id"`
	AppName        string `json:"app_name"`
	GitHubURL      string `json:"github_url"`
	CurrentVersion string `json:"current_version"`
}

type config struct {
	ServerURL      string        `env:"ABC_UPDATER_URL,default=https://abc-updater-autopush.tycho.joonix.net"`
	RequestTimeout time.Duration `env:"ABC_UPDATER_TIMEOUT,default=2m"`
}

const (
	appDataURLFormat      = "%s/%s/data.json"
	outputFormat          = "A new version of %s is available! Your current version is %s. Version %s is available at %s.\n"
	maxErrorResponseBytes = 2048
)

// CheckAppVersion checks if a newer version of an app is available. Relevant update info will be
// written to the writer provided if applicable.
func CheckAppVersion(ctx context.Context, params *CheckVersionParams) {
	optOutSettings := params.OptOutSettings
	if optOutSettings == nil {
		optOutSettings = initOptOutSettings(params.AppID)
	}
	if optOutSettings.MuteAllVersionUpdates {
		return
	}

	lookuper := params.ConfigLookuper
	if lookuper == nil {
		lookuper = envconfig.OsLookuper()
	}

	var c config
	if err := envconfig.ProcessWith(ctx, &c, lookuper); err != nil {
		return
	}

	// Use ParseRequestURI over Parse because Parse validation is more loose and will accept
	// things such as relative paths without a host.
	if _, err := url.ParseRequestURI(c.ServerURL); err != nil {
		return
	}

	if !semver.IsValid(params.Version) {
		return
	}

	client := &http.Client{
		Timeout: c.RequestTimeout,
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf(appDataURLFormat, c.ServerURL, params.AppID), nil)
	if err != nil {
		return
	}

	resp, err := client.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return
	}

	var result AppResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return
	}

	// semver requires v prefix. Current version data is stored without prefix so prepend v.
	if semver.Compare(params.Version, "v"+result.CurrentVersion) < 0 {
		if optOutSettings.shouldIgnoreVersion(result.CurrentVersion) {
			return
		}
		outStr := fmt.Sprintf(outputFormat, result.AppName, params.Version, result.CurrentVersion, result.GitHubURL)
		if _, err := params.Writer.Write([]byte(outStr)); err != nil {
			return
		}
	}
}
