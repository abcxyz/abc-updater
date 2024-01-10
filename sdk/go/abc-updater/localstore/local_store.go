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

// Package localstore is an interface for persistent JSON storage on the users machine. This
// is internal and only intended to be imported from this repo.
package localstore

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type localData struct {
	LastVersionCheckUTCEpochSec int64 `json:"last_version_check_utc_epoch_sec"`
}

type localStore struct {
	directory string
}

const dataFilename = "data.json"

// Init sets up localStore with the default config location for the app.
func Init(appID string) (*localStore, error) {
	if appID == "" {
		return nil, fmt.Errorf("must supply non empty appID")
	}
	dir, err := defaultDir(appID)
	if err != nil {
		return nil, fmt.Errorf("failed to get default directory: %w", err)
	}

	return InitWithDir(dir)
}

// defaultDir returns the default localStore directory given an appID.
func defaultDir(appID string) (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get user home directory: %w", err)
	}

	return filepath.Join(homeDir, ".config", "abcupdater", appID), nil
}

// InitWithDir sets up localStore with the provided directory.
func InitWithDir(dir string) (*localStore, error) {
	if dir == "" {
		return nil, fmt.Errorf("directory cannot be empty string")
	}
	return &localStore{directory: dir}, nil
}

// LoadLocalData reads from local store and returns localData.
func (l *localStore) LoadLocalData() (*localData, error) {
	datafileFullPath := filepath.Join(l.directory, dataFilename)
	f, err := os.Open(datafileFullPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open data file: %w", err)
	}
	defer f.Close()

	var data localData
	if err := json.NewDecoder(f).Decode(&data); err != nil {
		return nil, fmt.Errorf("failed to decode data from %s: %w", datafileFullPath, err)
	}

	return &data, nil
}

// UpdateLocalData updates the local store with the provided localData.
func (l *localStore) UpdateLocalData(localData *localData) error {
	if err := os.MkdirAll(l.directory, 0o755); err != nil {
		return fmt.Errorf("failed to create directory for localStore at %s: %w", l.directory, err)
	}

	localDataPath := l.localDataPath()
	f, err := os.Create(localDataPath)
	if err != nil {
		return fmt.Errorf("failed to create data file at %s: %w", localDataPath, err)
	}
	defer f.Close()

	encoder := json.NewEncoder(f)
	if err := encoder.Encode(localData); err != nil {
		return fmt.Errorf("failed to encode data: %w", err)
	}

	return nil
}

// localDataPath is the fullpath for the local data file.
func (l *localStore) localDataPath() string {
	return filepath.Join(l.directory, dataFilename)
}
