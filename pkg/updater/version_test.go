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
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/sethvargo/go-envconfig"

	"github.com/abcxyz/pkg/testutil"
)

func TestCheckAppVersion(t *testing.T) {
	t.Parallel()

	testAppResponse := &AppResponse{
		AppID:          "sample_app_1",
		AppName:        "Sample App 1",
		AppRepoURL:     "https://github.com/abcxyz/sample_app_1",
		CurrentVersion: "1.0.0",
	}

	cases := []struct {
		name    string
		appID   string
		version string
		env     map[string]string
		want    string
		wantErr string
		cached  *LocalVersionData
	}{
		{
			name:    "outdated_version",
			appID:   "sample_app_1",
			version: "v0.0.1",
			want:    `Sample App 1 version 1.0.0 is available at [https://github.com/abcxyz/sample_app_1]. Use SAMPLE_APP_1_IGNORE_VERSIONS="1.0.0" (or "all") to ignore.`,
		},
		{
			name:    "current_version",
			appID:   "sample_app_1",
			version: "v1.0.0",
			want:    "",
		},
		{
			name:    "outdated_version_but_cached_check",
			appID:   "sample_app_1",
			version: "0.0.1",
			want:    "",
			cached: &LocalVersionData{
				LastCheckTimestamp: time.Now().Unix(),
				AppResponse:        testAppResponse,
			},
		},
		{
			name:    "outdated_version_cached_check_expired",
			appID:   "sample_app_1",
			version: "0.0.1",
			want:    `Sample App 1 version 1.0.0 is available at [https://github.com/abcxyz/sample_app_1]. Use SAMPLE_APP_1_IGNORE_VERSIONS="1.0.0" (or "all") to ignore.`,
			cached: &LocalVersionData{
				LastCheckTimestamp: time.Now().Add(-25 * time.Hour).Unix(),
				AppResponse:        testAppResponse,
			},
		},
		{
			name:    "invalid_app_id",
			appID:   "bad_app",
			version: "v1.0.0",
			want:    "",
			wantErr: "not found",
		},
		{
			name:    "invalid_version",
			appID:   "sample_app_1",
			version: "vab1.0.0.12.2",
			want:    "",
			wantErr: "failed to parse check version \"vab1.0.0.12.2\"",
		},
		{
			name:    "opt_out_ignore_all",
			appID:   "sample_app_1",
			version: "v0.1.0",
			env: map[string]string{
				ignoreVersionsEnvVar: "all",
			},
			want: "",
		},
		{
			name:    "opt_out_ignore_match",
			appID:   "sample_app_1",
			version: "v0.1.0",
			env: map[string]string{
				ignoreVersionsEnvVar: "1.0.0",
			},
			want: "",
		},
		{
			name:    "opt_out_no_match_not_ignored",
			appID:   "sample_app_1",
			version: "v0.0.1",
			env: map[string]string{
				ignoreVersionsEnvVar: "0.0.2",
			},
			want: `Sample App 1 version 1.0.0 is available at [https://github.com/abcxyz/sample_app_1]. Use SAMPLE_APP_1_IGNORE_VERSIONS="1.0.0" (or "all") to ignore.`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ts := httptest.NewServer(func() http.Handler {
				mux := http.NewServeMux()
				mux.HandleFunc("GET /sample_app_1/data.json", func(w http.ResponseWriter, r *http.Request) {
					if err := json.NewEncoder(w).Encode(testAppResponse); err != nil {
						t.Fatal(err)
					}
				})
				return mux
			}())
			t.Cleanup(ts.Close)

			cacheFile := filepath.Join(t.TempDir(), "data.json")

			params := &CheckVersionParams{
				AppID:   tc.appID,
				Version: tc.version,
				Lookuper: envconfig.MultiLookuper(
					envconfig.MapLookuper(map[string]string{"UPDATER_URL": ts.URL}),
					envconfig.MapLookuper(tc.env)),
				CacheFileOverride: cacheFile,
			}

			if tc.cached != nil {
				if err := setLocalCachedData(params, tc.cached); err != nil {
					t.Errorf("unexpected error setting up test cache file: %v", err)
				}
			}

			output, err := CheckAppVersion(t.Context(), params)
			if diff := testutil.DiffErrString(err, tc.wantErr); diff != "" {
				t.Error(diff)
			}

			if diff := cmp.Diff(output, tc.want); diff != "" {
				t.Errorf("output was not as expected (-got,+want): %s", diff)
			}
		})
	}
}

