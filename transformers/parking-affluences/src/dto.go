// SPDX-FileCopyrightText: 2026 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

// AffluencesPayload is the merged per-poll document produced by the
// api-crawler collector: one array element per Affluences site, each carrying
// the site's static metadata plus the nested real-time reading. Site.ID is
// the provider key from which the BDP station code is derived (clib.GenerateID).
type AffluencesPayload []Site

// Site is the Affluences site metadata ($.data of /v1/site/{id}) with the
// real-time reading merged in under "realtime" by the collector.
type Site struct {
	ID            string     `json:"id"`
	ParentID      *string    `json:"parent_id"`
	PrimaryName   string     `json:"primary_name"`
	SecondaryName string     `json:"secondary_name"`
	Categories    []Category `json:"categories"`
	Timezone      string     `json:"timezone"`
	Location      Location   `json:"location"`
	PhoneNumber   *string    `json:"phone_number"`
	Email         *string    `json:"email"`
	URL           *string    `json:"url"`

	Realtime Realtime `json:"realtime"`
}

type Category struct {
	ID         int    `json:"id"`
	Name       string `json:"name"`
	NamePlural string `json:"name_plural"`
}

type Location struct {
	Coordinates Coordinates `json:"coordinates"`
	Address     Address     `json:"address"`
}

type Coordinates struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

type Address struct {
	Route       *string `json:"route"`
	City        *string `json:"city"`
	ZipCode     *string `json:"zip_code"`
	Region      *string `json:"region"`
	CountryCode *string `json:"country_code"`
}

// Realtime is $.data of /v1/data/realtime/{id}. Only occupancy, capacity and
// open are activated for the monitored sites; entries/exits/waiting_* are
// documented as not-to-be-used and left out. occupancy and capacity are
// pointers so a not-yet-activated site (null) is skipped rather than read
// as zero.
type Realtime struct {
	SiteID               string   `json:"site_id"`
	Open                 bool     `json:"open"`
	Occupancy            *int     `json:"occupancy"`
	OccupancyRate        *float64 `json:"occupancy_rate"`
	OccupancyDatetimeUTC string   `json:"occupancy_datetime_utc"`
	Capacity             *int     `json:"capacity"`
}
