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
	"errors"
	"fmt"
	"strings"

	"github.com/hashicorp/go-version"
	"github.com/sethvargo/go-envconfig"
)

type OptOutSettings struct {
	NoMetrics         bool     `env:"NO_METRICS"`
	IgnoreVersions    []string `env:"IGNORE_VERSIONS"`
	IgnoreAllVersions bool
}

// LoadOptOutSettings will return an OptOutSettings struct populated based on the lookuper provided.
func LoadOptOutSettings(ctx context.Context, lookuper envconfig.Lookuper, appID string) (*OptOutSettings, error) {
	l := envconfig.PrefixLookuper(envVarPrefix(appID), lookuper)
	var c OptOutSettings
	if err := envconfig.ProcessWith(ctx, &envconfig.Config{
		Target:   &c,
		Lookuper: l,
	}); err != nil {
		// if we fail loading envconfig, default to ignore updates
		c.IgnoreAllVersions = true
		return &c, fmt.Errorf("failed to process envconfig: %w", err)
	}

	for _, version := range c.IgnoreVersions {
		if strings.ToLower(version) == "all" {
			c.IgnoreAllVersions = true
		}
	}

	return &c, nil
}

func envVarPrefix(appID string) string {
	return strings.ToUpper(appID) + "_"
}

func IgnoreVersionsEnvVar(appID string) string {
	return envVarPrefix(appID) + "IGNORE_VERSIONS"
}

// IsIgnored returns true if the version specified should be ignored.
func (o *OptOutSettings) IsIgnored(checkVersion string) (bool, error) {
	if o.IgnoreAllVersions {
		return true, nil
	}

	v, err := version.NewVersion(checkVersion)
	if err != nil {
		return false, fmt.Errorf("failed to parse version: %w", err)
	}

	var cumulativeErr error
	for _, ignoredVersion := range o.IgnoreVersions {
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
