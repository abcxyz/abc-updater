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
	Version string

	// The writer where the update info will be written to.
	Writer io.Writer

	// Optional configuration options, will be set to defaults if not specified.
	// They will be overwritten by any environment variables present.
	Config *ABCUpdaterConfig
}

// AppResponse is the response returned with app data.
type AppResponse struct {
	AppID          string `json:"app_id"`
	AppName        string `json:"app_name"`
	GitHubURL      string `json:"github_url"`
	CurrentVersion string `json:"current_version"`
}

type ABCUpdaterConfig struct {
	ServerURL      string        `env:"ABC_UPDATER_URL,default=https://abc-updater-autopush.tycho.joonix.net"`
	RequestTimeout time.Duration `env:"ABC_UPDATER_TIMEOUT,default=2m"`
}

const appDataURLFormat = "%s/%s/data.json"

// CheckAppVersion checks if a newer version of an app is available. Relevant update info will be
// written to the writer provided if applicable.
func CheckAppVersion(ctx context.Context, params *CheckVersionParams) error {
	c := params.Config
	if err := envconfig.Process(ctx, c); err != nil {
		return fmt.Errorf("failed to processes env vars: %w", err)
	}

	if _, err := url.ParseRequestURI(c.ServerURL); err != nil {
		return fmt.Errorf("url from env var is not valid")
	}

	if !semver.IsValid(params.Version) {
		return fmt.Errorf("version is not a valid semantic version string")
	}

	client := &http.Client{
		Timeout: c.RequestTimeout,
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf(appDataURLFormat, c.ServerURL, params.AppID), nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("unable to read response body")
		}

		return fmt.Errorf(string(b))
	}

	result := &AppResponse{}
	if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	if semver.Compare(params.Version, "v"+result.CurrentVersion) < 0 {
		if _, err := params.Writer.Write([]byte(fmt.Sprintf("A new version of %s is available! Your current version is %s. Version %s is available at %s.\n", result.AppName, params.Version, result.CurrentVersion, result.GitHubURL))); err != nil {
			return fmt.Errorf("failed to write output: %w", err)
		}
	}

	return nil
}
