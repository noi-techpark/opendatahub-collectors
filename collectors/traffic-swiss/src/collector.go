// SPDX-FileCopyrightText: 2026 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/hashicorp/go-retryablehttp"
)

// Root is the JSON payload published to RabbitMQ and consumed by the transformer.
type Root struct {
	Stations     []StationDTO     `json:"stations"`
	Measurements []MeasurementDTO `json:"measurements"`
}

// StationDTO represents a traffic sensor station.
type StationDTO struct {
	ID        string         `json:"id"`
	Lat       float64        `json:"lat"`
	Lon       float64        `json:"lon"`
	Metadata  map[string]any `json:"metadata"`
	DataTypes []string       `json:"data_types"`
}

// MeasurementDTO represents a single aggregated measurement.
type MeasurementDTO struct {
	StationID string    `json:"station_id"`
	DataType  string    `json:"data_type"`
	Value     float64   `json:"value"`
	Timestamp time.Time `json:"timestamp"`
}

// ── Static XML structs ────────────────────────────────────────────────────────
// Real DATEX II v2.3 structure (validated against opentransportdata.swiss):
//
//   <D2LogicalModel xmlns="http://datex2.eu/schema/2/2_0">
//     <payloadPublication xsi:type="dx223:MeasurementSiteTablePublication">
//       <measurementSiteTable id="OTD:TrafficData">
//         <measurementSiteRecord id="CH:0002.01">
//           <measurementSpecificCharacteristics index="11">   ← outer: has index attr
//             <measurementSpecificCharacteristics>            ← inner: has data
//               <period>60</period>
//               <specificMeasurementValueType>trafficFlow</specificMeasurementValueType>
//               <specificVehicleCharacteristics>
//                 <vehicleType>car</vehicleType>
//               </specificVehicleCharacteristics>
//             </measurementSpecificCharacteristics>
//           </measurementSpecificCharacteristics>
//           <measurementSiteLocation>...</measurementSiteLocation>
//         </measurementSiteRecord>
//       </measurementSiteTable>
//     </payloadPublication>
//   </D2LogicalModel>

type StaticFeed struct {
	XMLName xml.Name                `xml:"D2LogicalModel"`
	Sites   []MeasurementSiteRecord `xml:"payloadPublication>measurementSiteTable>measurementSiteRecord"`
}

type MeasurementSiteRecord struct {
	ID              string                   `xml:"id,attr"`
	Characteristics []IndexedCharacteristics `xml:"measurementSpecificCharacteristics"`
	Location        MeasurementSiteLocation  `xml:"measurementSiteLocation"`
}

// IndexedCharacteristics is the outer wrapper element that carries the index attribute.
type IndexedCharacteristics struct {
	Index   string                            `xml:"index,attr"`
	Details MeasurementSpecificCharacteristics `xml:"measurementSpecificCharacteristics"`
}

type MeasurementSpecificCharacteristics struct {
	Period      int    `xml:"period"`
	ValueType   string `xml:"specificMeasurementValueType"`
	VehicleType string `xml:"specificVehicleCharacteristics>vehicleType"`
}

type MeasurementSiteLocation struct {
	Lane        string  `xml:"supplementaryPositionalDescription>affectedCarriageway>lane"`
	Carriageway string  `xml:"supplementaryPositionalDescription>affectedCarriageway>carriageway"`
	Lat         float64 `xml:"pointByCoordinates>pointCoordinates>latitude"`
	Lon         float64 `xml:"pointByCoordinates>pointCoordinates>longitude"`
}

// ── Real-time XML structs ─────────────────────────────────────────────────────
// The realtime endpoint is a SOAP service. The response wraps a d2LogicalModel inside
// a SOAP envelope. After extracting the SOAP body, the structure is:
//
//   <d2LogicalModel>
//     <payloadPublication xsi:type="dx223:MeasuredDataPublication">
//       <siteMeasurements>
//         <measurementSiteReference id="CH:0002.01"/>
//         <measurementTimeDefault>2024-09-20T10:00:00Z</measurementTimeDefault>
//         <measuredValue index="11">
//           <measuredValue>
//             <basicData xsi:type="dx223:TrafficFlow">
//               <vehicleFlow>
//                 <vehicleFlowRate>300</vehicleFlowRate>
//               </vehicleFlow>
//             </basicData>
//           </measuredValue>
//         </measuredValue>
//         <measuredValue index="12">
//           <measuredValue>
//             <basicData xsi:type="dx223:TrafficSpeed">
//               <averageVehicleSpeed>
//                 <speed>112.4</speed>
//               </averageVehicleSpeed>
//             </basicData>
//           </measuredValue>
//         </measuredValue>
//       </siteMeasurements>
//     </payloadPublication>
//   </d2LogicalModel>

type SoapEnvelope struct {
	XMLName xml.Name `xml:"Envelope"`
	Body    SoapBody `xml:"Body"`
}

type SoapBody struct {
	Inner []byte `xml:",innerxml"`
}

type RealtimeFeed struct {
	SiteMeasurements []SiteMeasurement `xml:"payloadPublication>siteMeasurements"`
}

type SiteMeasurement struct {
	SiteRef     SiteMeasurementReference `xml:"measurementSiteReference"`
	TimeDefault string                   `xml:"measurementTimeDefault"`
	Values      []MeasuredValue          `xml:"measuredValue"`
}

type SiteMeasurementReference struct {
	ID string `xml:"id,attr"`
}

