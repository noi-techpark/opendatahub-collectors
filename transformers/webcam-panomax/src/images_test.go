// SPDX-FileCopyrightText: 2025 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: CC0-1.0

package main

import "testing"

func Test_mapToCore_images(t *testing.T) {
	cam := PanomaxCamera{Id: 7, CamId: 7, Images: []PanomaxImage{
		{Url: "https://live-image.panomax.com/cams/7/recent_full.jpg"},
		{Url: "https://live-image.panomax.com/cams/7/recent_thumb.jpg"},
		{Url: "https://live-image.panomax.com/cams/7/recent_default.jpg"},
		{Url: "https://live-image.panomax.com/cams/7/recent_small.jpg"},
	}}
	gallery := mapToCore(cam, nil, buildID(7), nil).ImageGallery

	want := []struct {
		suffix    string
		position  bool
		thumbnail bool
	}{
		{"recent_thumb.jpg", true, true},
		{"recent_small.jpg", true, false},
		{"recent_full.jpg", false, false},
		{"recent_default.jpg", false, false},
	}
	for i, w := range want {
		img := gallery[i]
		if img.ImageUrl != "https://live-image.panomax.com/cams/7/"+w.suffix {
			t.Errorf("image %d: url %s, want %s", i, img.ImageUrl, w.suffix)
		}
		if (img.ListPosition != nil) != w.position || (img.ListPosition != nil && *img.ListPosition != 0) {
			t.Errorf("image %d (%s): unexpected ListPosition %v", i, w.suffix, img.ListPosition)
		}
		if (len(img.ImageTags) == 1 && img.ImageTags[0] == "thumbnail") != w.thumbnail {
			t.Errorf("image %d (%s): unexpected ImageTags %v", i, w.suffix, img.ImageTags)
		}
		if img.IsInGallery != nil || img.ImageDesc == nil {
			t.Errorf("image %d: IsInGallery should be null and ImageDesc {}", i)
		}
	}
}

func Test_mapToCore_englishOnlyAndTourCam(t *testing.T) {
	cam := PanomaxCamera{Id: 7, CamId: 7, Name: "Großglockner", State: "Carinthia", Country: "at", TourCam: true}

	// An existing record still has de/it from earlier runs: they are removed.
	base := mapToCore(cam, nil, buildID(7), nil)
	base.HasLanguage = []string{"de", "it", "en"}
	base.Detail["de"] = base.Detail["en"]
	base.ContactInfos["it"] = base.ContactInfos["en"]

	webcam := mapToCore(cam, &base, buildID(7), nil)
	if len(webcam.HasLanguage) != 1 || webcam.HasLanguage[0] != "en" || len(webcam.Detail) != 1 || len(webcam.ContactInfos) != 1 {
		t.Errorf("expected only en, got HasLanguage %v, Detail %d, ContactInfos %d", webcam.HasLanguage, len(webcam.Detail), len(webcam.ContactInfos))
	}
	if webcam.Detail["en"].Title != "Großglockner" || webcam.ContactInfos["en"].Region != "Carinthia" || webcam.ContactInfos["en"].CountryCode != "AT" {
		t.Errorf("unexpected en content: %+v / %+v", webcam.Detail["en"], webcam.ContactInfos["en"])
	}
	if !webcam.WebCamProperties.TourCam {
		t.Error("TourCam not mapped")
	}
}
