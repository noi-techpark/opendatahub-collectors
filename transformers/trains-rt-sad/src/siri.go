// SPDX-FileCopyrightText: 2026 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"encoding/xml"
)

type Siri struct {
	Version           string `xml:"version,attr"`
	ServiceDelivery   ServiceDelivery
	XMLName           xml.Name `json:"-" xml:"Siri"`
	NsNetex           string   `json:"-" xml:"xmlns,attr"`
	NsXsi             string   `json:"-" xml:"xmlns:xsi,attr"`
	XsiSchemaLocation string   `json:"-" xml:"xsi:schemaLocation,attr"`
}

func NewSiri() Siri {
	siri := Siri{}
	siri.Version = "2.1"
	siri.NsNetex = "http://www.siri.org.uk/siri"
	siri.NsXsi = "http://www.w3.org/2001/XMLSchema-instance"
	siri.XsiSchemaLocation = "http://www.siri.org.uk/siri"

	return siri
}

type DeliveryThingy struct {
	ResponseTimestamp string
	ProducerRef       string
}

type ServiceDelivery struct {
	DeliveryThingy
	VehicleMonitoringDelivery VehicleMonitoringDelivery
}

type VehicleMonitoringDelivery struct {
	DeliveryThingy
	VehicleActivity []VehicleActivity
}

type VehicleActivity struct {
	RecordedAtTime          string
	ValidUntilTime          string
	VehicleMonitoringRef    string
	MonitoredVehicleJourney struct {
		LineRef                 string
		DirectionRef            string
		FramedVehicleJourneyRef struct {
			DataFrameRef           string
			DatedVehicleJourneyRef string
		}
		PublishedLineName  string
		DirectionName      string
		OperatorRef        string
		ProductCategoryRef string
		Monitored          bool
		InCongestion       bool
		VehicleLocation    struct {
			Longitude float32
			Latitude  float32
		}
		Delay      string
		VehicleRef string
	}
}
