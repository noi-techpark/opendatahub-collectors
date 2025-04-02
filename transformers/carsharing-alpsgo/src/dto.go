// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"fmt"
	"time"

	"github.com/noi-techpark/go-bdp-client/bdplib"
)

// Root holds the top-level fields: availabilities, stations, vehicles.
type Root struct {
	Availabilities []Availability `json:"availabilities"`
	Stations       []Station      `json:"stations"`
	Vehicles       []Vehicle      `json:"vehicles"`
}

// --------------------------------------------------------
// Availabilities + Slots
// --------------------------------------------------------

type Availability struct {
	VehicleID int    `json:"vehicle_id"`
	Slots     []Slot `json:"slots"`
}

type Slot struct {
	Available bool    `json:"available"`
	From      string  `json:"from"`
	Until     *string `json:"until"` // pointer, because "until" can be null
}

// --------------------------------------------------------
// Stations
// --------------------------------------------------------

type Station struct {
	ID                    int         `json:"id"`
	CapacityCurrentlyFree int         `json:"capacity_currently_free"`
	CapacityMax           int         `json:"capacity_max"`
	CenterLat             float64     `json:"center_lat"`
	CenterLng             float64     `json:"center_lng"`
	City                  string      `json:"city"`
	Image                 *string     `json:"image"`
	Kind                  *string     `json:"kind"`
	Lat                   float64     `json:"lat"`
	Lng                   float64     `json:"lng"`
	Name                  string      `json:"name"`
	NavigationalLat       float64     `json:"navigational_lat"`
	NavigationalLng       float64     `json:"navigational_lng"`
	PickupDescription     string      `json:"pickup_description"`
	Polygon               interface{} `json:"polygon"` // or json.RawMessage if it's structured geometry
	Postcode              string      `json:"postcode"`
	Radius                int         `json:"radius"`
	ReturnDescription     string      `json:"return_description"`
	Street                string      `json:"street"`
}

func (s Station) ToBDPStation(bdp bdplib.Bdp) bdplib.Station {
	bdpStation := bdplib.CreateStation(
		fmt.Sprintf("%d", s.ID), s.Name, StationTypeCarSharing, s.Lat, s.Lng, bdp.GetOrigin(),
	)
	bdpStation.MetaData = make(map[string]any)

	bdpStation.MetaData["capacity_max"] = s.CapacityMax

	// Add fields that are not used in the primary station creation, if they have values.
	if s.CenterLat != 0 {
		bdpStation.MetaData["center_lat"] = s.CenterLat
	}
	if s.CenterLng != 0 {
		bdpStation.MetaData["center_lng"] = s.CenterLng
	}
	if s.City != "" {
		bdpStation.MetaData["city"] = s.City
	}
	if s.Image != nil && *s.Image != "" {
		bdpStation.MetaData["image"] = *s.Image
	}
	if s.Kind != nil && *s.Kind != "" {
		bdpStation.MetaData["kind"] = *s.Kind
	}
	if s.NavigationalLat != 0 {
		bdpStation.MetaData["navigational_lat"] = s.NavigationalLat
	}
	if s.NavigationalLng != 0 {
		bdpStation.MetaData["navigational_lng"] = s.NavigationalLng
	}
	if s.PickupDescription != "" {
		bdpStation.MetaData["pickup_description"] = s.PickupDescription
	}
	if s.Polygon != nil {
		bdpStation.MetaData["polygon"] = s.Polygon
	}
	if s.Postcode != "" {
		bdpStation.MetaData["postcode"] = s.Postcode
	}
	if s.Radius != 0 {
		bdpStation.MetaData["radius"] = s.Radius
	}
	if s.ReturnDescription != "" {
		bdpStation.MetaData["return_description"] = s.ReturnDescription
	}
	if s.Street != "" {
		bdpStation.MetaData["street"] = s.Street
	}

	return bdpStation
}

// --------------------------------------------------------
// Vehicles
// --------------------------------------------------------

