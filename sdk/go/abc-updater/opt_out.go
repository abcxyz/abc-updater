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
	"strings"

	"github.com/Masterminds/semver/v3"
	"github.com/sethvargo/go-envconfig"
)

type optOutSettings struct {
	IgnoreVersions    []string `env:"IGNORE_VERSIONS"`
	ignoreAllVersions bool
	errorLoading      bool
}

// loadOptOutSettings will return an optOutSettings struct populated based on the lookuper provided.
func loadOptOutSettings(ctx context.Context, lookuper envconfig.Lookuper, appID string) *optOutSettings {
	l := envconfig.PrefixLookuper(envVarPrefix(appID), lookuper)
	var c optOutSettings
	if err := envconfig.ProcessWith(ctx, &c, l); err != nil {
		c.errorLoading = true
		return &c
	}

	for _, version := range c.IgnoreVersions {
		if strings.ToLower(version) == "all" {
			c.ignoreAllVersions = true
		}
	}

	return &c
}

func envVarPrefix(appID string) string {
	return strings.ToUpper(appID) + "_"
}

func (o *optOutSettings) allVersionUpdatesIgnored() bool {
	return o.errorLoading || o.ignoreAllVersions
}

func (o *optOutSettings) isIgnored(version string) bool {
	if o.allVersionUpdatesIgnored() {
		return true
	}

	for _, ignoredVersion := range o.IgnoreVersions {
		c, err := semver.NewConstraint(ignoredVersion)
		if err != nil {
			continue
		}

		v, err := semver.NewVersion(version)
		if err != nil {
			continue
		}

		satisfies := c.Check(v)
		if satisfies {
			return true
		}
	}

	return false
}
