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
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"path/filepath"

	"github.com/abcxyz/abc-updater/pkg/localstore"
)

// InstallIDData defines the json file that defines installation id.
type InstallIDData struct {
	// Time ID was created, in UTC epoch seconds. Currently unused.
	IDCreatedTimestamp int64 `json:"idCreatedTimestamp"`

	// InstallID. Expected to be a hex 8-4-4-4-12 formatted v4 UUID.
	InstallID string `json:"installId"`
}

// Only check if non-empty for now, as we don't currently have versioned APIs,
// so we want to be forward compatible.
func validInstallID(id string) bool {
	return len(id) > 0
}

// Generate a cryptographically secure 64bit base64-encoded random install ID.
// Collisions aren't a huge concern, so no need for UUID level entropy.
func generateInstallID() (string, error) {
	// 8 bytes = 64 bits.
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("error generating install ID: %w", err)
	}
	return base64.StdEncoding.EncodeToString(b), nil
}

// A per-application install id is randomly generated.
func loadInstallID(appID, installIDFileOverride string) (*InstallIDData, error) {
	path := installIDFileOverride
	if path == "" {
		dir, err := localstore.DefaultDir(appID)
		if err != nil {
			return nil, fmt.Errorf("could not calculate install ID path: %w", err)
		}
		path = filepath.Join(dir, installIDFileName)
	}
	var stored InstallIDData

	if err := localstore.LoadJSONFile(path, &stored); err != nil {
		return nil, fmt.Errorf("could not load install id: %w", err)
	}
	// Validate InstallID
	if !validInstallID(stored.InstallID) {
		return nil, fmt.Errorf("invalid install id")
	}

	return &stored, nil
}

func storeInstallID(appID, installIDFileOverride string, data *InstallIDData) error {
	if installIDFileOverride == "" {
		dir, err := localstore.DefaultDir(appID)
		if err != nil {
			return fmt.Errorf("could not calculate install ID path: %w", err)
		}
		installIDFileOverride = filepath.Join(dir, installIDFileName)
	}
	if err := localstore.StoreJSONFile(installIDFileOverride, data); err != nil {
		return fmt.Errorf("could not store install id: %w", err)
	}
	return nil
}
