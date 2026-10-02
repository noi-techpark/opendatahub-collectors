// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"testing"

	"opendatahub.com/tr-dss-lift/dto"
	odhmodel "opendatahub.com/tr-dss-lift/odhmodel"
)

func Test_mergeMapping_keepsExisting(t *testing.T) {
	base := &odhmodel.ODHActivityPoi{}
	base.Mapping = map[string]map[string]string{
		"idm": {"id": "ABC"},
		"dss": {"pid": "1", "custom": "keep", "capacity": "4"},
	}

	got := mergeMapping(base, map[string]string{"pid": "2", "capacity": "", "skiarea_rid": "4a"})

	if got["idm"]["id"] != "ABC" {
		t.Errorf("other source was dropped: %v", got)
	}
	want := map[string]string{"pid": "2", "custom": "keep", "capacity": "4", "skiarea_rid": "4a"}
	for k, v := range want {
		if got["dss"][k] != v {
			t.Errorf("dss[%s] = %q, want %q", k, got["dss"][k], v)
		}
	}
	if base.Mapping["dss"]["pid"] != "1" {
		t.Error("base mapping was modified in place")
	}
}

func Test_skiAreaRid(t *testing.T) {
	for _, c := range []struct {
		region int64
		sub    string
		want   string
	}{{4, "4a", "4a"}, {3, "all", "3"}, {6, "6a", "6a"}, {0, "", ""}} {
		if got := skiAreaRid(dto.DssLift{RegionId: c.region, SubregionId: c.sub}); got != c.want {
			t.Errorf("skiAreaRid(%d,%q) = %q, want %q", c.region, c.sub, got, c.want)
		}
	}
}
