// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package com.opendatahub.transformer.parking.offstreet.meranobolzano;

import static org.junit.jupiter.api.Assertions.assertEquals;

import java.nio.charset.StandardCharsets;

import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.context.annotation.Import;
import org.springframework.core.io.Resource;
import org.springframework.test.context.TestPropertySource;
import org.springframework.test.context.junit.jupiter.SpringJUnitConfig;

import com.fasterxml.jackson.databind.ObjectMapper;
import com.opendatahub.timeseries.bdp.dto.dto.SimpleRecordDto;
import com.opendatahub.timeseries.bdp.dto.dto.StationDto;
import com.opendatahub.timeseries.bdp.dto.dto.StationList;

@SpringJUnitConfig
@Import({ParkingMerano.class, ObjectMapper.class})
@TestPropertySource(locations = "classpath:application.properties")
public class MeranoTest {
    @Autowired
    ParkingMerano merano;
    
    @Test
    public void metaMerano(@Value("classpath:merano_payload.json") Resource json) throws Exception{
        var dto = merano.deserializeJson(json.getContentAsString(StandardCharsets.UTF_8));
        StationList result = merano.mapStations(dto);
        StationDto huber = result.stream().filter(s -> "me:parkhuber".equals(s.getId())).findFirst().orElseThrow();
        assertEquals(51, huber.getMetaData().get("capacity"));
    }

    @Test
    public void dataMerano(@Value("classpath:merano_payload.json") Resource json) throws Exception{
        var dto = merano.deserializeJson(json.getContentAsString(StandardCharsets.UTF_8));
        var mapped = merano.mapRecords(dto);
        var rec = (SimpleRecordDto) mapped.getBranch().get("me:parkhuber").getBranch().get("occupied").getData().get(0);
        assertEquals(300, rec.getPeriod());
        assertEquals(11, rec.getValue());
    }
}
