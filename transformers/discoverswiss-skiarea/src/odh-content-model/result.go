// SPDX-FileCopyrightText: 2026 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: MPL-2.0

package odhContentModel

// TransformResult represents the complete transformation result
// containing a SkiArea, associated POIs, and weather measuring points
type TransformResult struct {
	SkiArea         SkiArea            `json:"SkiArea"`
	POI             []ODHActivityPoi   `json:"POI"`
	Measuringpoints []MeasuringpointV2 `json:"Measuringpoints,omitempty"`
}
