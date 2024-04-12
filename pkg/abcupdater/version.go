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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"text/template"
	"time"

	"github.com/hashicorp/go-version"
	"github.com/sethvargo/go-envconfig"

	"github.com/abcxyz/abc-updater/pkg/abcupdater/localstore"
	"github.com/abcxyz/pkg/logging"
)

// CheckVersionParams are the parameters for checking for application updates.
type CheckVersionParams struct {
	// The ID of the application to check.
	AppID string

	// The version of the app to check for updates.
	// Should be of form vMAJOR[.MINOR[.PATCH[-PRERELEASE][+BUILD]]] (e.g., v1.0.1)
	Version string

	// An optional Lookuper to load envconfig structs. Will default to os environment variables.
	Lookuper envconfig.Lookuper

	// Optional override for cached file location. Mostly intended for testing.
	// If empty uses default location.
	CacheFileOverride string
}

// AppResponse is the response object for an app version request.
// It contains information about the most recent version for a given app.
type AppResponse struct {
	AppID          string `json:"appId"`
	AppName        string `json:"appName"`
	AppRepoURL     string `json:"appRepoUrl"`
	CurrentVersion string `json:"currentVersion"`
}

type config struct {
	ServerURL string `env:"ABC_UPDATER_URL,default=https://abc-updater.tycho.joonix.net"`
}

// LocalVersionData defines the json file that caches version lookup data.
// Future versions may alert users of cached version info with every invocation.
type LocalVersionData struct {
	// Last time version information was checked, in UTC epoch seconds.
	LastCheckTimestamp int64 `json:"lastCheckTimestamp"`
	// Currently unused
	AppResponse
}

// versionUpdateDetails is used for filling outputTemplate.
type versionUpdateDetails struct {
	AppName        string
	AppRepoURL     string
	CheckVersion   string
	CurrentVersion string
	OptOutEnvVar   string
}

const (
	localVersionFileName = "data.json"
	appDataURLFormat     = "%s/%s/data.json"
	outputTemplate       = `A new version of {{.AppName}} is available! Your current version is {{.CheckVersion}}. Version {{.CurrentVersion}} is available at {{.AppRepoURL}}.

To disable notifications for this new version, set {{.OptOutEnvVar}}="{{.CurrentVersion}}". To disable all version notifications, set {{.OptOutEnvVar}}="all".
`
	maxErrorResponseBytes = 2048
)

// CheckAppVersion calls CheckAppVersionSync in a go routine. It returns a closure
// to be run after program logic which will block until a response is returned
// or provided context is canceled. If no provided deadline in context, defaults to 2 seconds.
// If there is an update, out() will be called during the
// returned closure.
//
// If no update is available: out() will not be called.
// If there is an error: out() will not be called, message will be logged as WARN.
// If the context is canceled: out() is not called.
// If processing config fails: an error will be returned synchronously.
// Example out(): `func(s string) {fmt.Fprintln(os.Stderr, s)}`.
func CheckAppVersion(ctx context.Context, params *CheckVersionParams, out func(string)) (func(), error) {
	cancel := func() {}
	if _, ok := ctx.Deadline(); !ok {
		ctx, cancel = context.WithTimeout(ctx, time.Second*2) //nolint:lostcancel
	}

	lookuper := params.Lookuper
	if lookuper == nil {
		lookuper = envconfig.OsLookuper()
	}
	var c config
	if err := envconfig.ProcessWith(ctx, &envconfig.Config{
		Target:   &c,
		Lookuper: lookuper,
	}); err != nil {
		// This leaks context. OK since only runs once, and timeout is short.
		return nil, fmt.Errorf("failed to process envconfig: %w", err) //nolint:lostcancel
	}
	return asyncFunctionCall(ctx, func() (string, error) {
		defer cancel()
		return CheckAppVersionSync(ctx, params)
	}, out), nil
}

