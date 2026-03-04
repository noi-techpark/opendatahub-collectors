// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: MPL-2.0

package dto

// A22Event represents a single traffic event from the A22 API response.
type A22Event struct {
	Id                int64   `json:"id"`
	Idtipoevento      int64   `json:"idtipoevento"`
	Idsottotipoevento int64   `json:"idsottotipoevento"`
	Autostrada        string  `json:"autostrada"`
	Iddirezione       int64   `json:"iddirezione"`
	Idcorsia          int64   `json:"idcorsia"`
	DataInizio        string  `json:"data_inizio"`
	DataFine          *string `json:"data_fine"`
	FasciaOraria      *bool   `json:"fascia_oraria"`
	MetroInizio       int64   `json:"metro_inizio"`
	MetroFine         int64   `json:"metro_fine"`
	LatInizio         float64 `json:"lat_inizio"`
	LonInizio         float64 `json:"lon_inizio"`
	LatFine           float64 `json:"lat_fine"`
	LonFine           float64 `json:"lon_fine"`
}
