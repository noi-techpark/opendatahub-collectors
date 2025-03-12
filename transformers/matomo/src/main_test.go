// SPDX-FileCopyrightText: 2025 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"os"
	"testing"

	"gotest.tools/v3/assert"
)

func TestUnmarshal(t *testing.T) {
	f, err := os.ReadFile("./testdata/noi.matomo.cloud.json")
	assert.NilError(t, err)
	_, err = unmarshalRawJson(string(f))
	assert.NilError(t, err)
}

func TestShorten(t *testing.T) {
	assert.Equal(t, "test", shortenUnique("test"))
	short1 := shortenUnique("noi.bz.it/it/chi-siamo/societa-trasparente/bandi-di-gara-e-contratti/composizione-delle-commissioni-di-valutazione-e-curricula-e-collegio-consultivo-tecnico/commissioni-di-valutazione/lieferung-und-installation-audio-video-anlage-noi-techpark-bozen-baulos-b1")
	short2 := shortenUnique("noi.bz.it/it/chi-siamo/societa-trasparente/bandi-di-gara-e-contratti/composizione-delle-commissioni-di-valutazione-e-curricula-e-collegio-consultivo-tecnico/commissioni-di-valutazione/lieferung-und-installation-audio-video-anlage-noi-techpark-bozen-baulos-b1dahsdiahwoidaoiwhd")
	assert.Equal(t, 255, len(short1))
	assert.Assert(t, short1 != short2)
}
