// SPDX-FileCopyrightText: NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package com.opendatahub.dc.echarging;

import java.util.Date;
import java.util.List;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.stereotype.Service;

import com.fasterxml.jackson.core.type.TypeReference;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.opendatahub.dc.echarging.dto.ChargerDtoV2;
import com.opendatahub.transformer.lib.listener.MongoService;
import com.opendatahub.transformer.lib.listener.MsgDto;
import com.opendatahub.transformer.lib.listener.RawDto;
import com.opendatahub.transformer.lib.listener.TransformerListener;

import com.opendatahub.timeseries.bdp.dto.dto.DataMapDto;
import com.opendatahub.timeseries.bdp.dto.dto.DataTypeDto;
import com.opendatahub.timeseries.bdp.dto.dto.RecordDtoImpl;
import com.opendatahub.timeseries.bdp.dto.dto.StationList;

@Service
public class Transformer {
    private static final Logger log = LoggerFactory.getLogger(Transformer.class);

    private static final int DATA_CHUNK_SIZE = 1000;
    private static final int STATION_CHUNK_SIZE = 50;

    @Autowired
    private ObjectMapper mapper;

    @Autowired
    private ChargePusher pusher;

    @Autowired
    private MongoService mongo;
    
    @TransformerListener
    public void listen(String msgPayload) throws Exception {
        MsgDto msg = mapper.readValue(msgPayload, MsgDto.class); 
        log.debug("received new event: {}", msg);
        RawDto raw = mongo.getRaw(msg.db, msg.collection, msg.id);
        log.debug("Raw data from db: {}", raw);
        syncAll(raw);
        log.debug("All done");
    }

    public void syncAll(RawDto raw) throws Exception{
        log.info("Sync: Fetching from source");
        List<ChargerDtoV2> fetchedStations = mapper.readValue(raw.getRawdataString(), new TypeReference<List<ChargerDtoV2>>(){});
        syncStationsV2(fetchedStations);
        syncDataTypes();
        pushChargerDataV2(raw.getTimestamp(), fetchedStations);
    }

    public void syncStationsV2(List<ChargerDtoV2> fetchedStations) {
        log.info("Sync Stations and Plugs");

        StationList stations = pusher.mapStations2bdp(fetchedStations);
        StationList plugs = pusher.mapPlugsStations2Bdp(fetchedStations);
        log.info(
            "Sync Stations and Plugs: Pushing {} stations and {} plugs to the writer",
            stations == null ? 0 : stations.size(),
            plugs == null ? 0 : plugs.size()
        );

        if (stations != null && plugs != null) {
            pusher.syncStations(stations, STATION_CHUNK_SIZE);
            pusher.syncStations("EChargingPlug", plugs, STATION_CHUNK_SIZE);
        }

        log.info("Sync Stations and Plugs: Done");
    }

    public void pushChargerDataV2(Date timestamp, List<ChargerDtoV2> fetchedStations) {
        log.info("Sync Charger Data");
        int chunks = (int) Math.ceil((float) fetchedStations.size() / DATA_CHUNK_SIZE);
        log.info(
            "Sync Charger Data: Found {} stations. Splitting into {} chunks of max. {} each!",
            fetchedStations.size(),
            chunks,
            DATA_CHUNK_SIZE
        );

        for (int i = 0; i < chunks; i++) {
            // We have the following interval boundaries for subList: [from, to)
            int from = DATA_CHUNK_SIZE * i;
            int to = from + DATA_CHUNK_SIZE;
            if (to > fetchedStations.size())
                to = fetchedStations.size();
            List<ChargerDtoV2> stationChunk = fetchedStations.subList(from, to);
            DataMapDto<RecordDtoImpl> map = pusher.mapData(timestamp, stationChunk);
            DataMapDto<RecordDtoImpl> plugRec = pusher.mapPlugData2Bdp(timestamp, stationChunk);
            log.info("Sync Charger Data: Pushing to the writer: Chunk {} of {}", i+1, chunks);
            if (map != null && plugRec != null){
                pusher.pushData(map);
                pusher.pushData("EChargingPlug", plugRec);
            }
        }
        log.info("Sync Charger Data: Fetching from source and parsing: Done");
    }

    public void syncDataTypes() {
        List<DataTypeDto> types = pusher.getDataTypes();
        if (types != null){
            pusher.syncDataTypes("EChargingPlug",types);
        }
        log.info("Sync Data Types: DONE!");
    }
}
