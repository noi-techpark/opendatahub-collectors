// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package com.opendatahub.transformer.parking.offstreet.meranobolzano;

import java.util.ArrayList;
import java.util.List;
import java.util.Map;
import java.util.stream.Collectors;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.stereotype.Service;

import com.fasterxml.jackson.core.type.TypeReference;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.opendatahub.timeseries.bdp.dto.dto.DataMapDto;
import com.opendatahub.timeseries.bdp.dto.dto.RecordDtoImpl;
import com.opendatahub.timeseries.bdp.dto.dto.SimpleRecordDto;
import com.opendatahub.timeseries.bdp.dto.dto.StationDto;
import com.opendatahub.timeseries.bdp.dto.dto.StationList;
import com.opendatahub.transformer.parking.offstreet.meranobolzano.dto.ParkingBolzanoDto;

@Service
public class ParkingBolzano {
    private final static Logger log = LoggerFactory.getLogger(ParkingBolzano.class);

    @Value("${bolzano.origin}")
    public String origin;
    
    @Value("${bolzano.period}")
    private Integer period;

	private static final String OCCUPIED_TYPE = "occupied";

    @Autowired
    private ObjectMapper mapper;
    
    private StationDto mapStation(ParkingBolzanoDto dto) {
        var stationDto = new StationDto();
        var metaDataParkingPlace = dto.metadata;
        stationDto.setId(metaDataParkingPlace.get(0).toString());
        stationDto.setName(metaDataParkingPlace.get(1).toString());
        stationDto.getMetaData().put("capacity", Integer.valueOf(metaDataParkingPlace.get(2).toString()));
        stationDto.getMetaData().put("municipality", "Bolzano - Bozen");
        stationDto.setOrigin(origin);
        return stationDto;
    }

    public StationList mapStations(Map<String, ParkingBolzanoDto> dto){
        return dto.values().stream()
            .map(this::mapStation)
            .collect(Collectors.toCollection(StationList::new));
    }

    public Map<String, ParkingBolzanoDto> deserializeJson(String json) throws Exception {
        return mapper.readValue(json, new TypeReference<Map<String, ParkingBolzanoDto>>(){});
    }
    
    private boolean bool(Object o){
        return ((Integer) o) == 1;
    }
    
    public DataMapDto<RecordDtoImpl> mapRecords(Map<String, ParkingBolzanoDto> dtos){
        var stationsMap = new DataMapDto<>();

        for (ParkingBolzanoDto dto : dtos.values()) {
            StationDto parkingMetaData = mapStation(dto);
            var records = new ArrayList<RecordDtoImpl>();

            var dtoRecord = dto.data;
            if (dtoRecord != null && dtoRecord.size() >= 15) {
                boolean communicationState = bool(dtoRecord.get(7));
                boolean controlUnit = bool(dtoRecord.get(9));
                boolean totalChangeAlarm = bool(dtoRecord.get(11));
                boolean inactiveAlarm = bool(dtoRecord.get(12));
                boolean occupiedSlotsAlarm = bool(dtoRecord.get(13));

                if (!(communicationState || controlUnit || totalChangeAlarm || inactiveAlarm || occupiedSlotsAlarm)) {
                    var odhRecord = new SimpleRecordDto();
                    int capacity = (Integer) parkingMetaData.getMetaData().get("capacity");
                    odhRecord.setValue(capacity - (Integer) dtoRecord.get(5));
                    odhRecord.setTimestamp((Integer) dtoRecord.get(6) * 1000l);
                    odhRecord.setPeriod(period);
                    records.add(odhRecord);

                    var dataMap = new DataMapDto<>();
                    dataMap.setData(records);

                    var typeMap = new DataMapDto<>();
                    typeMap.getBranch().put(OCCUPIED_TYPE, dataMap);

                    stationsMap.getBranch().put(parkingMetaData.getId(), typeMap);
                }
            }
        }
        return stationsMap;
    }
}
