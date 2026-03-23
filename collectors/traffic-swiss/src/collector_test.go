// SPDX-FileCopyrightText: 2026 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"testing"
)

// minimalStaticXML mirrors the real DATEX II v2.3 structure from opentransportdata.swiss.
// Root element is D2LogicalModel; measurementSpecificCharacteristics has a two-level nesting:
// outer element carries the index attribute, inner element carries the data.
const minimalStaticXML = `<?xml version="1.0" encoding="UTF-8"?>
<D2LogicalModel xmlns="http://datex2.eu/schema/2/2_0">
  <payloadPublication>
    <measurementSiteTable>
      <measurementSiteRecord id="CH:0002.01">
        <measurementSpecificCharacteristics index="11">
          <measurementSpecificCharacteristics>
            <period>60</period>
            <specificMeasurementValueType>trafficSpeed</specificMeasurementValueType>
            <specificVehicleCharacteristics>
              <vehicleType>car</vehicleType>
            </specificVehicleCharacteristics>
          </measurementSpecificCharacteristics>
        </measurementSpecificCharacteristics>
        <measurementSpecificCharacteristics index="21">
          <measurementSpecificCharacteristics>
            <period>60</period>
            <specificMeasurementValueType>trafficFlow</specificMeasurementValueType>
            <specificVehicleCharacteristics>
              <vehicleType>lorry</vehicleType>
            </specificVehicleCharacteristics>
          </measurementSpecificCharacteristics>
        </measurementSpecificCharacteristics>
        <measurementSiteLocation>
          <supplementaryPositionalDescription>
            <affectedCarriageway>
              <lane>lane1</lane>
              <carriageway>exitSlipRoad</carriageway>
            </affectedCarriageway>
          </supplementaryPositionalDescription>
          <pointByCoordinates>
            <pointCoordinates>
              <latitude>46.998864</latitude>
              <longitude>8.311130</longitude>
            </pointCoordinates>
          </pointByCoordinates>
        </measurementSiteLocation>
      </measurementSiteRecord>
    </measurementSiteTable>
  </payloadPublication>
</D2LogicalModel>`

// minimalRealtimeXML mirrors the real SOAP response from the opentransportdata.swiss
// SOAP endpoint. measuredValue elements have direct children vehicleFlowRate / speed.
const minimalRealtimeXML = `<?xml version="1.0" encoding="UTF-8"?>
<SOAP-ENV:Envelope xmlns:SOAP-ENV="http://schemas.xmlsoap.org/soap/envelope/">
  <SOAP-ENV:Body>
    <d2LogicalModel xmlns="http://datex2.eu/schema/2/2_0">
      <payloadPublication>
        <siteMeasurements>
          <measurementSiteReference id="CH:0002.01"/>
          <measurementTimeDefault>2024-09-20T10:00:00Z</measurementTimeDefault>
          <measuredValue index="11">
            <vehicleFlowRate>42.0</vehicleFlowRate>
          </measuredValue>
          <measuredValue index="12">
            <speed>112.4</speed>
          </measuredValue>
        </siteMeasurements>
      </payloadPublication>
    </d2LogicalModel>
  </SOAP-ENV:Body>
</SOAP-ENV:Envelope>`

func TestParseStaticXML(t *testing.T) {
	dtos, chars, err := ParseStaticXML([]byte(minimalStaticXML))
	if err != nil {
		t.Fatalf("ParseStaticXML failed: %v", err)
	}
	if len(dtos) != 1 {
		t.Fatalf("expected 1 station, got %d", len(dtos))
	}
	s := dtos[0]
	if s.ID != "CH:0002.01" {
		t.Errorf("expected ID=CH:0002.01, got %q", s.ID)
	}
	if s.Lat == 0 || s.Lon == 0 {
		t.Errorf("expected non-zero lat/lon, got %v/%v", s.Lat, s.Lon)
	}
	if s.Metadata["lane"] != "lane1" {
		t.Errorf("expected lane=lane1, got %v", s.Metadata["lane"])
	}
	if len(s.DataTypes) != 2 {
		t.Errorf("expected 2 data types, got %d: %v", len(s.DataTypes), s.DataTypes)
	}

	idxMap, ok := chars["CH:0002.01"]
	if !ok {
		t.Fatal("expected char index for CH:0002.01")
	}
	if idxMap["11"] != "average-speed-light-vehicles" {
		t.Errorf("index 11 mapping wrong: %q", idxMap["11"])
	}
	if idxMap["21"] != "average-flow-heavy-vehicles" {
		t.Errorf("index 21 mapping wrong: %q", idxMap["21"])
	}
}

func TestParseRealtimeXML(t *testing.T) {
	sms, err := ParseRealtimeXML([]byte(minimalRealtimeXML))
	if err != nil {
		t.Fatalf("ParseRealtimeXML failed: %v", err)
	}
	if len(sms) != 1 {
		t.Fatalf("expected 1 siteMeasurement, got %d", len(sms))
	}
	sm := sms[0]
	if sm.SiteRef.ID != "CH:0002.01" {
		t.Errorf("expected SiteID=CH:0002.01, got %q", sm.SiteRef.ID)
	}
	if sm.TimeDefault != "2024-09-20T10:00:00Z" {
		t.Errorf("unexpected TimeDefault: %q", sm.TimeDefault)
	}
	if len(sm.Values) != 2 {
		t.Errorf("expected 2 measured values, got %d", len(sm.Values))
	}
	if sm.Values[0].Index != "11" {
		t.Errorf("expected index=11, got %q", sm.Values[0].Index)
	}
	if sm.Values[0].VehicleFlowRate != 42.0 {
		t.Errorf("expected flow=42.0, got %v", sm.Values[0].VehicleFlowRate)
	}
	if sm.Values[1].Index != "12" {
		t.Errorf("expected index=12, got %q", sm.Values[1].Index)
	}
	if sm.Values[1].SpeedValue != 112.4 {
		t.Errorf("expected speed=112.4, got %v", sm.Values[1].SpeedValue)
	}
}

func TestOdhDataTypeMapping(t *testing.T) {
	cases := []struct {
		valueType   string
		vehicleType string
		expected    string
	}{
		{"trafficSpeed", "car", "average-speed-light-vehicles"},
		{"trafficSpeed", "lorry", "average-speed-heavy-vehicles"},
		{"trafficSpeed", "anyVehicle", "average-speed"},
		{"trafficFlow", "car", "average-flow-light-vehicles"},
		{"trafficFlow", "lorry", "average-flow-heavy-vehicles"},
		{"trafficFlow", "anyVehicle", "average-flow"},
	}
	for _, c := range cases {
		got, ok := odhDataType(c.valueType, c.vehicleType)
		if !ok {
			t.Errorf("odhDataType(%q, %q) returned ok=false", c.valueType, c.vehicleType)
			continue
		}
		if got != c.expected {
			t.Errorf("odhDataType(%q, %q) = %q, want %q", c.valueType, c.vehicleType, got, c.expected)
		}
	}

	// Unknown combination should return false
	_, ok := odhDataType("unknown", "bicycle")
	if ok {
		t.Error("expected ok=false for unknown type")
	}
}