type Vehicle struct {
	ID                             int                 `json:"id"`
	AllowRFIDCardAccess            bool                `json:"allow_rfid_card_access"`
	BookableExtras                 []string            `json:"bookable_extras"`
	CarType                        string              `json:"car_type"`
	Cleanness                      string              `json:"cleanness"`
	CruisingRange                  SourceValue         `json:"cruising_range"`
	CurrentParkingHint             *string             `json:"current_parking_hint"`
	Distance                       interface{}         `json:"distance"` // or *float64, if numeric
	EVCharging                     string              `json:"ev_charging"`
	Fuel                           SimpleValue         `json:"fuel"`
	FuelType                       string              `json:"fuel_type"`
	HasFuelChargeCard              bool                `json:"has_fuel_charge_card"`
	HasParkingCard                 bool                `json:"has_parking_card"`
	Image                          VehicleImage        `json:"image"`
	Insurances                     []Insurance         `json:"insurances"`
	Label                          string              `json:"label"`
	LegalDocuments                 []LegalDocument     `json:"legal_documents"`
	License                        string              `json:"license"`
	Location                       *Location           `json:"location"`
	MinimumAgeInsuranceRequirement int                 `json:"minimum_age_insurance_requirement"`
	MinimumPricing                 MinimumPricing      `json:"minimum_pricing"`
	ReturnRequirements             ReturnRequirements  `json:"return_requirements"`
	RFIDSlot1                      RFIDSlot            `json:"rfid_slot_1"`
	RFIDSlot2                      RFIDSlot            `json:"rfid_slot_2"`
	Transmission                   string              `json:"transmission"`
	VehicleCategories              []VehicleCategories `json:"vehicle_categories"`
	VehicleModel                   VehicleModel        `json:"vehicle_model"`
	VehicleType                    string              `json:"vehicle_type"`
	VehicleUsageInstructionsURL    string              `json:"vehicle_usage_instructions_url"`
}

func (s Vehicle) ToBDPStation(bdp bdplib.Bdp) bdplib.Station {
	bdpStation := bdplib.CreateStation(
		fmt.Sprintf("%d", s.ID), s.License, StationTypeVechile,
		0,
		0,
		bdp.GetOrigin(),
	)

	bdpStation.MetaData = make(map[string]any)

	// Map location to ParentID.
	// if nil != s.Location {
	// 	bdpStation.MetaData["last_location_id"] = fmt.Sprintf("%d", s.Location.ID)
	// }

	// Map remaining fields to metadata.
	if s.FuelType != "" {
		bdpStation.MetaData["fuel_type"] = s.FuelType
	}
	if s.Transmission != "" {
		bdpStation.MetaData["transmission"] = s.Transmission
	}
	if s.CurrentParkingHint != nil && *s.CurrentParkingHint != "" {
		bdpStation.MetaData["current_parking_hint"] = *s.CurrentParkingHint
	}
	bdpStation.MetaData["allow_rfid_card_access"] = s.AllowRFIDCardAccess

	if s.Distance != nil {
		bdpStation.MetaData["distance"] = s.Distance
	}
	if s.Label != "" {
		bdpStation.MetaData["label"] = s.Label
	}
	if len(s.LegalDocuments) > 0 {
		bdpStation.MetaData["legal_documents"] = s.LegalDocuments
	}
	// For the image, add the whole struct; you may add additional checks if needed.
	bdpStation.MetaData["image"] = s.Image

	bdpStation.MetaData["return_requirements"] = s.ReturnRequirements
	bdpStation.MetaData["cruising_range"] = s.CruisingRange
	if s.EVCharging != "" {
		bdpStation.MetaData["ev_charging"] = s.EVCharging
	}
	// Use minimum_pricing as both "pricing" and "minimum_pricing" in metadata.
	bdpStation.MetaData["pricing"] = s.MinimumPricing
	bdpStation.MetaData["minimum_pricing"] = s.MinimumPricing

	bdpStation.MetaData["has_fuel_charge_card"] = s.HasFuelChargeCard
	bdpStation.MetaData["has_parking_card"] = s.HasParkingCard
	bdpStation.MetaData["minimum_age_insurance_requirement"] = s.MinimumAgeInsuranceRequirement

	if len(s.Insurances) > 0 {
		bdpStation.MetaData["insurances"] = s.Insurances
	}

	bdpStation.MetaData["rfid_slot_1"] = s.RFIDSlot1
	bdpStation.MetaData["rfid_slot_2"] = s.RFIDSlot2
	bdpStation.MetaData["vehicle_model"] = s.VehicleModel

	return bdpStation
}

