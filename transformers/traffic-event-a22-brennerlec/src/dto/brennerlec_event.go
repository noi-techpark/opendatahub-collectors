// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: MPL-2.0

package dto

// BrennerLECEvent represents a BrennerLEC speed limit event from the A22 API.
// These are dynamic speed limits applied to highway sections.
type BrennerLECEvent struct {
	Idtratta       string  `json:"idtratta"`       // Unique section/route identifier (e.g., "T1_SUD")
	Limite         *int64  `json:"limite"`          // Speed limit in km/h
	Dataattuazione *string `json:"dataattuazione"`  // Enforcement date in A22 format "/Date(epoch_ms)/" - null if not active
}