type MeasuredValue struct {
	Index           string  `xml:"index,attr"`
	VehicleFlowRate float64 `xml:"measuredValue>basicData>vehicleFlow>vehicleFlowRate"`
	SpeedValue      float64 `xml:"measuredValue>basicData>averageVehicleSpeed>speed"`
}

// ── SOAP request constants ────────────────────────────────────────────────────

const realtimeSoapAction = "http://opentransportdata.swiss/TDP/Soap_Datex2/Pull/v1/pullMeasuredData"

const realtimeSoapBody = `<?xml version="1.0" encoding="UTF-8"?>
<SOAP-ENV:Envelope xmlns:SOAP-ENV="http://schemas.xmlsoap.org/soap/envelope/" xmlns:dx223="http://datex2.eu/schema/2/2_0">
  <SOAP-ENV:Body>
    <dx223:d2LogicalModel modelBaseVersion="2">
      <dx223:exchange>
        <dx223:supplierIdentification>
          <dx223:country>ch</dx223:country>
          <dx223:nationalIdentifier>OTD</dx223:nationalIdentifier>
        </dx223:supplierIdentification>
      </dx223:exchange>
    </dx223:d2LogicalModel>
  </SOAP-ENV:Body>
</SOAP-ENV:Envelope>`

// ── Parsing functions ─────────────────────────────────────────────────────────

// ParseStaticXML parses the DATEX II static feed (D2LogicalModel XML).
// Returns station DTOs and a per-station index→odhDataType mapping.
func ParseStaticXML(data []byte) ([]StationDTO, map[string]map[string]string, error) {
	var feed StaticFeed
	if err := xml.Unmarshal(data, &feed); err != nil {
		return nil, nil, fmt.Errorf("unmarshal static XML: %w", err)
	}

	dtos := make([]StationDTO, 0, len(feed.Sites))
	chars := make(map[string]map[string]string)

	for _, site := range feed.Sites {
		idxMap := make(map[string]string)
		var dataTypes []string

		for _, c := range site.Characteristics {
			dt, ok := odhDataType(c.Details.ValueType, c.Details.VehicleType)
			if !ok {
				continue
			}
			idxMap[c.Index] = dt
			if !contains(dataTypes, dt) {
				dataTypes = append(dataTypes, dt)
			}
		}

		dtos = append(dtos, StationDTO{
			ID:  site.ID,
			Lat: site.Location.Lat,
			Lon: site.Location.Lon,
			Metadata: map[string]any{
				"lane":        site.Location.Lane,
				"carriageway": site.Location.Carriageway,
			},
			DataTypes: dataTypes,
		})
		chars[site.ID] = idxMap
	}

	return dtos, chars, nil
}

// ParseRealtimeXML parses the DATEX II real-time SOAP response.
func ParseRealtimeXML(data []byte) ([]SiteMeasurement, error) {
	var env SoapEnvelope
	if err := xml.Unmarshal(data, &env); err != nil {
		return nil, fmt.Errorf("unmarshal SOAP envelope: %w", err)
	}
	var feed RealtimeFeed
	if err := xml.Unmarshal(env.Body.Inner, &feed); err != nil {
		return nil, fmt.Errorf("unmarshal realtime payload: %w", err)
	}
	return feed.SiteMeasurements, nil
}

// FetchURL fetches a URL via GET using retryablehttp. bearerToken is optional.
func FetchURL(url, bearerToken string) ([]byte, error) {
	req, err := retryablehttp.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request for %s: %w", url, err)
	}
	if bearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+bearerToken)
	}

	client := retryablehttp.NewClient()
	client.RetryMax = 3
	client.Logger = nil

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected HTTP %d from %s", resp.StatusCode, url)
	}
	return io.ReadAll(resp.Body)
}

// FetchSOAP sends a SOAP 1.1 POST request and returns the raw response body.
func FetchSOAP(endpoint, soapAction, body, bearerToken string) ([]byte, error) {
	req, err := retryablehttp.NewRequest(http.MethodPost, endpoint, strings.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("creating SOAP request for %s: %w", endpoint, err)
	}
	req.Header.Set("Content-Type", "text/xml; charset=utf-8")
	req.Header.Set("SOAPAction", soapAction)
	if bearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+bearerToken)
	}

	client := retryablehttp.NewClient()
	client.RetryMax = 3
	client.Logger = nil
	client.CheckRetry = func(ctx context.Context, resp *http.Response, err error) (bool, error) {
		if resp != nil && (resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusForbidden) {
			return false, nil
		}
		return retryablehttp.DefaultRetryPolicy(ctx, resp, err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("SOAP request to %s: %w", endpoint, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected HTTP %d from %s", resp.StatusCode, endpoint)
	}
	return io.ReadAll(resp.Body)
}

// odhDataType maps a DATEX II (valueType, vehicleType) pair to an ODH data type name.
func odhDataType(valueType, vehicleType string) (string, bool) {
	m := map[string]string{
		"trafficSpeed/car":        "average-speed-light-vehicles",
		"trafficSpeed/lorry":      "average-speed-heavy-vehicles",
		"trafficSpeed/anyVehicle": "average-speed",
		"trafficFlow/car":         "average-flow-light-vehicles",
		"trafficFlow/lorry":       "average-flow-heavy-vehicles",
		"trafficFlow/anyVehicle":  "average-flow",
	}
	v, ok := m[valueType+"/"+vehicleType]
	return v, ok
}

// contains checks whether s is present in slice.
func contains(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}
