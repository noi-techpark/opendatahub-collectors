// SPDX-FileCopyrightText: NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import "time"

// CountingAreaList is the top-level structure, a slice of CountingArea
type CountingAreaList []CountingArea

// CountingArea represents a single element in the top-level array
type CountingArea struct {
	AppearanceParams  AppearanceParams `json:"appearance_params"`
	Counts            Counts           `json:"counts"`
	Created           time.Time        `json:"created"`
	ID                string           `json:"id"`
	LastResetTime     *time.Time       `json:"last_reset_time"` // Use *time.Time for null or date string
	MultiSg           bool             `json:"multi_sg"`
	Name              string           `json:"name"`
	ResetDayOfWeek    *string          `json:"reset_day_of_week"` // Use *string for null or string
	ResetMode         string           `json:"reset_mode"`
	ResetTimeOfDay    *string          `json:"reset_time_of_day"` // Use *string for null or string
	ResetTimezone     *string          `json:"reset_timezone"`    // Use *string for null or string
	SiteID            string           `json:"site_id"`
	Type              string           `json:"type"`
	Where             Where            `json:"where"`
	ParentStationCode string
}

// AppearanceParams contains parameters for object appearance
type AppearanceParams struct {
	AoiOverlapDegree             float64  `json:"aoi_overlap_degree"`
	BottomColours                *string  `json:"bottom_colours"` // null
	ClassCategories              []string `json:"class_categories"`
	DissimilarEmbeddings         *string  `json:"dissimilar_embeddings"` // null
	DissimilarityThreshold       float64  `json:"dissimilarity_threshold"`
	EnableObjectMotionEstimation bool     `json:"enableObjectMotionEstimation"`
	ExcludeStationaryObjects     bool     `json:"excludeStationaryObjects"`
	FaceWatchlistIds             []string `json:"face_watchlist_ids"`
	HasFace                      bool     `json:"has_face"`
	HasLicensePlate              bool     `json:"has_license_plate"`
	LicensePlateClauses          *string  `json:"license_plate_clauses"` // null
	MinimumAttributeScore        int      `json:"minimum_attribute_score"`
	MinimumObjectSize            int      `json:"minimum_object_size"`
	MinimumObjectSizeOverride    *string  `json:"minimum_object_size_override"` // null
	PerAttributeThresholds       *string  `json:"per_attribute_thresholds"`     // null
	PersonAccessories            *string  `json:"person_accessories"`           // null
	PersonBodyStates             *string  `json:"person_body_states"`           // null
	SimilarEmbeddings            *string  `json:"similar_embeddings"`           // null
	SimilarFaceEmbeddings        *string  `json:"similar_face_embeddings"`      // null
	SimilarityPlates             *string  `json:"similarity_plates"`            // null
	SimilarityThreshold          float64  `json:"similarity_threshold"`
	TopColours                   *string  `json:"top_colours"` // null
	VehicleColours               []string `json:"vehicle_colours"`
	VehicleTypes                 []int    `json:"vehicle_types"`
}

// Counts contains the totals array
type Counts struct {
	Totals []Total `json:"totals"`
}

// Total contains the aggregated counting data
type Total struct {
	CountDetails    CountDetails `json:"countDetails"`
	CountDetailsIn  CountDetails `json:"countDetailsIn"`
	CountDetailsOut CountDetails `json:"countDetailsOut"`
	CountPerson     int          `json:"countPerson"`
	CountPersonIn   int          `json:"countPersonIn"`
	CountPersonMin  int          `json:"countPersonMin"`
	CountPersonOut  int          `json:"countPersonOut"`
	CountVehicle    int          `json:"countVehicle"`
	CountVehicleIn  int          `json:"countVehicleIn"`
	CountVehicleMin int          `json:"countVehicleMin"`
	CountVehicleOut int          `json:"countVehicleOut"`
	Received        int64        `json:"received"` // Timestamp in milliseconds
}

// CountDetails holds vehicle type counts, which can be an empty object
type CountDetails struct {
	VehicleTypeCounts map[string]int `json:"vehicleTypeCounts"` // Use map[string]int for the dynamic keys
}

// Where specifies the location and devices
type Where struct {
	DeviceGroups  *string        `json:"device_groups"` // null
	DeviceSources []DeviceSource `json:"device_sources"`
}

// DeviceSource contains details about the source device
type DeviceSource struct {
	DeviceID               string   `json:"device_id"`
	Loi                    []string `json:"loi"`                       // Array is empty in sample, assuming []string
	ObjectTypeSizeSettings *string  `json:"object_type_size_settings"` // null
	Paoi                   []Paoi   `json:"paoi"`
	ServerGroupID          string   `json:"server_group_id"`
	Source                 string   `json:"source"`
	Version                int      `json:"version"`
	WithinGroup            bool     `json:"within_group"`
}

// Paoi (Polygon of Interest) contains a list of points
type Paoi struct {
	Points []Point `json:"points"`
}

// Point represents a coordinate
type Point struct {
	X float64 `json:"X"`
	Y float64 `json:"Y"`
}
