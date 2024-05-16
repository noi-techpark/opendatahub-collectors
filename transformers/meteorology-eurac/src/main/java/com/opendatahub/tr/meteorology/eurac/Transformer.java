// SPDX-FileCopyrightText: NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package com.opendatahub.tr.meteorology.eurac;

import java.util.ArrayList;
import java.util.List;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.stereotype.Service;
import org.springframework.web.reactive.function.client.WebClientRequestException;

import com.fasterxml.jackson.databind.ObjectMapper;
import com.opendatahub.tr.meteorology.eurac.dto.ClimatologyDto;
import com.opendatahub.tr.meteorology.eurac.dto.MetadataDto;
import com.opendatahub.transformer.lib.listener.MongoService;
import com.opendatahub.transformer.lib.listener.MsgDto;
import com.opendatahub.transformer.lib.listener.TransformerListener;

import com.opendatahub.timeseries.bdp.dto.dto.DataTypeDto;
import com.opendatahub.timeseries.bdp.dto.dto.StationDto;
import com.opendatahub.timeseries.bdp.dto.dto.StationList;

@Service
public class Transformer {
    private static final Logger log = LoggerFactory.getLogger(Transformer.class);

    private static final String STATION_ID_PREFIX = "EURAC_";

    private static final String DATATYPE_ID_TMIN = "air-temperature-min";
    private static final String DATATYPE_ID_TMAX = "air-temperature-max";
    private static final String DATATYPE_ID_TMEAN = "air-temperature";
    private static final String DATATYPE_ID_PREC = "precipitation";

    @Autowired
    private ObjectMapper mapper;

    @Autowired
    private OdhClient odhClient;

    @Autowired
    private MongoService mongo;

    @TransformerListener
    public void listen(String msgPayload) throws Exception {
        log.info("Message Payload: {}", msgPayload);
        MsgDto msg = mapper.readValue(msgPayload, MsgDto.class);
        log.debug("received new event: {}", msg);
        String raw = mongo.getRawPayload(msg.db, msg.collection, msg.id);
        log.debug("Raw data from db: {}", raw);
        syncAll(raw);
    }

    public void syncAll(String raw) throws Exception {
        log.info("Sync: Fetching from source");

        MetadataDto[] euracStations = mapper.readValue(raw, MetadataDto[].class);

        List<DataTypeDto> odhDataTypeList = new ArrayList<>();

        odhDataTypeList.add(new DataTypeDto(DATATYPE_ID_TMAX, "°C", "Maximum temperature", "max"));
        odhDataTypeList.add(new DataTypeDto(DATATYPE_ID_TMEAN, "°C", "Mean temperature", "mean"));
        odhDataTypeList.add(new DataTypeDto(DATATYPE_ID_PREC, "mm", "Precipitation", "total"));

        StationList odhStationList = new StationList();
        for (MetadataDto s : euracStations) {
            if (s.getIdSource() == null) { // id_source is null, we have to create a new station
                StationDto station = new StationDto(getStationIdNOI(s), s.getName(), s.getLat(), s.getLon());

                station.setOrigin(odhClient.getProvenance().getLineage());
                station.setElevation(s.getEle());

                // As an exception, we add ID to the map because we need it for other methods,
                // It is not in the Map by default
                s.setOtherField("id", s.getId());
                station.setMetaData(s.getOtherFields());

                odhStationList.add(station);
            }
        }

        try {
            odhClient.syncStations(odhStationList);
            odhClient.syncDataTypes(odhDataTypeList);
            log.info("Cron job for stations successful");
        } catch (WebClientRequestException e) {
            log.error("Cron job for stations failed: Request exception: {}", e.getMessage());
        }
        log.info("Sync Data DONE!");
    }

    private String getStationIdNOI(MetadataDto euracStation) {
        return getStationIdNOI(euracStation.getIdSource(), euracStation.getId());
    }

    private String getStationIdNOI(ClimatologyDto climatology) {
        return getStationIdNOI(climatology.getIdSource(), climatology.getId());
    }

    private String getStationIdNOI(String stationIdSource, int stationId) {
        if (stationIdSource != null) {
            return stationIdSource;
        } else {
            return STATION_ID_PREFIX + stationId;
        }
    }
}
