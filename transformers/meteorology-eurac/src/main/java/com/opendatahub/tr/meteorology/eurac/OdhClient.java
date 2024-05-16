// SPDX-FileCopyrightText: NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package com.opendatahub.tr.meteorology.eurac;

import com.opendatahub.timeseries.bdp.dto.dto.DataMapDto;
import com.opendatahub.timeseries.bdp.dto.dto.ProvenanceDto;
import com.opendatahub.timeseries.bdp.dto.dto.RecordDtoImpl;
import com.opendatahub.timeseries.bdp.client.json.NonBlockingJSONPusher;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.stereotype.Service;

@Service
public class OdhClient extends NonBlockingJSONPusher {

    @Value("${odh_client.stationtype}")
    private String stationtype;

    @Value("${odh_client.provenance.name}")
    private String provenanceName;

    @Value("${odh_client.provenance.version}")
    private String provenanceVersion;

    @Value("${odh_client.provenance.origin}")
    private String provenanceOrigin;

    @Override
    public <T> DataMapDto<RecordDtoImpl> mapData(T data) {
		/* You can ignore this legacy method call */
        return null;
    }

    @Override
    public String initStationType() {
        return stationtype;
    }

    @Override
    public ProvenanceDto defineProvenance() {
        return new ProvenanceDto(null, provenanceName, provenanceVersion, provenanceOrigin);
    }

	public ProvenanceDto getProvenance() {
		return provenance;
	}

}
