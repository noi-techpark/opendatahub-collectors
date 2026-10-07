// SPDX-FileCopyrightText: 2025 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: CC0-1.0

package main

import (
	"encoding/json"
	"testing"
)

func Test_oneOrMany(t *testing.T) {
	var single, list, missing PanocloudLogos
	if err := json.Unmarshal([]byte(`{"Logo":{"@attributes":{"logoUrl":"a.png"}}}`), &single); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(`{"Logo":[{"@attributes":{"logoUrl":"a.png"}},{"@attributes":{"logoUrl":"b.png"}}]}`), &list); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(`{"Logo":null}`), &missing); err != nil {
		t.Fatal(err)
	}
	if len(single.Logo) != 1 || len(list.Logo) != 2 || list.Logo[1].Attributes.LogoUrl != "b.png" || len(missing.Logo) != 0 {
		t.Errorf("unexpected logos: %+v / %+v / %+v", single, list, missing)
	}
}

func Test_mapToCore_contactAndImages(t *testing.T) {
	var cam PanocloudCamera
	payload := `{
		"@attributes": {"name": "Seehotel", "pageTitle": "", "addressIso": "it", "geoPlacename": "Graun", "geoRegion": "Südtirol",
			"defaultLang": "de", "description": "Kurz", "longdescription": ""},
		"Images": {"image": [
			{"@attributes": {"fileType": "big", "fileUrl": "x.webcam/big.jpg", "mimeType": "image/jpeg"}},
			{"@attributes": {"fileType": "thumbnail", "fileUrl": "x.webcam/tmb.jpg", "mimeType": "image/jpeg"}}
		]},
		"Logos": {"Logo": [{"@attributes": {"logoUrl": "x.webcam/logo1.png"}}, {"@attributes": {"logoUrl": "x.webcam/logo2.png"}}]}
	}`
	if err := json.Unmarshal([]byte(payload), &cam); err != nil {
		t.Fatal(err)
	}
	webcam := mapToCore(cam, nil, "PANOCLOUD_1_1")

	ci := webcam.ContactInfos["de"]
	if ci.CompanyName != "Seehotel" || ci.CountryCode != "IT" || ci.CountryName != "Italien" || ci.City != "Graun" ||
		ci.Region != "Südtirol" || ci.LogoUrl != "https://x.webcam/logo1.png" {
		t.Errorf("unexpected ContactInfo %+v", ci)
	}
	if webcam.Detail["de"].BaseText != "Kurz" {
		t.Errorf("BaseText should fall back to the description, got %q", webcam.Detail["de"].BaseText)
	}
	gallery := webcam.ImageGallery
	if len(gallery) != 2 || gallery[0].ImageUrl != "https://x.webcam/tmb.jpg" || gallery[0].ListPosition == nil || gallery[1].ListPosition != nil {
		t.Errorf("thumbnail should come first with ListPosition 0, got %+v", gallery)
	}
}
