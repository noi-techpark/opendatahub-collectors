// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: MPL-2.0

package dto

type Root struct {
	RoadWorks []A22OpendataEvent `json:"RoadWorks"`
	Traffic   []A22OpendataEvent `json:"Traffic"`
}

// A22OpendataEvent represents a traffic event from the A22 opendata feed.
// Used for both "lavori" (road works) and "traffico" (traffic) datasets.
type A22OpendataEvent struct {
	IDNotizia   string  `json:"IDNotizia"`
	Direzione   string  `json:"Direzione"`
	Icona       string  `json:"Icona"`
	Descrizione string  `json:"Descrizione"`
	KmInizio    float64 `json:"KmInizio"`
	KmFine      float64 `json:"KmFine"`
	DataInizio  string  `json:"DataInizio"`
	DataFine    *string `json:"DataFine,omitempty"`
}
