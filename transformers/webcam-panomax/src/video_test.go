// SPDX-FileCopyrightText: 2025 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: CC0-1.0

package main

import (
	"encoding/json"
	"testing"

	contentmodel "github.com/noi-techpark/opendatahub-collectors/transformers/webcam-panomax/content-model"
)

func Test_PanomaxRawData_formats(t *testing.T) {
	var legacy PanomaxRawData
	if err := json.Unmarshal([]byte(`[{"id":7,"camId":7}]`), &legacy); err != nil {
		t.Fatal(err)
	}
	if len(legacy.Webcams) != 1 || legacy.Webcams[0].CamId != 7 || len(legacy.Videos) != 0 {
		t.Errorf("legacy array not parsed: %+v", legacy)
	}

	var current PanomaxRawData
	payload := `{"webcams":[{"id":7,"camId":7}],"videos":[{"id":7,"videos":[{"url":"u","width":"480","height":"270","fileName":"7_mobile.mp4"}]}]}`
	if err := json.Unmarshal([]byte(payload), &current); err != nil {
		t.Fatal(err)
	}
	if len(current.Webcams) != 1 || len(current.Videos) != 1 || current.Videos[0].Videos[0].FileName != "7_mobile.mp4" {
		t.Errorf("payload not parsed: %+v", current)
	}
}

func Test_mapToCore_videos(t *testing.T) {
	videos := []PanomaxVideo{
		{Url: "https://video.panomax.com/cams/19/videos/19_fullHD.mp4", Width: "1920", Height: "1080", FileName: "19_fullHD.mp4"},
		{Url: "https://video.panomax.com/cams/19/videos/19_mobile.mp4", Width: "480", Height: "270", FileName: "19_mobile.mp4"},
	}
	webcam := mapToCore(PanomaxCamera{Id: 1, CamId: 19}, nil, buildID(1), videos)

	items := webcam.VideoItems["en"]
	if len(items) != 2 || len(webcam.VideoItems) != 1 {
		t.Fatalf("expected both videos under en, got %+v", webcam.VideoItems)
	}
	if items[1].Url != videos[1].Url || items[1].VideoTitle != "19_mobile.mp4" || items[1].Width != 480 ||
		items[1].Height != 270 || items[1].VideoSource != "panomax" || items[1].Language != "en" || !items[1].Active {
		t.Errorf("unexpected video item %+v", items[1])
	}

	// Leftover videos of an existing record (e.g. junk "de" entries) are replaced.
	base := webcam
	base.VideoItems = map[string][]contentmodel.VideoItem{"de": {{Url: "testde"}}}
	updated := mapToCore(PanomaxCamera{Id: 1, CamId: 19}, &base, buildID(1), nil)
	if len(updated.VideoItems) != 0 {
		t.Errorf("expected no videos for a cam without videos, got %+v", updated.VideoItems)
	}
}