// CheckAppVersionSync checks if a newer version of an app is available. Any relevant update info will be
// returned as a string. Accepts a context for cancellation.
func CheckAppVersionSync(ctx context.Context, params *CheckVersionParams) (string, error) {
	lookuper := params.Lookuper
	if lookuper == nil {
		lookuper = envconfig.OsLookuper()
	}

	optOutSettings, err := loadOptOutSettings(ctx, lookuper, params.AppID)
	if err != nil {
		return "", fmt.Errorf("failed to load opt out settings: %w", err)
	}

	if optOutSettings.allVersionUpdatesIgnored() {
		return "", nil
	}

	fetchNewData := true
	cachedData, err := loadLocalCachedData(params)
	if err == nil && cachedData != nil {
		oneDayAgo := time.Now().Add(-24 * time.Hour)
		fetchNewData = oneDayAgo.Unix() >= cachedData.LastCheckTimestamp
	}
	if !fetchNewData {
		return "", nil
	}

	var c config
	if err := envconfig.ProcessWith(ctx, &envconfig.Config{
		Target:   &c,
		Lookuper: lookuper,
	}); err != nil {
		return "", fmt.Errorf("failed to process envconfig: %w", err)
	}

	// Use ParseRequestURI over Parse because Parse validation is more loose and will accept
	// things such as relative paths without a host.
	if _, err := url.ParseRequestURI(c.ServerURL); err != nil {
		return "", fmt.Errorf("failed to parse server url: %w", err)
	}

	checkVersion, err := version.NewVersion(params.Version)
	if err != nil {
		return "", fmt.Errorf("failed to parse check version %q: %w", params.Version, err)
	}

	client := &http.Client{}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf(appDataURLFormat, c.ServerURL, params.AppID), nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, err := io.ReadAll(io.LimitReader(resp.Body, maxErrorResponseBytes))
		if err != nil {
			return "", fmt.Errorf("unable to read response body")
		}

		return "", fmt.Errorf("not a 200 response: %s", string(b))
	}

	var result AppResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to decode response body: %w", err)
	}

	_ = setLocalCachedData(params, &LocalVersionData{
		LastCheckTimestamp: time.Now().Unix(),
		AppResponse:        result,
	})

	ignore, err := optOutSettings.isIgnored(result.CurrentVersion)
	if err != nil {
		return "", err
	}
	if ignore {
		return "", nil
	}

	currentVersion, err := version.NewVersion(result.CurrentVersion)
	if err != nil {
		return "", fmt.Errorf("failed to parse current version %q: %w", params.Version, err)
	}

	if checkVersion.LessThan(currentVersion) {
		output, err := updateVersionOutput(&versionUpdateDetails{
			AppName:        result.AppName,
			CheckVersion:   checkVersion.String(),
			CurrentVersion: currentVersion.String(),
			AppRepoURL:     result.AppRepoURL,
			OptOutEnvVar:   ignoreVersionsEnvVar(result.AppID),
		})
		if err != nil {
			return "", fmt.Errorf("failed to generate version check output: %w", err)
		}
		return output, nil
	}

	return "", nil
}

// asyncFunctionCall handles the async part of CheckAppVersion, but accepts
// a function other than CheckAppVersionSync for testing.
func asyncFunctionCall(ctx context.Context, funcToCall func() (string, error), outFunc func(string)) func() {
	updatesCh := make(chan string, 1)

	go func() {
		defer close(updatesCh)
		message, err := funcToCall()
		if err != nil {
			logging.FromContext(ctx).WarnContext(ctx, "failed to check for new versions",
				"error", err)
		}
		updatesCh <- message
	}()

	return func() {
		select {
		case <-ctx.Done():
			// Context was cancelled
		case msg := <-updatesCh:
			if len(msg) > 0 {
				outFunc(msg)
			}
		}
	}
}

func updateVersionOutput(updateDetails *versionUpdateDetails) (string, error) {
	tmpl, err := template.New("version_update_template").Parse(outputTemplate)
	if err != nil {
		return "", fmt.Errorf("failed to create output text template: %w", err)
	}

	var b bytes.Buffer
	err = tmpl.Execute(&b, updateDetails)
	if err != nil {
		return "", fmt.Errorf("failed to execute template: %w", err)
	}

	return b.String(), nil
}

func loadLocalCachedData(c *CheckVersionParams) (*LocalVersionData, error) {
	path := c.CacheFileOverride
	if path == "" {
		dir, err := localstore.DefaultDir(c.AppID)
		if err != nil {
			return nil, fmt.Errorf("could not calculate cache path: %w", err)
		}
		path = filepath.Join(dir, localVersionFileName)
	}
	var cached LocalVersionData
	err := localstore.LoadJSONFile(path, &cached)
	if err != nil {
		return nil, fmt.Errorf("could not load cached data: %w", err)
	}
	return &cached, nil
}

func setLocalCachedData(c *CheckVersionParams, data *LocalVersionData) error {
	path := c.CacheFileOverride
	if path == "" {
		dir, err := localstore.DefaultDir(c.AppID)
		if err != nil {
			return fmt.Errorf("could not calculate cache path: %w", err)
		}
		path = filepath.Join(dir, localVersionFileName)
	}
	if err := localstore.StoreJSONFile(path, data); err != nil {
		return fmt.Errorf("could not cache version: %w", err)
	}
	return nil
}
