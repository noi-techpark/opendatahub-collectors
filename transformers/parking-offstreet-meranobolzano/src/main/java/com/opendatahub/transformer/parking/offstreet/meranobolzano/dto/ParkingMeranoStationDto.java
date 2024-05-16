// SPDX-FileCopyrightText: NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package com.opendatahub.transformer.parking.offstreet.meranobolzano.dto;

import com.fasterxml.jackson.annotation.JsonIgnoreProperties;
import com.fasterxml.jackson.annotation.JsonProperty;

@JsonIgnoreProperties(ignoreUnknown = true)
public class ParkingMeranoStationDto {
    @JsonProperty("AreaName")
    public String areaName;
    @JsonProperty("CurrentDateTime")
    public String currentDateTime;
    @JsonProperty("FreeParkingSpaces")
    public Integer freeParkingSpaces;
    @JsonProperty("TotalParkingSpaces")
    public Integer totalParkingSpaces;
}