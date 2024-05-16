// SPDX-FileCopyrightText: NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package com.opendatahub.transformer.spreadsheet.google.services;

import org.springframework.beans.factory.annotation.Value;
import org.springframework.stereotype.Component;

import com.opendatahub.timeseries.bdp.dto.dto.DataMapDto;
import com.opendatahub.timeseries.bdp.dto.dto.ProvenanceDto;
import com.opendatahub.timeseries.bdp.dto.dto.RecordDtoImpl;
import com.opendatahub.timeseries.bdp.client.json.NonBlockingJSONPusher;

@Component
public class ODHClient extends NonBlockingJSONPusher{
	@Value(value="${stationtype}")
	private String stationtype;

	@Value("${provenance_name}")
	private String provenanceName;
	@Value("${provenance_version}")
	private String provenanceVersion;
	
    @Value("${spreadsheetId}")
    private String origin;

	@Override
	public <T> DataMapDto<RecordDtoImpl> mapData(T data) {
		return null;
	}

	@Override
	public String initStationType() {
		return stationtype;
	}

	@Override
	public ProvenanceDto defineProvenance() {
		return new ProvenanceDto(null, provenanceName, provenanceVersion, origin);
	}

}