// --------------------------------------------------------
// Nested fields for Vehicle
// --------------------------------------------------------

// SourceValue includes a "source" field plus a nested numeric structure.
type SourceValue struct {
	Source string      `json:"source"`
	Value  ValueDetail `json:"value"`
}

// SimpleValue is a small variant that doesn't show "currency". If you do see
// currency in your real data, unify them into a single type with optional fields.
type SimpleValue struct {
	Cents     int    `json:"cents"`
	Formatted string `json:"formatted"`
	Value     string `json:"value"`
}

// ValueDetail can hold currency, value, formatting, etc.
// If different subfields appear in different objects, you can unify them or
// define separate structs that match precisely.
type ValueDetail struct {
	Cents        int     `json:"cents"`
	Currency     *string `json:"currency"`      // might be null
	CurrencyCode *string `json:"currency_code"` // might be null
	Formatted    string  `json:"formatted"`
	Value        string  `json:"value"`
	VATRate      *int    `json:"vat_rate"`
	VATStatus    *string `json:"vat_status"`
}

// VehicleImage holds the URLs for the vehicle images.
type VehicleImage struct {
	MediumURL string `json:"medium_url"`
	ThumbURL  string `json:"thumb_url"`
	URL       string `json:"url"`
}

// Insurance includes the "deductible_value" and other details.
type Insurance struct {
	ID                    int         `json:"id"`
	Kind                  string      `json:"kind"`
	Title                 string      `json:"title"`
	Description           string      `json:"description"`
	Attachment            interface{} `json:"attachment"`
	MinimumAgeRequirement interface{} `json:"minimum_age_requirement"` // might be int, null, or omitted
	DeductibleValue       ValueDetail `json:"deductible_value"`
}

// LegalDocument for legal_documents array in Vehicle
type LegalDocument struct {
	Kind  string `json:"kind"`
	Title string `json:"title"`
	URL   string `json:"url"`
}

// Location is embedded under the vehicle
type Location struct {
	ID                    int         `json:"id"`
	Kind                  *string     `json:"kind"`
	CapacityCurrentlyFree int         `json:"capacity_currently_free"`
	CapacityMax           int         `json:"capacity_max"`
	CenterLat             float64     `json:"center_lat"`
	CenterLng             float64     `json:"center_lng"`
	City                  string      `json:"city"`
	Lat                   float64     `json:"lat"`
	Lng                   float64     `json:"lng"`
	Name                  string      `json:"name"`
	NavigationalLat       float64     `json:"navigational_lat"`
	NavigationalLng       float64     `json:"navigational_lng"`
	PickupDescription     *string     `json:"pickup_description"`
	Polygon               interface{} `json:"polygon"`
	Postcode              string      `json:"postcode"`
	Radius                int         `json:"radius"`
	ReturnDescription     *string     `json:"return_description"`
	Street                string      `json:"street"`
}

