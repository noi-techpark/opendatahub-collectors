// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"errors"

	"github.com/noi-techpark/opendatahub-go-sdk/clib"
)

const TagSource = "urbangreen"

// collectedTag holds tag data with mappings from multiple standards
type collectedTag struct {
	tagID   string
	names   map[string]string
	mapping map[string]map[string]string // version -> {type: id}
}

// SyncUrbanGreenTags syncs all MainType and SubType entries from all standards as tags to the content API
func SyncUrbanGreenTags(ctx context.Context, client clib.ContentAPI, standards *Standards) error {
	// Collect all tags with their mappings across all standard versions
	collected := make(map[string]*collectedTag)

	for _, standard := range standards.AllVersions() {
		// Collect MainType tags
		for _, mainType := range standard.MainTypes {
			tag := getOrCreateCollectedTag(collected, mainType.TagID, mainType.Names)
			addMapping(tag, standard.Version, "MainType", mainType.ID)
		}

		// Collect SubType tags
		for _, subType := range standard.SubTypes {
			tag := getOrCreateCollectedTag(collected, subType.TagID, subType.Names)
			addMapping(tag, standard.Version, "SubType", subType.ID)
		}
	}

	// Sync all collected tags
	for _, tag := range collected {
		if err := syncTag(ctx, client, tag); err != nil {
			return err
		}
	}

	return nil
}

func getOrCreateCollectedTag(collected map[string]*collectedTag, tagID string, names map[string]string) *collectedTag {
	if tag, exists := collected[tagID]; exists {
		return tag
	}

	tag := &collectedTag{
		tagID:   tagID,
		names:   names,
		mapping: make(map[string]map[string]string),
	}
	collected[tagID] = tag
	return tag
}

func addMapping(tag *collectedTag, version, typeKey, id string) {
	if tag.mapping[version] == nil {
		tag.mapping[version] = make(map[string]string)
	}
	tag.mapping[version][typeKey] = id
}

func syncTag(ctx context.Context, client clib.ContentAPI, collected *collectedTag) error {
	tag := &clib.Tag{
		ID:      clib.StringPtr(collected.tagID),
		Source:  TagSource,
		TagName: collected.names,
		Mapping: collected.mapping,
		Types:   []string{},
		LicenseInfo: &clib.LicenseInfo{
			ClosedData: false,
			License:    clib.StringPtr("CC0"),
		},
	}

	err := client.Post(ctx, "Tag", map[string]string{"generateid": "false"}, tag)
	if err != nil && !errors.Is(err, clib.ErrAlreadyExists) {
		return err
	}

	return nil
}
