// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package com.opendatahub.transformer.parking.offstreet.meranobolzano;

import java.text.ParseException;
import java.text.SimpleDateFormat;
import java.util.ArrayList;
import java.util.Arrays;
import java.util.Date;
import java.util.List;
import java.util.stream.Collectors;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.stereotype.Service;

import com.fasterxml.jackson.databind.ObjectMapper;
import com.opendatahub.timeseries.bdp.dto.dto.DataMapDto;
import com.opendatahub.timeseries.bdp.dto.dto.RecordDtoImpl;
import com.opendatahub.timeseries.bdp.dto.dto.SimpleRecordDto;
import com.opendatahub.timeseries.bdp.dto.dto.StationDto;
import com.opendatahub.timeseries.bdp.dto.dto.StationList;
import com.opendatahub.transformer.parking.offstreet.meranobolzano.dto.ParkingMeranoStationDto;

@Service
public class ParkingMerano {
    private final static Logger log = LoggerFactory.getLogger(ParkingMerano.class);

    @Value("${merano.origin}")
    public String origin;

    @Value("${merano.period}")
    private Integer period;

    private static final String OCCUPIED_TYPE = "occupied";

    @Autowired
    private ObjectMapper mapper;

    private final static SimpleDateFormat format = new SimpleDateFormat("yyyy-MM-dd'T'HH:mm:ss");

    public ParkingMeranoStationDto[] deserializeJson(String json) throws Exception {
        return mapper.readValue(json, ParkingMeranoStationDto[].class);
    }

    public StationList mapStations(ParkingMeranoStationDto[] stationDtos) {
        return Arrays.stream(stationDtos)
                .map(dto -> {
                    StationDto stationDto = new StationDto();
                    stationDto.setId("me:" + dto.areaName.toLowerCase().replaceAll("\\s+", ""));
                    stationDto.setName(dto.areaName);
                    stationDto.getMetaData().put("capacity", dto.totalParkingSpaces);
                    stationDto.getMetaData().put("municipality", "Meran - Merano");
                    stationDto.setOrigin(origin);
                    return stationDto;
                })
                .collect(Collectors.toCollection(StationList::new));
    }

    public DataMapDto<RecordDtoImpl> mapRecords(ParkingMeranoStationDto[] dtos) {
        var odhMap = new DataMapDto<>();
        for (ParkingMeranoStationDto dto : dtos) {
            DataMapDto<RecordDtoImpl> dMap = new DataMapDto<>();
            DataMapDto<RecordDtoImpl> tMap = new DataMapDto<>();
            List<RecordDtoImpl> records = new ArrayList<RecordDtoImpl>();
            SimpleRecordDto record = new SimpleRecordDto();
            record.setValue(dto.totalParkingSpaces - dto.freeParkingSpaces);
            Date date;
            try {
                date = format.parse(dto.currentDateTime);
                record.setTimestamp(date.getTime());
            } catch (ParseException e) {
                log.error("Error parsing date!", e);
            }
            record.setPeriod(period);
            records.add(record);
            dMap.setData(records);
            if (tMap.getBranch().get(OCCUPIED_TYPE) == null)
                tMap.getBranch().put(OCCUPIED_TYPE, dMap);

            String stationKey = "me:" + dto.areaName.toLowerCase().replaceAll("\\s+", "");
            if (odhMap.getBranch().get(stationKey) == null)
                odhMap.getBranch().put(stationKey, tMap);
        }
        return odhMap;
    }
}
