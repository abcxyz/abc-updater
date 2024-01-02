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
	"os"
	"strings"
)

type optOutSettings struct {
	MuteAllVersionUpdates bool
	MuteVersion           string
}

func loadOptOutSettings(appID string) *optOutSettings {
	settings := &optOutSettings{}
	if disabled := os.Getenv(muteUpdatesAllEnvVar(appID)); disabled != "" {
		settings.MuteAllVersionUpdates = true
	}

	if ignoreVersion := os.Getenv(muteUpdatesVersionEnvVar(appID)); ignoreVersion != "" {
		settings.MuteVersion = ignoreVersion
	}

	return settings
}

func abcAppEnvVarPrefix(appID string) string {
	return "ABC_UPDATER_APP_" + strings.ToUpper(appID)
}

func muteUpdatesAllEnvVar(appID string) string {
	return abcAppEnvVarPrefix(appID) + "_MUTE_UPDATES_ALL"
}

func muteUpdatesVersionEnvVar(appID string) string {
	return abcAppEnvVarPrefix(appID) + "_MUTE_UPDATES_VERSION"
}
