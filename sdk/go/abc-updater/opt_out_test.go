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
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/sethvargo/go-envconfig"
)

func TestLoadOptOutSettings(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		appID       string
		lookuperMap map[string]string
		want        *optOutSettings
	}{
		{
			name:        "no_env_vars_set",
			appID:       "sample_app_1",
			lookuperMap: map[string]string{},
			want: &optOutSettings{
				ignoreAllVersions: false,
				IgnoreVersions:    nil,
				errorLoading:      false,
			},
		},
		{
			name:  "set_ignore_all",
			appID: "sample_app_1",
			lookuperMap: map[string]string{
				"SAMPLE_APP_1_IGNORE_VERSIONS": "all",
			},
			want: &optOutSettings{
				ignoreAllVersions: true,
				IgnoreVersions:    []string{"all"},
				errorLoading:      false,
			},
		},
		{
			name:  "set_ignore_single_version",
			appID: "sample_app_1",
			lookuperMap: map[string]string{
				"SAMPLE_APP_1_IGNORE_VERSIONS": "1.0.0",
			},
			want: &optOutSettings{
				ignoreAllVersions: false,
				IgnoreVersions:    []string{"1.0.0"},
				errorLoading:      false,
			},
		},
		{
			name:  "set_ignore_multiple_version",
			appID: "sample_app_1",
			lookuperMap: map[string]string{
				"SAMPLE_APP_1_IGNORE_VERSIONS": "<1.0.0,2.0.0,3.0.0",
			},
			want: &optOutSettings{
				ignoreAllVersions: false,
				IgnoreVersions:    []string{"<1.0.0", "2.0.0", "3.0.0"},
				errorLoading:      false,
			},
		},
	}

	for _, tc := range cases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			config := loadOptOutSettings(context.Background(), envconfig.MapLookuper(tc.lookuperMap), tc.appID)

			if diff := cmp.Diff(tc.want.IgnoreVersions, config.IgnoreVersions); diff != "" {
				t.Errorf("Config unexpected diff (-want,+got):\n%s", diff)
			}

			if got, want := config.ignoreAllVersions, tc.want.ignoreAllVersions; got != want {
				t.Errorf("incorrect ignoreAllVersions got=%t, want=%t", got, want)
			}

			if got, want := config.errorLoading, tc.want.errorLoading; got != want {
				t.Errorf("incorrect errorLoading got=%t, want=%t", got, want)
			}
		})
	}
}

func TestAllVersionUpdatesIgnored(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name           string
		appID          string
		optOutSettings *optOutSettings
		want           bool
	}{
		{
			name:  "no_error_no_ignore_all",
			appID: "sample_app_1",
			optOutSettings: &optOutSettings{
				ignoreAllVersions: false,
				IgnoreVersions:    nil,
				errorLoading:      false,
			},
			want: false,
		},
		{
			name:  "error_no_ignore_all",
			appID: "sample_app_1",
			optOutSettings: &optOutSettings{
				ignoreAllVersions: false,
				IgnoreVersions:    nil,
				errorLoading:      true,
			},
			want: true,
		},
		{
			name:  "no_error_ignore_all",
			appID: "sample_app_1",
			optOutSettings: &optOutSettings{
				ignoreAllVersions: true,
				IgnoreVersions:    nil,
				errorLoading:      false,
			},
			want: true,
		},
		{
			name:  "error_and_ignore_all",
			appID: "sample_app_1",
			optOutSettings: &optOutSettings{
				ignoreAllVersions: true,
				IgnoreVersions:    nil,
				errorLoading:      true,
			},
			want: true,
		},
	}

	for _, tc := range cases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := tc.optOutSettings.allVersionUpdatesIgnored()

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
		appID          string
		version        string
		optOutSettings *optOutSettings
		want           bool
	}{
		{
			name:    "nothing_ignored",
			appID:   "sample_app_1",
			version: "1.0.0",
			optOutSettings: &optOutSettings{
				ignoreAllVersions: false,
				IgnoreVersions:    nil,
				errorLoading:      false,
			},
			want: false,
		},
		{
			name:    "all_ignored",
			appID:   "sample_app_1",
			version: "1.0.0",
			optOutSettings: &optOutSettings{
				ignoreAllVersions: true,
				IgnoreVersions:    nil,
				errorLoading:      false,
			},
			want: true,
		},
		{
			name:    "version_no_match",
			appID:   "sample_app_1",
			version: "1.0.0",
			optOutSettings: &optOutSettings{
				ignoreAllVersions: false,
				IgnoreVersions:    []string{"1.0.1", "<1.0.0", ">1.0.0"},
				errorLoading:      false,
			},
			want: false,
		},
		{
			name:    "version_match_last",
			appID:   "sample_app_1",
			version: "1.0.0",
			optOutSettings: &optOutSettings{
				ignoreAllVersions: false,
				IgnoreVersions:    []string{"1.0.1", "<1.0.0", ">1.0.0", "1.0.0"},
				errorLoading:      false,
			},
			want: true,
		},
		{
			name:    "version_exact_match",
			appID:   "sample_app_1",
			version: "1.0.0",
			optOutSettings: &optOutSettings{
				ignoreAllVersions: false,
				IgnoreVersions:    []string{"1.0.0"},
				errorLoading:      false,
			},
			want: true,
		},
		{
			name:    "version_constraint_lt",
			appID:   "sample_app_1",
			version: "1.0.0",
			optOutSettings: &optOutSettings{
				ignoreAllVersions: false,
				IgnoreVersions:    []string{"<1.0.1"},
				errorLoading:      false,
			},
			want: true,
		},
		{
			name:    "version_constraint_gt",
			appID:   "sample_app_1",
			version: "1.0.0",
			optOutSettings: &optOutSettings{
				ignoreAllVersions: false,
				IgnoreVersions:    []string{">0.0.1"},
				errorLoading:      false,
			},
			want: true,
		},
		{
			name:    "version_constraint_lte",
			appID:   "sample_app_1",
			version: "1.0.0",
			optOutSettings: &optOutSettings{
				ignoreAllVersions: false,
				IgnoreVersions:    []string{"<=1.0.0"},
				errorLoading:      false,
			},
			want: true,
		},
		{
			name:    "version_constraint_gte",
			appID:   "sample_app_1",
			version: "1.0.0",
			optOutSettings: &optOutSettings{
				ignoreAllVersions: false,
				IgnoreVersions:    []string{">=1.0.0"},
				errorLoading:      false,
			},
			want: true,
		},
		{
			name:    "version_prerelease",
			appID:   "sample_app_1",
			version: "1.1.0-alpha",
			optOutSettings: &optOutSettings{
				ignoreAllVersions: false,
				IgnoreVersions:    []string{"1.1.0-alpha"},
				errorLoading:      false,
			},
			want: true,
		},
	}

	for _, tc := range cases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := tc.optOutSettings.isIgnored(tc.version)

			if want := tc.want; got != want {
				t.Errorf("incorrect isIgnored got=%t, want=%t", got, want)
			}
		})
	}
}
