// SPDX-FileCopyrightText: NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package com.opendatahub.transformer.spreadsheet.google;

import java.util.ArrayList;
import java.util.Date;
import java.util.List;
import java.util.function.Function;
import java.util.stream.Collectors;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.autoconfigure.condition.ConditionalOnProperty;
import org.springframework.stereotype.Service;

import com.google.api.services.sheets.v4.model.Sheet;
import com.google.api.services.sheets.v4.model.Spreadsheet;
import com.opendatahub.transformer.spreadsheet.google.dto.DataTypeWrapperDto;
import com.opendatahub.transformer.spreadsheet.google.dto.MappingResult;
import com.opendatahub.transformer.spreadsheet.google.mapper.DynamicMapper;
import com.opendatahub.transformer.spreadsheet.google.services.ODHClient;
import com.opendatahub.transformer.spreadsheet.google.util.SheetUtil;

import com.opendatahub.timeseries.bdp.dto.dto.DataMapDto;
import com.opendatahub.timeseries.bdp.dto.dto.DataTypeDto;
import com.opendatahub.timeseries.bdp.dto.dto.RecordDtoImpl;
import com.opendatahub.timeseries.bdp.dto.dto.SimpleRecordDto;
import com.opendatahub.timeseries.bdp.dto.dto.StationList;

@Service
/* This is the default implementation of a spreadsheet transformer. 
If the transformerImpl property is not set, or set to "default", this service is loaded*/
@ConditionalOnProperty(value = "transformerImpl", havingValue = "default", matchIfMissing = true)
public class GenericTransformer implements SpreadsheetTransformer {
    private Logger logger = LoggerFactory.getLogger(GenericTransformer.class);

    @Autowired
    private ODHClient odhClient;

    @Autowired
    private DynamicMapper mappingUtil;
    
    @Autowired
    private SheetUtil sheetUtil;

    public void syncData(String raw) throws Exception {
        logger.info("Start data syncronization");
        Spreadsheet fetchedSpreadSheet = com.google.api.client.googleapis.util.Utils
            .getDefaultJsonFactory()
            .fromString(raw, Spreadsheet.class);
        StationList dtos = new StationList();
        List<DataTypeWrapperDto> types = new ArrayList<DataTypeWrapperDto>();
        logger.debug("Start reading spreadsheet");
        for (Sheet sheet : fetchedSpreadSheet.getSheets()) {
            try {
                List<List<Object>> values = sheetUtil.sheetAsList(sheet);

                if (values.isEmpty() || values.get(0) == null)
                    throw new IllegalStateException("Spreadsheet " + sheet.getProperties().getTitle()
                            + " has no header row. Needs to start on top left.");
                logger.debug("Starting to map sheet using mapper: " + mappingUtil.getClass().getCanonicalName());
                MappingResult result = mappingUtil.mapSheet(values, sheet);
                if (!result.getStationDtos().isEmpty())
                    dtos.addAll(result.getStationDtos());
                if (result.getDataType() != null) {
                    types.add(result.getDataType());
                }
            } catch (Exception ex) {
                logger.error("Failed to read sheet(tab). Start reading next", ex);
                continue;
            }
        }
        if (!dtos.isEmpty()) {
            logger.debug("Syncronize stations if some where fetched and successfully parsed");
            odhClient.syncStations(dtos);
            logger.debug("Syncronize stations completed");
        }
        if (!types.isEmpty()) {
            logger.debug("Syncronize data types/type-metadata if some where fetched and successfully parsed");
            List<DataTypeDto> dTypes = types.stream().map(mapper).collect(Collectors.toList());
            odhClient.syncDataTypes(dTypes);
            logger.debug("Syncronize datatypes completed");
        }
        if (!dtos.isEmpty() && !types.isEmpty()) {
            DataMapDto<? extends RecordDtoImpl> dto = new DataMapDto<RecordDtoImpl>();
            logger.debug("Connect datatypes with stations through record");
            for (DataTypeWrapperDto typeDto : types) {
                SimpleRecordDto simpleRecordDto = new SimpleRecordDto(new Date().getTime(), typeDto.getSheetName(), 0);
                logger.trace("Connect" + dtos.get(0).getId() + "with" + typeDto.getType().getName());
                dto.addRecord(dtos.get(0).getId(), typeDto.getType().getName(), simpleRecordDto);
            }
            odhClient.pushData(dto);
        }
        logger.info("Data syncronization completed");
    }

    private Function<DataTypeWrapperDto, DataTypeDto> mapper = (dto) -> {
        return dto.getType();
    };
}
