// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

// Forecast represents the main forecast data structure.
type Forecast struct {
	Info           Info           `json:"info"`
	Municipalities []Municipality `json:"municipalities"`
}

// Info contains metadata about the forecast.
type Info struct {
	Model            string `json:"model"`
	CurrentModelRun  string `json:"currentModelRun"`
	NextModelRun     string `json:"nextModelRun"`
	FileName         string `json:"fileName"`
	FileCreationDate string `json:"fileCreationDate"`

	AbsTempMin float64 `json:"absTempMin"`
	AbsTempMax float64 `json:"absTempMax"`
	AbsPrecMin float64 `json:"absPrecMin"`
	AbsPrecMax float64 `json:"absPrecMax"`
}

// Municipality represents forecast data for a specific municipality.
type Municipality struct {
	Code       string            `json:"code"`
	NameDe     string            `json:"nameDe"`
	NameIt     string            `json:"nameIt"`
	NameEn     string            `json:"nameEn"`
	NameRm     string            `json:"nameRm"`
	TempMin24  ForecastDoubleSet `json:"tempMin24"`
	TempMax24  ForecastDoubleSet `json:"tempMax24"`
	Temp3      ForecastDoubleSet `json:"temp3"`
	Ssd24      ForecastDoubleSet `json:"ssd24"`
	PrecProb3  ForecastDoubleSet `json:"precProb3"`
	PrecProb24 ForecastDoubleSet `json:"precProb24"`
	PrecSum3   ForecastDoubleSet `json:"precSum3"`
	PrecSum24  ForecastDoubleSet `json:"precSum24"`
	Symbols3   ForecastStringSet `json:"symbols3"`
	Symbols24  ForecastStringSet `json:"symbols24"`
	WindDir3   ForecastDoubleSet `json:"windDir3"`
	WindSpd3   ForecastDoubleSet `json:"windSpd3"`
}

// ForecastDoubleSet contains forecast data for a specific double type parameter.
type ForecastDoubleSet struct {
	NameDe string           `json:"nameDe"`
	NameIt string           `json:"nameIt"`
	NameEn string           `json:"nameEn"`
	NameRm string           `json:"nameRm"`
	Unit   string           `json:"unit"`
	Data   []ForecastDouble `json:"data"`
}

// ForecastStringSet contains forecast data for a specific string type parameter.
type ForecastStringSet struct {
	NameDe string           `json:"nameDe"`
	NameIt string           `json:"nameIt"`
	NameEn string           `json:"nameEn"`
	NameRm string           `json:"nameRm"`
	Unit   string           `json:"unit"`
	Data   []ForecastString `json:"data"`
}

// ForecastDouble represents a single forecast data point with a double value.
type ForecastDouble struct {
	Date  string  `json:"date"`
	Value float64 `json:"value"`
}

// ForecastString represents a single forecast data point with a string value.
type ForecastString struct {
	Date  string `json:"date"`
	Value string `json:"value"`
}

// LocationDto represents a geographic location.
type LocationDto struct {
	Lat float64 `json:"lat"`
	Lon float64 `json:"lon"`
}

// MunicipalityDto represents a minimal municipality structure.
type MunicipalityDto struct {
	ID        string  `json:"Id"`
	Name      string  `json:"Detail.de.Title"`
	Latitude  float64 `json:"Latitude"`
	Longitude float64 `json:"Longitude"`
}
