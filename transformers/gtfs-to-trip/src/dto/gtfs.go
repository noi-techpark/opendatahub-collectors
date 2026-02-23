// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: MPL-2.0

package dto

type Agency struct {
	AgencyID       string `csv:"agency_id"`
	AgencyName     string `csv:"agency_name"`
	AgencyURL      string `csv:"agency_url"`
	AgencyTimezone string `csv:"agency_timezone"`
	AgencyLang     string `csv:"agency_lang"`
}

type Stop struct {
	StopID       string `csv:"stop_id"`
	StopName     string `csv:"stop_name"`
	StopLat      string `csv:"stop_lat"`
	StopLon      string `csv:"stop_lon"`
	StopURL      string `csv:"stop_url"`
	StopTimezone string `csv:"stop_timezone"`
}

type Route struct {
	RouteID           string `csv:"route_id"`
	RouteLongName     string `csv:"route_long_name"`
	RouteShortName    string `csv:"route_short_name"`
	AgencyID          string `csv:"agency_id"`
	RouteType         string `csv:"route_type"`
	RouteSortOrder    string `csv:"route_sort_order"`
	ContinuousPickup  string `csv:"continuous_pickup"`
	ContinuousDropOff string `csv:"continuous_drop_off"`
}

type Calendar struct {
	ServiceID string `csv:"service_id"`
	Monday    string `csv:"monday"`
	Tuesday   string `csv:"tuesday"`
	Wednesday string `csv:"wednesday"`
	Thursday  string `csv:"thursday"`
	Friday    string `csv:"friday"`
	Saturday  string `csv:"saturday"`
	Sunday    string `csv:"sunday"`
	StartDate string `csv:"start_date"`
	EndDate   string `csv:"end_date"`
}

type Trip struct {
	RouteID       string `csv:"route_id"`
	ServiceID     string `csv:"service_id"`
	TripHeadsign  string `csv:"trip_headsign"`
	TripShortName string `csv:"trip_short_name"`
	DirectionID   string `csv:"direction_id"`
	ShapeID       string `csv:"shape_id"`
	TripID        string `csv:"trip_id"`
}

type StopTime struct {
	TripID            string `csv:"trip_id"`
	ArrivalTime       string `csv:"arrival_time"`
	DepartureTime     string `csv:"departure_time"`
	StopID            string `csv:"stop_id"`
	StopSequence      string `csv:"stop_sequence"`
	PickupType        string `csv:"pickup_type"`
	DropOffType       string `csv:"drop_off_type"`
	ContinuousPickup  string `csv:"continuous_pickup"`
	ContinuousDropOff string `csv:"continuous_drop_off"`
	ShapeDistTraveled string `csv:"shape_dist_traveled"`
}

type ShapePoint struct {
	ShapeID           string `csv:"shape_id"`
	ShapePtLat        string `csv:"shape_pt_lat"`
	ShapePtLon        string `csv:"shape_pt_lon"`
	ShapePtSequence   string `csv:"shape_pt_sequence"`
	ShapeDistTraveled string `csv:"shape_dist_traveled"`
}
