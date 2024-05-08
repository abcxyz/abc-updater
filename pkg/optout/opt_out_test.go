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

package optout

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/sethvargo/go-envconfig"

	"github.com/abcxyz/pkg/testutil"
)

func TestLoadOptOutSettings(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		appID       string
		lookuperMap map[string]string
		want        *OptOutSettings
		wantErr     string
	}{
		{
			name:        "no_env_vars_set",
			appID:       "sample_app_1",
			lookuperMap: map[string]string{},
			want: &OptOutSettings{
				NoMetrics:         false,
				IgnoreAllVersions: false,
				IgnoreVersions:    nil,
			},
		},
		{
			name:  "set_no_metrics",
			appID: "sample_app_1",
			lookuperMap: map[string]string{
				"NO_METRICS": "true",
			},
			want: &OptOutSettings{
				NoMetrics:         true,
				IgnoreAllVersions: false,
				IgnoreVersions:    nil,
			},
		},
		{
			name:  "set_ignore_all",
			appID: "sample_app_1",
			lookuperMap: map[string]string{
				"SAMPLE_APP_1_IGNORE_VERSIONS": "all",
			},
			want: &OptOutSettings{
				NoMetrics:         false,
				IgnoreAllVersions: true,
				IgnoreVersions:    []string{"all"},
			},
		},
		{
			name:  "set_ignore_single_version",
			appID: "sample_app_1",
			lookuperMap: map[string]string{
				"SAMPLE_APP_1_IGNORE_VERSIONS": "1.0.0",
			},
			want: &OptOutSettings{
				IgnoreAllVersions: false,
				IgnoreVersions:    []string{"1.0.0"},
			},
		},
		{
			name:  "set_ignore_multiple_version",
			appID: "sample_app_1",
			lookuperMap: map[string]string{
				"SAMPLE_APP_1_IGNORE_VERSIONS": "<1.0.0,2.0.0,3.0.0",
			},
			want: &OptOutSettings{
				IgnoreAllVersions: false,
				IgnoreVersions:    []string{"<1.0.0", "2.0.0", "3.0.0"},
			},
		},
		{
			name:  "invalid_app_id",
			appID: "sample app 1",
			lookuperMap: map[string]string{
				"SAMPLE_APP_1_IGNORE_VERSIONS": "<1.0.0,2.0.0,3.0.0",
			},
			want: &OptOutSettings{
				IgnoreAllVersions: false,
				IgnoreVersions:    nil,
			},
		},
	}

	for _, tc := range cases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			config, err := LoadOptOutSettings(context.Background(), envconfig.MapLookuper(tc.lookuperMap), tc.appID)
			if diff := testutil.DiffErrString(err, tc.wantErr); diff != "" {
				t.Error(diff)
			}

			if diff := cmp.Diff(tc.want.IgnoreVersions, config.IgnoreVersions); diff != "" {
				t.Errorf("Config unexpected diff (-want,+got):\n%s", diff)
			}

			if got, want := config.IgnoreAllVersions, tc.want.IgnoreAllVersions; got != want {
				t.Errorf("incorrect IgnoreAllVersions got=%t, want=%t", got, want)
			}
		})
	}
}

func TestAllVersionUpdatesIgnored(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name           string
		optOutSettings *OptOutSettings
		want           bool
	}{
		{
			name: "no_error_no_ignore_all",
			optOutSettings: &OptOutSettings{
				IgnoreAllVersions: false,
				IgnoreVersions:    nil,
			},
			want: false,
		},
		{
			name: "no_error_ignore_all",
			optOutSettings: &OptOutSettings{
				IgnoreAllVersions: true,
				IgnoreVersions:    nil,
			},
			want: true,
		},
	}

	for _, tc := range cases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := tc.optOutSettings.IgnoreAllVersions

			if want := tc.want; got != want {
				t.Errorf("incorrect allVersionUpdatesIgnored got=%t, want=%t", got, want)
			}
		})
	}
}

func TestIsIgnored(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name           string
		version        string
		optOutSettings *OptOutSettings
		want           bool
		wantErr        string
	}{
		{
			name:    "nothing_ignored",
			version: "1.0.0",
			optOutSettings: &OptOutSettings{
				IgnoreAllVersions: false,
				IgnoreVersions:    nil,
			},
			want: false,
		},
		{
			name:    "all_ignored",
			version: "1.0.0",
			optOutSettings: &OptOutSettings{
				IgnoreAllVersions: true,
				IgnoreVersions:    nil,
			},
			want: true,
		},
		{
			name:    "version_no_match",
			version: "1.0.0",
			optOutSettings: &OptOutSettings{
				IgnoreAllVersions: false,
				IgnoreVersions:    []string{"1.0.1", "<1.0.0", ">1.0.0"},
			},
			want: false,
		},
		{
			name:    "version_match_last",
			version: "1.0.0",
			optOutSettings: &OptOutSettings{
				IgnoreAllVersions: false,
				IgnoreVersions:    []string{"1.0.1", "<1.0.0", ">1.0.0", "1.0.0"},
			},
			want: true,
		},
		{
			name:    "version_exact_match",
			version: "1.0.0",
			optOutSettings: &OptOutSettings{
				IgnoreAllVersions: false,
				IgnoreVersions:    []string{"1.0.0"},
			},
			want: true,
		},
		{
			name:    "version_constraint_lt",
			version: "1.0.0",
			optOutSettings: &OptOutSettings{
				IgnoreAllVersions: false,
				IgnoreVersions:    []string{"<1.0.1"},
			},
			want: true,
		},
		{
			name:    "version_constraint_gt",
			version: "1.0.0",
			optOutSettings: &OptOutSettings{
				IgnoreAllVersions: false,
				IgnoreVersions:    []string{">0.0.1"},
			},
			want: true,
		},
		{
			name:    "version_constraint_lte",
			version: "1.0.0",
			optOutSettings: &OptOutSettings{
				IgnoreAllVersions: false,
				IgnoreVersions:    []string{"<=1.0.0"},
			},
			want: true,
		},
		{
			name:    "version_constraint_gte",
			version: "1.0.0",
			optOutSettings: &OptOutSettings{
				IgnoreAllVersions: false,
				IgnoreVersions:    []string{">=1.0.0"},
			},
			want: true,
		},
		{
			name:    "version_prerelease",
			version: "1.1.0-alpha",
			optOutSettings: &OptOutSettings{
				IgnoreAllVersions: false,
				IgnoreVersions:    []string{"1.1.0-alpha"},
			},
			want: true,
		},
		{
			name:    "version_broken",
			version: "abcd",
			optOutSettings: &OptOutSettings{
				IgnoreAllVersions: false,
				IgnoreVersions:    []string{"1.1.0-alpha"},
			},
			want:    false,
			wantErr: "failed to parse version",
		},
		{
			name:    "constraint_broken",
			version: "1.0.0",
			optOutSettings: &OptOutSettings{
				IgnoreAllVersions: false,
				IgnoreVersions:    []string{"alsdkfas"},
			},
			want:    false,
			wantErr: "Malformed constraint",
		},
	}

	for _, tc := range cases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := tc.optOutSettings.IsIgnored(tc.version)

			if diff := testutil.DiffErrString(err, tc.wantErr); diff != "" {
				t.Error(diff)
			}

			if want := tc.want; got != want {
				t.Errorf("incorrect IsIgnored got=%t, want=%t", got, want)
			}
		})
	}
}
