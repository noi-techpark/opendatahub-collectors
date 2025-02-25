// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

type FacilityData []Facility

type Facility struct {
	IdCompany       int
	FacilityId      int
	FacilityID      int
	Description     string
	City            string
	Address         string
	ZIPCode         string
	Telephone1      string
	Telephone2      string
	PostNumber      int
	ReceiptMerchant string
	Web             string
	Latitude        float64
	Longitude       float64
	FacilityDetails []FreePlace
}

func (f Facility) GetID() int {
	if f.FacilityID != 0 {
		return f.FacilityID
	}
	return f.FacilityId
}

type FreePlace struct {
	FacilityId          int
	FacilityDescription string
	ParkNo              int
	CountingCategoryNo  int
	CountingCategory    string
	FreeLimit           int
	OccupancyLimit      int
	CurrentLevel        int
	Reservation         int
	Capacity            int
	FreePlaces          int
	Latitude            float64
	Longitude           float64
}
