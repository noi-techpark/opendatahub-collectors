// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"opendatahub.com/rest-push-skidata/skidata"
)

// FacilityCredential is the in-package alias for skidata.FacilityCredential
// so existing references in this package keep working unchanged.
type FacilityCredential = skidata.FacilityCredential

// Backoffice is the credential system of record.
//
// The collector used to hold them in an environment variable read once at
// startup, which made a password change a redeploy of a pod holding every live
// subscription. It now reads them from the parking control tower, which owns
// them, encrypts them at rest and records who changed what.
type Backoffice struct {
	URL    string
	User   string
	Pass   string
	Client *http.Client
}

// Fetch returns the full credential set.
//
// Whole-set, never a delta: the supervisor reconciles against what it is given,
// so a partial answer would silently unsubscribe every facility missing from
// it. That is also why a transport error must be returned rather than an empty
// slice -- they mean opposite things.
func (b *Backoffice) Fetch(ctx context.Context) ([]FacilityCredential, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		strings.TrimRight(b.URL, "/")+"/internal/credentials", nil)
	if err != nil {
		return nil, fmt.Errorf("building the credentials request: %w", err)
	}
	req.SetBasicAuth(b.User, b.Pass)
	req.Header.Set("Accept", "application/json")

	res, err := b.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("reaching the backoffice: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(res.Body, 300))
		return nil, fmt.Errorf("backoffice returned %s: %s", res.Status, bytes.TrimSpace(body))
	}
	var out []FacilityCredential
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decoding the credential set: %w", err)
	}
	return out, nil
}
