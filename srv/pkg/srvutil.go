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

package pkg

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/abcxyz/pkg/renderer"
	"io"
	"net/http"
)

// DecodeRequest provides a common implementation of JSON unmarshaling with
// well-defined error handling.
//
// Errors will be written to the provided response writer, with an error returned to the caller to alert them
// no further processing should happen on the request.
//
// It automatically closes the request body to prevent leaking.
// TODO: move this to abcxyz/pkg
func DecodeRequest[T any](ctx context.Context, w http.ResponseWriter, r *http.Request, h *renderer.Renderer) (*T, error) {
	req := new(T)

	t := r.Header.Get("content-type")
	if exp := "application/json"; len(t) < 16 || t[:16] != exp {
		err := fmt.Errorf("invalid content type: content-type %q is not %q", t, exp)
		h.RenderJSON(w, http.StatusUnsupportedMediaType, err)
		return nil, err
	}

	defer r.Body.Close()
	body := http.MaxBytesReader(w, r.Body, 2<<20) // 2MiB

	d := json.NewDecoder(body)

	if err := d.Decode(&req); err != nil {
		var syntaxErr *json.SyntaxError
		var unmarshalError *json.UnmarshalTypeError
		switch {
		case errors.As(err, &syntaxErr):
			err = fmt.Errorf("malformed json at position %d", syntaxErr.Offset)
			h.RenderJSON(w, http.StatusBadRequest, err)
			return nil, err
		case errors.Is(err, io.ErrUnexpectedEOF):
			err = fmt.Errorf("malformed json")
			h.RenderJSON(w, http.StatusBadRequest, err)
			return nil, err
		case errors.As(err, &unmarshalError):
			err = fmt.Errorf("invalid value for %q at position %d (expected %s, got %s)",
				unmarshalError.Field, unmarshalError.Offset, unmarshalError.Type, unmarshalError.Value)
			h.RenderJSON(w, http.StatusBadRequest, err)
			return nil, err
		case errors.Is(err, io.EOF):
			err = fmt.Errorf("body must not be empty")
			h.RenderJSON(w, http.StatusBadRequest, err)
			return nil, err
		case err.Error() == "http: request body too large":
			err = fmt.Errorf("request body too large")
			h.RenderJSON(w, http.StatusRequestEntityTooLarge, err)
			return nil, err
		default:
			err = fmt.Errorf("failed to decode request as json: %w", err)
			h.RenderJSON(w, http.StatusBadRequest, err)
			return nil, err
		}
	}
	if d.More() {
		err := fmt.Errorf("body contained more than one json object")
		h.RenderJSON(w, http.StatusBadRequest, err)
		return nil, err
	}
	return req, nil
}
