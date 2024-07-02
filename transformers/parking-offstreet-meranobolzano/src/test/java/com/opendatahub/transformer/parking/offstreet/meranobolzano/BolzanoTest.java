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
@Import({ParkingBolzano.class, ObjectMapper.class})
@TestPropertySource(locations = "classpath:application.properties")
public class BolzanoTest {
    @Autowired
    ParkingBolzano bolzano;
    
    @Test
    public void metaBolzano(@Value("classpath:bolzano_payload.json") Resource json) throws Exception{
        var dto = bolzano.deserializeJson(json.getContentAsString(StandardCharsets.UTF_8));
        StationList result = bolzano.mapStations(dto);
        StationDto palasport = result.stream().filter(s -> "115".equals(s.getId())).findFirst().orElseThrow();
        assertEquals("P15 - Palasport via Resia", palasport.getName());
        assertEquals("FAMAS", palasport.getOrigin());
        assertEquals("Bolzano - Bozen", palasport.getMetaData().get("municipality"));
        assertEquals(425, palasport.getMetaData().get("capacity"));
    }

    @Test
    public void dataBolzano(@Value("classpath:bolzano_payload.json") Resource json) throws Exception{
        var dto = bolzano.deserializeJson(json.getContentAsString(StandardCharsets.UTF_8));
        var mapped = bolzano.mapRecords(dto);
        var rec = (SimpleRecordDto) mapped.getBranch().get("115").getBranch().get("occupied").getData().get(0);
        assertEquals(300, rec.getPeriod());
        assertEquals(233, rec.getValue());
    }
}
