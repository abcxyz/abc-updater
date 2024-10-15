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

	"github.com/abcxyz/abc-updater/pkg/localstore"
)

// InstallInfo defines the json file that defines installation time.
type InstallInfo struct {
	// InstallTime is a minute-precision time of installation in UTC.
	InstallTime string `json:"installTime"`
}

func loadInstallInfo(pth string) (*InstallInfo, error) {
	var stored InstallInfo
	if err := localstore.LoadJSONFile(pth, &stored); err != nil {
		return nil, fmt.Errorf("failed to load install info: %w", err)
	}

	if stored.InstallTime == "" {
		return nil, fmt.Errorf("invalid zero value for install info")
	}

	return &stored, nil
}

func storeInstallInfo(pth string, data *InstallInfo) error {
	if err := localstore.StoreJSONFile(pth, data); err != nil {
		return fmt.Errorf("failed to store install info: %w", err)
	}
	return nil
}
