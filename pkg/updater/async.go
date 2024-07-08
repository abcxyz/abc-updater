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

package updater

import (
	"context"

	"github.com/abcxyz/pkg/logging"
)

// asyncFunctionCall handles the async part of CheckAppVersionAsync, but accepts
// a function other than CheckAppVersion for testing.
func asyncFunctionCall(ctx context.Context, funcToCall func() (string, error), outFunc func(string)) func() {
	updatesCh := make(chan string, 1)

	go func() {
		defer close(updatesCh)
		message, err := funcToCall()
		if err != nil {
			logging.FromContext(ctx).WarnContext(ctx, "failed to check for new versions",
				"error", err)
			return
		}
		updatesCh <- message
	}()

	return func() {
		select {
		case <-ctx.Done():
			// Context was cancelled
		case msg, ok := <-updatesCh:
			if ok && len(msg) > 0 {
				outFunc(msg)
			}
		}
	}
}