func TestCheckAppVersionAsync(t *testing.T) {
	t.Parallel()

	testAppResponse := &AppResponse{
		AppID:          "sample_app_1",
		AppName:        "Sample App 1",
		AppRepoURL:     "https://github.com/abcxyz/sample_app_1",
		CurrentVersion: "1.0.0",
	}

	cases := []struct {
		name    string
		timeout time.Duration
		want    string
		wantErr string
	}{
		{
			name: "happy_path_no_error",
			want: `Sample App 1 version 1.0.0 is available at [https://github.com/abcxyz/sample_app_1]. Use SAMPLE_APP_1_IGNORE_VERSIONS="1.0.0" (or "all") to ignore.`,
		},
		{
			name:    "happy_path_no_error_provided_timeout",
			timeout: 4 * time.Second,
			want:    `Sample App 1 version 1.0.0 is available at [https://github.com/abcxyz/sample_app_1]. Use SAMPLE_APP_1_IGNORE_VERSIONS="1.0.0" (or "all") to ignore.`,
		},
		{
			name:    "time_out_causes_error",
			timeout: 1 * time.Nanosecond,
			wantErr: "context deadline exceeded",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ts := httptest.NewServer(func() http.Handler {
				mux := http.NewServeMux()
				mux.HandleFunc("GET /sample_app_1/data.json", func(w http.ResponseWriter, r *http.Request) {
					// Add artificial latency to ensure our timeouts hit
					time.Sleep(50 * time.Nanosecond)

					if err := json.NewEncoder(w).Encode(testAppResponse); err != nil {
						t.Fatal(err)
					}
				})
				return mux
			}())
			t.Cleanup(ts.Close)

			ctx := t.Context()
			if tc.timeout > 0 {
				var done func()
				ctx, done = context.WithTimeout(ctx, tc.timeout)
				defer done()
			}

			cacheFile := filepath.Join(t.TempDir(), "data.json")

			params := &CheckVersionParams{
				AppID:   "sample_app_1",
				Version: "0.1.2",
				Lookuper: envconfig.MapLookuper(map[string]string{
					"UPDATER_URL": ts.URL,
				}),
				CacheFileOverride: cacheFile,
			}

			output, err := CheckAppVersion(ctx, params)
			if diff := testutil.DiffErrString(err, tc.wantErr); diff != "" {
				t.Error(diff)
			}

			if diff := cmp.Diff(output, tc.want); diff != "" {
				t.Errorf("output was not as expected (-got,+want): %s", diff)
			}
		})
	}
}

func TestIsIgnored(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		version string
		config  *versionConfig
		want    bool
		wantErr string
	}{
		{
			name:    "nothing_ignored",
			version: "1.0.0",
			config: &versionConfig{
				IgnoreVersions: nil,
			},
			want: false,
		},
		{
			name:    "all_ignored",
			version: "1.0.0",
			config: &versionConfig{
				IgnoreVersions: []string{"all"},
			},
			want: true,
		},
		{
			name:    "all_ignored_other_info",
			version: "1.0.0",
			config: &versionConfig{
				IgnoreVersions: []string{"1.0.1", "<1.0.0", "all", ">1.0.0"},
			},
			want: true,
		},
		{
			name:    "version_no_match",
			version: "1.0.0",
			config: &versionConfig{
				IgnoreVersions: []string{"1.0.1", "<1.0.0", ">1.0.0"},
			},
			want: false,
		},
		{
			name:    "version_match_last",
			version: "1.0.0",
			config: &versionConfig{
				IgnoreVersions: []string{"1.0.1", "<1.0.0", ">1.0.0", "1.0.0"},
			},
			want: true,
		},
		{
			name:    "version_exact_match",
			version: "1.0.0",
			config: &versionConfig{
				IgnoreVersions: []string{"1.0.0"},
			},
			want: true,
		},
		{
			name:    "version_constraint_lt",
			version: "1.0.0",
			config: &versionConfig{
				IgnoreVersions: []string{"<1.0.1"},
			},
			want: true,
		},
		{
			name:    "version_constraint_gt",
			version: "1.0.0",
			config: &versionConfig{
				IgnoreVersions: []string{">0.0.1"},
			},
			want: true,
		},
		{
			name:    "version_constraint_lte",
			version: "1.0.0",
			config: &versionConfig{
				IgnoreVersions: []string{"<=1.0.0"},
			},
			want: true,
		},
		{
			name:    "version_constraint_gte",
			version: "1.0.0",
			config: &versionConfig{
				IgnoreVersions: []string{">=1.0.0"},
			},
			want: true,
		},
		{
			name:    "version_prerelease",
			version: "1.1.0-alpha",
			config: &versionConfig{
				IgnoreVersions: []string{"1.1.0-alpha"},
			},
			want: true,
		},
		{
			name:    "version_broken",
			version: "abcd",
			config: &versionConfig{
				IgnoreVersions: []string{"1.1.0-alpha"},
			},
			want:    false,
			wantErr: "failed to parse version",
		},
		{
			name:    "constraint_broken",
			version: "1.0.0",
			config: &versionConfig{
				IgnoreVersions: []string{"alsdkfas"},
			},
			want:    false,
			wantErr: "Malformed constraint",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := tc.config.isIgnored(tc.version)

			if diff := testutil.DiffErrString(err, tc.wantErr); diff != "" {
				t.Error(diff)
			}

			if want := tc.want; got != want {
				t.Errorf("incorrect IsIgnored got=%t, want=%t", got, want)
			}
		})
	}
}

func TestIgnoreAll(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		config *versionConfig
		want   bool
	}{
		{
			name: "no_versions_not_ignored",
			config: &versionConfig{
				IgnoreVersions: nil,
			},
			want: false,
		},
		{
			name: "only_all_ignored",
			config: &versionConfig{
				IgnoreVersions: []string{"all"},
			},
			want: true,
		},
		{
			name: "concrete_list_not_ignored",
			config: &versionConfig{
				IgnoreVersions: []string{"1.0.0", "3.0.2"},
			},
			want: false,
		},
		{
			name: "concrete_list_with_all_ignored",
			config: &versionConfig{
				IgnoreVersions: []string{"1.0.0", "3.0.2", "all"},
			},
			want: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := tc.config.ignoreAll()

			if want := tc.want; got != want {
				t.Errorf("incorrect allVersionUpdatesIgnored got=%t, want=%t", got, want)
			}
		})
	}
}
