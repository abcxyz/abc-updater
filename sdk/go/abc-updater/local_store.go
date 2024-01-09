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

package abcupdater

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type localData struct {
	LastVersionCheckTimestamp int64 `json:"last_version_check_timestamp"`
}

type localStoreSettings struct {
	// appID will be used to set a default localStore directory.
	AppID string

	// directory can be supplied to override the default localStore directory.
	Directory string

	// testLocalStoreDirFn is used to override the default function for getting localStore dir.
	testLocalStoreDirFn func(string) (string, error)
}

type localStore struct {
	directory string
}

const defaultLocalStoreDirFormat = "%s/.config/abcupdater/%s/"

func initLocalStore(settings *localStoreSettings) (*localStore, error) {
	directory := settings.Directory
	if directory == "" {
		if settings.AppID == "" {
			return nil, fmt.Errorf("must supply either appID or directory in settings")
		}
		localStoreDirFn := defaultLocalStoreDir
		if settings.testLocalStoreDirFn != nil {
			localStoreDirFn = settings.testLocalStoreDirFn
		}

		defaultDir, err := localStoreDirFn(settings.AppID)
		if err != nil {
			return nil, fmt.Errorf("failed to get default directory: %w", err)
		}
		directory = defaultDir
	}

	if err := os.MkdirAll(directory, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create directory for localStore: %w", err)
	}

	return &localStore{directory: directory}, nil
}

func defaultLocalStoreDir(appID string) (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get user home directory: %w", err)
	}

	return fmt.Sprintf(defaultLocalStoreDirFormat, homeDir, appID), nil
}

func (l *localStore) loadLocalData() (*localData, error) {
	dataFilename := filepath.Join(l.directory, "data.json")
	f, err := os.Open(dataFilename)
	if err != nil {
		return nil, fmt.Errorf("failed to open data file: %w", err)
	}
	defer f.Close()

	var data localData
	if err := json.NewDecoder(f).Decode(&data); err != nil {
		return nil, fmt.Errorf("failed to decode data: %w", err)
	}

	return &data, nil

}

func (l *localStore) updateLocalData(localData *localData) error {
	f, err := os.Create(l.localDataFilename())
	if err != nil {
		return fmt.Errorf("failed to create data file: %w", err)
	}
	defer f.Close()

	encoder := json.NewEncoder(f)
	if err := encoder.Encode(localData); err != nil {
		return fmt.Errorf("failed to encode data: %w", err)
	}

	return nil
}

func (l *localStore) localDataFilename() string {
	return filepath.Join(l.directory, "data.json")
}
