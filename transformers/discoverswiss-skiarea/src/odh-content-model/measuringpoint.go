// SPDX-FileCopyrightText: 2026 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: MPL-2.0

package odhContentModel

// WeatherObservation represents a weather observation entry
type WeatherObservation struct {
	Id            *string           `json:"Id,omitempty"`
	WeatherStatus map[string]string `json:"WeatherStatus,omitempty"`
	WeatherCode   *string           `json:"WeatherCode,omitempty"`
	IconID        *string           `json:"IconID,omitempty"`
	Date          *string           `json:"Date,omitempty"`
}

// MeasuringpointV2 corresponds to the ODH Weather/Measuringpoint model.
// Used for weather and snow condition data linked to a ski area.
type MeasuringpointV2 struct {
	Generic // Embedded (do NOT set Geo — not in MeasuringpointV2 schema)

	Detail             map[string]DetailGeneric `json:"Detail,omitempty"`
	SnowHeight         *string                  `json:"SnowHeight,omitempty"`
	NewSnowHeight      *string                  `json:"newSnowHeight,omitempty"`
	Temperature        *string                  `json:"Temperature,omitempty"`
	LastSnowDate       *string                  `json:"LastSnowDate,omitempty"`
	WeatherObservation []WeatherObservation     `json:"WeatherObservation,omitempty"`
	LocationInfo       *LocationInfo            `json:"LocationInfo,omitempty"`
	AreaIds            []string                 `json:"AreaIds,omitempty"`
	SkiAreaIds         []string                 `json:"SkiAreaIds,omitempty"`
	GpsInfo            []GpsInfo                `json:"GpsInfo,omitempty"`
	Tags               []Tag                    `json:"Tags,omitempty"`
}
