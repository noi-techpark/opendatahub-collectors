// SPDX-FileCopyrightText: NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package com.opendatahub.dc.echarging;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertNotNull;
import static org.junit.jupiter.api.Assertions.assertTrue;

import java.util.ArrayList;
import java.util.List;
import java.util.Map;

import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.test.context.junit4.AbstractJUnit4SpringContextTests;

import com.opendatahub.dc.echarging.dto.ChargerDtoV2;
import com.opendatahub.dc.echarging.dto.ChargingPointsDtoV2;
import com.opendatahub.dc.echarging.dto.ChargingPositionDto;
import com.opendatahub.dc.echarging.dto.OutletDtoV2;

import com.opendatahub.timeseries.bdp.dto.dto.DataMapDto;
import com.opendatahub.timeseries.bdp.dto.dto.DataTypeDto;
import com.opendatahub.timeseries.bdp.dto.dto.RecordDtoImpl;
import com.opendatahub.timeseries.bdp.dto.dto.SimpleRecordDto;
import com.opendatahub.timeseries.bdp.dto.dto.StationDto;
import com.opendatahub.timeseries.bdp.dto.dto.StationList;

public class PusherTestIT extends AbstractJUnit4SpringContextTests{

    @Autowired
    private ChargePusher pusher;

    private List<ChargerDtoV2> charger = null;

    @BeforeEach
    public void setup() {
        charger = new ArrayList<ChargerDtoV2>();
        ChargerDtoV2 o = new ChargerDtoV2();
        o.setCode("thisisacode");
        o.setCategories(new String[] {"hey","to"});
        o.setAccessType("Got to know");
        o.setAccessInfo("Kind of accessInfo");
        o.setIsOnline(true);
        ChargingPositionDto position = new ChargingPositionDto();
        position.setCity("Chicago");
        position.setAddress("Baverlz 23");
        position.setCountry("usa");
        o.setPosition(position);
        o.setIsReservable(false);
        o.setLatitude(45.2313);
        o.setLongitude(42.2313);
        o.setModel("TESLA");
        o.setPaymentInfo("INfo for payment");
        o.setProvider("Patrick");
        o.setOrigin("Unknown");
        o.setName("Hello world");
        List<ChargingPointsDtoV2> chargingPoints = new ArrayList<>();
        ChargingPointsDtoV2 cp = new ChargingPointsDtoV2();
        cp.setId("huibu");
        cp.setRechargeState("ACTIVE");
        cp.setState("ACTIVE");
        List<OutletDtoV2> outlets = new ArrayList<>();
        OutletDtoV2 out = new OutletDtoV2();
        out.setHasFixedCable(true);
        out.setId("yeah");
        out.setMaxCurrent(20.5);
        out.setMinCurrent(1.);
        out.setMaxPower(2000.);
        out.setOutletTypeCode("Outlettype");
        outlets.add(out);
        cp.setOutlets(outlets);
        chargingPoints.add(cp);
        o.setChargingPoints(chargingPoints);
        charger.add(o);

    }
    @Test
    public void testMappingData(){
        DataMapDto<RecordDtoImpl> parseData = pusher.mapData(charger);
        assertNotNull(parseData);
        for(Map.Entry<String,DataMapDto<RecordDtoImpl>> entry: parseData.getBranch().entrySet()) {
            DataMapDto<RecordDtoImpl> dataMapDto = entry.getValue().getBranch().get(DataTypeDto.NUMBER_AVAILABE);
            assertNotNull(dataMapDto);
            assertNotNull(dataMapDto.getData());
            assertFalse(dataMapDto.getData().isEmpty());
            RecordDtoImpl recordDtoImpl = dataMapDto.getData().get(0);
            assertNotNull(recordDtoImpl);
            assertTrue(recordDtoImpl instanceof SimpleRecordDto);
            SimpleRecordDto dto = (SimpleRecordDto) recordDtoImpl;
            assertNotNull(dto.getTimestamp());
            assertNotNull(dto.getValue());
        }
    }
    @Test
    public void testMappingStations() {
        StationList stationList = pusher.mapStations2bdp(charger);
        assertFalse(stationList.isEmpty());
        assertNotNull(stationList.get(0));
        assertTrue(stationList.get(0) instanceof StationDto);
        StationDto eStation=  stationList.get(0);

        assertEquals(Double.valueOf(42.2313) , eStation.getLongitude());
        assertEquals(Double.valueOf(45.2313) , eStation.getLatitude());
        assertEquals("thisisacode",eStation.getId());
    }
}
