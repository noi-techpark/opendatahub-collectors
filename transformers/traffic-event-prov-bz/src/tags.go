// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
)

type Tag struct {
	ID     string `json:"id"`
	NameEn string `json:"name-en"`
	NameIt string `json:"name-it"`
	NameDe string `json:"name-de"`
}

type Tags []Tag

func (t Tags) FindById(id string) *Tag {
	for _, tag := range t {
		if tag.ID == id {
			return &tag
		}
	}
	return nil
}

func ReadTags(filename string) (Tags, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("failed opening json file: %w", err)
	}
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("failed reading json file: %w", err)
	}

	var tags Tags
	if err := json.Unmarshal(data, &tags); err != nil {
		return nil, fmt.Errorf("failed unmarshaling tags: %w", err)
	}

	return tags, nil
}
