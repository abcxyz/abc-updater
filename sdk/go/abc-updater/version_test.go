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
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sethvargo/go-envconfig"
)

func TestCheckAppVersion(t *testing.T) {
	t.Parallel()

	sampleAppResponse, err := json.Marshal(AppResponse{
		AppID:          "sample_app_1",
		AppName:        "Sample App 1",
		GitHubURL:      "https://github.com/abcxyz/sample_app_1",
		CurrentVersion: "1.0.0",
	})
	if err != nil {
		t.Errorf("failed to encode json %v", err)
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.RequestURI, "sample_app_1/data.json") {
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprintln(w, http.StatusText(http.StatusNotFound))
			return
		}

		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, string(sampleAppResponse))
	}))

	lookuper := envconfig.MapLookuper(map[string]string{
		"ABC_UPDATER_URL": ts.URL,
	})

	t.Cleanup(func() {
		ts.Close()
	})

	cases := []struct {
		name    string
		appID   string
		version string
		want    string
	}{
		{
			name:    "outdated_version",
			appID:   "sample_app_1",
			version: "v0.0.1",
			want: fmt.Sprintf(outputFormat,
				"Sample App 1",
				"0.0.1",
				"1.0.0",
				"https://github.com/abcxyz/sample_app_1"),
		},
		{
			name:    "current_version",
			appID:   "sample_app_1",
			version: "v1.0.0",
			want:    "",
		},
		{
			name:    "invalid_app_id",
			appID:   "bad_app",
			version: "v1.0.0",
			want:    "",
		},
		{
			name:    "invalid_version",
			appID:   "sample_app_1",
			version: "vab1.0.0.12.2",
			want:    "",
		},
	}

	for _, tc := range cases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var b bytes.Buffer
			params := &CheckVersionParams{
				AppID:          tc.appID,
				Version:        tc.version,
				Writer:         &b,
				ConfigLookuper: lookuper,
			}

			CheckAppVersion(context.Background(), params)

			if got, want := b.String(), tc.want; got != want {
				t.Errorf("incorrect output got=%s, want=%s", got, want)
			}
		})
	}
}

func TestCheckAppVersion_OptOut(t *testing.T) {
	t.Parallel()

	sampleAppResponse, err := json.Marshal(AppResponse{
		AppID:          "sample_app_1",
		AppName:        "Sample App 1",
		GitHubURL:      "https://github.com/abcxyz/sample_app_1",
		CurrentVersion: "1.0.0",
	})
	if err != nil {
		t.Errorf("failed to encode json %v", err)
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.RequestURI, "sample_app_1/data.json") {
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprintln(w, http.StatusText(http.StatusNotFound))
			return
		}

		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, string(sampleAppResponse))
	}))

	lookuper := envconfig.MapLookuper(map[string]string{
		"ABC_UPDATER_URL": ts.URL,
	})

	t.Cleanup(func() {
		ts.Close()
	})

	cases := []struct {
		name           string
		appID          string
		version        string
		optOutSettings *OptOutSettings
		want           string
	}{
		{
			name:    "ignore_all",
			appID:   "sample_app_1",
			version: "v1.0.0",
			optOutSettings: &OptOutSettings{
				ignoreAllVersions: true,
			},
			want: "",
		},
		{
			name:    "ignore_match",
			appID:   "sample_app_1",
			version: "v1.0.0",
			optOutSettings: &OptOutSettings{
				IgnoreVersions: []string{"1.0.0"},
			},
			want: "",
		},
		{
			name:           "success_no_ignore_match",
			appID:          "sample_app_1",
			version:        "v0.0.1",
			optOutSettings: &OptOutSettings{},
			want: fmt.Sprintf(outputFormat,
				"Sample App 1",
				"0.0.1",
				"1.0.0",
				"https://github.com/abcxyz/sample_app_1"),
		},
	}

	for _, tc := range cases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var b bytes.Buffer
			params := &CheckVersionParams{
				AppID:          tc.appID,
				Version:        tc.version,
				Writer:         &b,
				ConfigLookuper: lookuper,
			}

			CheckAppVersion(context.Background(), params)

			if got, want := b.String(), tc.want; got != want {
				t.Errorf("incorrect output got=%s, want=%s", got, want)
			}
		})
	}
}
