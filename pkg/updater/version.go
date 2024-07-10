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

package updater

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
	"text/template"
	"time"

	"github.com/hashicorp/go-version"
	"github.com/sethvargo/go-envconfig"

	"github.com/abcxyz/abc-updater/pkg/localstore"
)

// CheckVersionParams are the parameters for checking for application updates.
type CheckVersionParams struct {
	// The ID of the application to check.
	AppID string

	// The version of the app to check for updates.
	// Should be of form vMAJOR[.MINOR[.PATCH[-PRERELEASE][+BUILD]]] (e.g., v1.0.1)
	Version string

	// An optional Lookuper to load envconfig structs. Will default to os environment variables
	// prefixed with toUpper(AppID).
	Lookuper envconfig.Lookuper

	// Optional override for cached file location. Mostly intended for testing.
	// If empty uses default location.
	CacheFileOverride string
}

const ignoreVersionsEnvVar = "IGNORE_VERSIONS"

// AppResponse is the response object for an app version request.
// It contains information about the most recent version for a given app.
type AppResponse struct {
	AppID          string `json:"appId"`
	AppName        string `json:"appName"`
	AppRepoURL     string `json:"appRepoUrl"`
	CurrentVersion string `json:"currentVersion"`
}

type versionConfig struct {
	ServerURL      string   `env:"UPDATER_URL,default=https://abc-updater.tycho.joonix.net"`
	IgnoreVersions []string `env:"IGNORE_VERSIONS"`
}

func (c *versionConfig) ignoreAll() bool {
	for _, version := range c.IgnoreVersions {
		if strings.ToLower(version) == "all" {
			return true
		}
	}
	return false
}

// IsIgnored returns true if the version specified should be ignored.
func (c *versionConfig) isIgnored(checkVersion string) (bool, error) {
	v, err := version.NewVersion(checkVersion)
	if err != nil {
		return false, fmt.Errorf("failed to parse version: %w", err)
	}

	if c.ignoreAll() {
		return true, nil
	}

	var cumulativeErr error
	for _, ignoredVersion := range c.IgnoreVersions {
		c, err := version.NewConstraint(ignoredVersion)
		if err != nil {
			cumulativeErr = errors.Join(cumulativeErr, err)
			continue
		}

		// Constraint checks without pre-releases will only match versions without pre-release.
		// https://github.com/hashicorp/go-version/issues/130
		if c.Check(v) {
			return true, nil
		}
	}

	return false, cumulativeErr
}

// LocalVersionData defines the json file that caches version lookup data.
// Future versions may alert users of cached version info with every invocation.
type LocalVersionData struct {
	// Last time version information was checked, in UTC epoch seconds.
	LastCheckTimestamp int64 `json:"lastCheckTimestamp"`

	// Currently unused
	AppResponse *AppResponse
}

// versionUpdateDetails is used for filling outputTemplate.
type versionUpdateDetails struct {
	AppName       string
	AppRepoURL    string
	RemoteVersion string
	OptOutEnvVar  string
}

const (
	localVersionFileName  = "data.json"
	appDataURLFormat      = "%s/%s/data.json"
	outputTemplate        = `{{.AppName}} version {{.RemoteVersion}} is available at [{{.AppRepoURL}}]. Use {{.OptOutEnvVar}}="{{.RemoteVersion}}" (or "all") to ignore.`
	maxErrorResponseBytes = 2048
)

// CheckAppVersion checks if a newer version of an app is available. Any
// relevant update info will be returned as a string. It accepts a context for
// cancellation, or will time out after 5 seconds, whatever is sooner.
func CheckAppVersion(ctx context.Context, params *CheckVersionParams) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	lookuper := params.Lookuper
	if lookuper == nil {
		lookuper = envconfig.OsLookuper()
		lookuper = envconfig.PrefixLookuper(strings.ToUpper(params.AppID)+"_", lookuper)
	}

	var c versionConfig
	if err := envconfig.ProcessWith(ctx, &envconfig.Config{
		Target:   &c,
		Lookuper: lookuper,
	}); err != nil {
		return "", fmt.Errorf("failed to process envconfig: %w", err)
	}

	if c.ignoreAll() {
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
	req.Header.Set("User-Agent", "github.com/abcxyz/abc-updater")
	req.Header.Set("Accept", "application/json")

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
		AppResponse:        &result,
	})

	ignore, err := c.isIgnored(result.CurrentVersion)
	if err != nil {
		return "", fmt.Errorf("error checking optout: %w", err)
	}
	if ignore {
		return "", nil
	}

	remoteVersion, err := version.NewVersion(result.CurrentVersion)
	if err != nil {
		return "", fmt.Errorf("failed to parse current version %q: %w", params.Version, err)
	}

	if checkVersion.LessThan(remoteVersion) {
		output, err := updateVersionOutput(&versionUpdateDetails{
			AppName:       result.AppName,
			RemoteVersion: remoteVersion.String(),
			AppRepoURL:    result.AppRepoURL,
			OptOutEnvVar:  strings.ToUpper(result.AppID) + "_" + ignoreVersionsEnvVar,
		})
		if err != nil {
			return "", fmt.Errorf("failed to generate version check output: %w", err)
		}
		return output, nil
	}

	return "", nil
}

// CheckAppVersionAsync calls CheckAppVersion in a go routine. It returns a
// closure to be run after program logic which will block until a response is
// returned or provided context is canceled. The response will include the new
// version available (if any), and any errors that occur.
func CheckAppVersionAsync(ctx context.Context, params *CheckVersionParams) func() (string, error) {
	type result struct {
		val string
		err error
	}

	resultCh := make(chan *result, 1)
	go func() {
		defer close(resultCh)
		val, err := CheckAppVersion(ctx, params)
		resultCh <- &result{
			val: val,
			err: err,
		}
	}()

	return func() (string, error) {
		result := <-resultCh
		return result.val, result.err
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
