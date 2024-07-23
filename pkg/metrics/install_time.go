// Copyright 2024 The Authors (see AUTHORS file)
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

package metrics

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/abcxyz/abc-updater/pkg/localstore"
)

// InstallInfo defines the json file that defines installation time.
type InstallInfo struct {
	// InstallTime. Minute-precision time of installation in UTC.
	InstallTime time.Time `json:"installTime"`
}

func loadInstallTime(appID, installTimeFileOverride string) (*InstallInfo, error) {
	path := installTimeFileOverride
	if path == "" {
		dir, err := localstore.DefaultDir(appID)
		if err != nil {
			return nil, fmt.Errorf("could not calculate install time path: %w", err)
		}
		path = filepath.Join(dir, installTimeFileName)
	}
	var stored InstallInfo

	if err := localstore.LoadJSONFile(path, &stored); err != nil {
		return nil, fmt.Errorf("could not load install time: %w", err)
	}

	stored.InstallTime = stored.InstallTime.UTC().Truncate(installTimeResolution)

	var zeroInfo InstallInfo

	if stored.InstallTime == zeroInfo.InstallTime {
		return nil, fmt.Errorf("invalid zero value for install time")
	}

	return &stored, nil
}

func storeInstallTime(appID, installTimeFileOverride string, data *InstallInfo) error {
	if installTimeFileOverride == "" {
		dir, err := localstore.DefaultDir(appID)
		if err != nil {
			return fmt.Errorf("could not calculate install time path: %w", err)
		}
		installTimeFileOverride = filepath.Join(dir, installTimeFileName)
	}
	if err := localstore.StoreJSONFile(installTimeFileOverride, data); err != nil {
		return fmt.Errorf("could not store install time: %w", err)
	}
	return nil
}
