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
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"golang.org/x/mod/semver"
)

type AppResponse struct {
	AppID          string `json:"app_id"`
	AppName        string `json:"app_name"`
	GithubURL      string `json:"github_url"`
	CurrentVersion string `json:"current_version"`
}

const (
	// Can be overwritten via ABC_UPDATER_URL env var.
	abcUpdaterURLDefault  = "https://abc-updater-autopush.tycho.joonix.net"
	requestTimeoutSeconds = 2
)

// CheckAppVersion checks if a newer version of an app is available. Relevant update info will be
// written to the writer provided if applicable.
func CheckAppVersion(appID, version string, w io.Writer) error {
	if !semver.IsValid(version) {
		return errors.New("version is not a valid semantic version string")
	}

	client := http.Client{
		Timeout: requestTimeoutSeconds * time.Second,
	}

	requestURL := abcUpdaterURLDefault
	if overrideURL := os.Getenv("ABC_UPDATER_URL"); overrideURL != "" {
		requestURL = overrideURL
	}

	resp, err := client.Get(fmt.Sprintf("%s/%s/data.json", requestURL, appID))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return errors.New("unable to retrieve data for requested app")
	}

	result := &AppResponse{}
	if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
		return err
	}

	if semver.Compare(version, "v"+result.CurrentVersion) < 0 {
		w.Write([]byte(fmt.Sprintf("A new version of %s is available! Your current version is %s. Version %s is available at %s.\n", result.AppName, version, result.CurrentVersion, result.GithubURL)))
	}

	return nil
}