// MinimumPricing breaks down the cost fields, including nested "details."
type MinimumPricing struct {
	BaseFee                      ValueDetail     `json:"base_fee"`
	ChargePlannedDuration        bool            `json:"charge_planned_duration"`
	Coupon                       interface{}     `json:"coupon"`
	Details                      []PricingDetail `json:"details"`
	From                         time.Time       `json:"from"`
	KM                           int             `json:"km"`
	KMCharge                     ValueDetail     `json:"km_charge"`
	KMDiscountApplied            interface{}     `json:"km_discount_applied"`
	KMIncluded                   int             `json:"km_included"`
	KMToCharge                   int             `json:"km_to_charge"`
	MinimalValueAdjustmentCharge interface{}     `json:"minimal_value_adjustment_charge"`
	ToPayPerKM                   ValueDetail     `json:"to_pay_per_km"`
	TotalToPay                   ValueDetail     `json:"total_to_pay"`
	Until                        time.Time       `json:"until"`
}

// PricingDetail holds finer‐grained breakdown of cost lines.
type PricingDetail struct {
	Count             int         `json:"count"`
	KMIncludedPerUnit int         `json:"km_included_per_unit"`
	Name              string      `json:"name"`
	Title             string      `json:"title"`
	ToPayPerUnit      ValueDetail `json:"to_pay_per_unit"`
	Total             ValueDetail `json:"total"`
}

// ReturnRequirements describes how and where the user must return the vehicle.
type ReturnRequirements struct {
	Areas                   []ReturnArea `json:"areas"`
	Card1                   interface{}  `json:"card_1"`
	Card2                   interface{}  `json:"card_2"`
	CentralLock             string       `json:"central_lock"`
	Doors                   string       `json:"doors"`
	Fuel                    int          `json:"fuel"`
	FuelIgnoredWhenCharging bool         `json:"fuel_ignored_when_charging"`
	Ignition                string       `json:"ignition"`
	Keyfob                  string       `json:"keyfob"`
	MustBeCharging          bool         `json:"must_be_charging"`
	SeatBox                 interface{}  `json:"seat_box"`
	TopBox                  interface{}  `json:"top_box"`
}

// ReturnArea defines sub‐areas where the car can be returned, each with a center, etc.
type ReturnArea struct {
	Center       ReturnAreaCenter `json:"center"`
	HintExamples string           `json:"hint_examples"`
	HintRequired bool             `json:"hint_required"`
	MaxDistance  int              `json:"max_distance"`
	Polygon      interface{}      `json:"polygon"`
}

// ReturnAreaCenter is a smaller location-like object.
type ReturnAreaCenter struct {
	ID                    int         `json:"id"`
	Kind                  interface{} `json:"kind"`
	CapacityCurrentlyFree int         `json:"capacity_currently_free"`
	CapacityMax           int         `json:"capacity_max"`
	CenterLat             float64     `json:"center_lat"`
	CenterLng             float64     `json:"center_lng"`
	City                  string      `json:"city"`
	Lat                   float64     `json:"lat"`
	Lng                   float64     `json:"lng"`
	Name                  string      `json:"name"`
	NavigationalLat       float64     `json:"navigational_lat"`
	NavigationalLng       float64     `json:"navigational_lng"`
	PickupDescription     interface{} `json:"pickup_description"`
	Polygon               interface{} `json:"polygon"`
	Postcode              string      `json:"postcode"`
	Radius                int         `json:"radius"`
	ReturnDescription     interface{} `json:"return_description"`
	Street                string      `json:"street"`
}

// RFIDSlot is used by rfid_slot_1 and rfid_slot_2
type RFIDSlot struct {
	Kind   string  `json:"kind"`
	Label  *string `json:"label"`
	Pin    *string `json:"pin"`
	Vendor *string `json:"vendor"`
}

// VehicleModel points to a CarManufacturer.
type VehicleModel struct {
	ID              int             `json:"id"`
	ModelName       string          `json:"model_name"`
	Name            string          `json:"name"`
	CarType         *string         `json:"car_type"`
	CarManufacturer CarManufacturer `json:"car_manufacturer"`
}

// CarManufacturer has an ID and a name (e.g., "VW").
type CarManufacturer struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// VehicleCategories has an ID and a name (e.g., "camioncino").
type VehicleCategories struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}
