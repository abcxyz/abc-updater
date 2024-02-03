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

// Package localstore is a helper for persistent JSON storage on the users
// machine.
package localstore

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// DefaultDir returns the default local updater storage directory given an appID.
func DefaultDir(appID string) (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get user home directory: %w", err)
	}

	return filepath.Join(homeDir, ".config", "abcupdater", appID), nil
}

// LoadJSONFile unmarshals file contents from the given file path into a generic object. data cannot be nil.
func LoadJSONFile[T any](path string, data T) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("failed to open json file: %w", err)
	}
	defer f.Close()

	if err := json.NewDecoder(f).Decode(data); err != nil {
		return fmt.Errorf("failed to load json file: %w", err)
	}
	return nil
}

// StoreJSONFile marshals data from the given object into file with given path. File and directory tree will be
// created if they do not exist. data cannot be nil.
func StoreJSONFile[T any](path string, data T) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("failed to create directory for json file at %s: %w", dir, err)
	}
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create json file at %s: %w", path, err)
	}
	defer f.Close()

	encoder := json.NewEncoder(f)
	if err := encoder.Encode(data); err != nil {
		return fmt.Errorf("failed to encode JSON: %w", err)
	}

	return nil
}
