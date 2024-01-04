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
	"log/slog"

	"github.com/abcxyz/pkg/logging"
	"github.com/sethvargo/go-envconfig"
)

type debugLogger struct {
	Enabled bool `env:"ABC_UPDATER_DEBUG_ENABLED,default=0"`
	logger  *slog.Logger
}

func initLogger(ctx context.Context) *debugLogger {
	var d debugLogger
	if err := envconfig.ProcessWith(ctx, &d, envconfig.OsLookuper()); err != nil {
		return &debugLogger{}
	}

	d.logger = logging.NewFromEnv("ABC_UPDATER_")
	return &d
}

func (d *debugLogger) errorContext(ctx context.Context, msg string, args ...any) {
	if !d.Enabled {
		return
	}

	d.logger.ErrorContext(ctx, msg, args...)
}
