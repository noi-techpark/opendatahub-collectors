// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"testing"

	"opendatahub.com/tr-dss-skiareas/dto"
)

func Test_dssMapping_mergedIntoExisting(t *testing.T) {
	existing := map[string]map[string]string{
		"idm": {"id": "SKIFFC3B47C3CEA4426AE850E333EFE79CE"},
		"dss": {"rid": "4b", "custom": "keep"},
	}
	area := dto.DssSkiArea{
		Rid:          "4b",
		ActiveWinter: 1,
		ActiveBike:   0,
		Pid:          " 42 ",
		Skiresorts:   []dto.DssSkiresort{{Rid: 23, Pid: 82}, {Rid: 0}, {Rid: 24, Pid: 83}},
	}
	area.Email.Lifts = " info@lifts.example "

	got := mergeMapping(existing, dssMapping(area))

	if got["idm"]["id"] != "SKIFFC3B47C3CEA4426AE850E333EFE79CE" {
		t.Errorf("idm mapping was dropped: %v", got)
	}
	want := map[string]string{
		"rid": "4b", "custom": "keep", "skiresort_rids": "23,24",
		"pid": "42", "skiresort_pids": "82,83",
		"activeWinter": "true", "activeBike": "false", "activeHike": "false",
		"email_lifts": "info@lifts.example",
	}
	for k, v := range want {
		if got["dss"][k] != v {
			t.Errorf("dss[%s] = %q, want %q", k, got["dss"][k], v)
		}
	}
}
