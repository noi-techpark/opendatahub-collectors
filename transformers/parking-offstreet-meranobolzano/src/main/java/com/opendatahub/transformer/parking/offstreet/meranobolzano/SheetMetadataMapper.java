// SPDX-FileCopyrightText: NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package com.opendatahub.transformer.parking.offstreet.meranobolzano;

import java.io.ByteArrayInputStream;
import java.io.ByteArrayOutputStream;
import java.io.IOException;
import java.util.ArrayList;
import java.util.Base64;
import java.util.HashMap;
import java.util.List;
import java.util.Objects;
import java.util.stream.Collectors;
import java.util.zip.GZIPInputStream;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.stereotype.Service;

import com.google.api.services.sheets.v4.model.CellData;
import com.google.api.services.sheets.v4.model.GridData;
import com.google.api.services.sheets.v4.model.RowData;
import com.google.api.services.sheets.v4.model.Sheet;
import com.google.api.services.sheets.v4.model.Spreadsheet;
import com.opendatahub.timeseries.bdp.dto.dto.StationDto;
import com.opendatahub.timeseries.bdp.dto.dto.StationList;

import jakarta.annotation.PostConstruct;

@Service
public class SheetMetadataMapper {
    private List<String> stationIdsInSheet;
    private HashMap<String, HashMap<String, String>> mapping;
    private List<Integer> metadataColumnIndexes;
    private List<String> enrichedFields;
    private static final Logger log = LoggerFactory.getLogger(SheetMetadataMapper.class);
    
    @Value("${meta.sheet.name}")
    private String sheetName;

    @PostConstruct
    private void init() {
        stationIdsInSheet = new ArrayList<>();
        mapping = new HashMap<>();
        metadataColumnIndexes = new ArrayList<>();
        enrichedFields = new ArrayList<>();
    }
    
    public Spreadsheet deserializeJson(String json) throws Exception {
        Spreadsheet spreadSheet = com.google.api.client.googleapis.util.Utils
                .getDefaultJsonFactory()
                .fromString(json, Spreadsheet.class);
        return spreadSheet;
    }
    
    public String decodeGzip64(String raw) throws Exception {
        log.debug("Decoding payload from base64");
        try{
            var gzip = Base64.getUrlDecoder().decode(raw);

            log.debug("Decoding payload from gzip");
            var baos = new ByteArrayOutputStream();
            new GZIPInputStream(new ByteArrayInputStream(gzip)).transferTo(baos);
            String sheet = new String(baos.toByteArray());
            return sheet;
        } catch (Exception e) {
            log.debug("Dumping raw payload: {}", raw);
            throw e;
        }
    }

    private List<List<Object>> sheetAsList(Sheet sheet){
        List<List<Object>> values = new ArrayList<>();
        
        for (GridData gridData : sheet.getData()) {
            for (RowData rowData : gridData.getRowData()){
                List<Object> row = new ArrayList<>();
                for (CellData cellData : rowData.getValues()){
                    row.add(cellData.getFormattedValue());
                }
                values.add(row);
            }
        }

        return values;
    }

    public void mapMetaData(Spreadsheet spreadsheet, StationList stations) throws IOException {
        resetValues();
        Sheet sheet = spreadsheet.getSheets().stream()
            .filter(s -> sheetName.equals(s.getProperties().getTitle()))
            .findFirst().get();
        List<List<Object>> rows = sheetAsList(sheet);
        List<String> headerRow = extractHeaderRow(rows);

        // initialize enriched fields
        for(int i = 3; i < headerRow.size(); i++)
            enrichedFields.add(headerRow.get(i));

        initializeMetadataColumnIndexes(headerRow);
        initializeMapping(rows, headerRow);
        
        enrichMetadata(stations);
    }

    private void initializeMetadataColumnIndexes(List<String> headerRow) {
        for (int i = 0; i < headerRow.size(); i++) {
            String header = headerRow.get(i);
            if (enrichedFields.contains(header))
                metadataColumnIndexes.add(i);
        }
    }

    private void initializeMapping(List<List<Object>> rows, List<String> headerRow ){
        for (List<Object> row : rows) {
            String id = (String) row.get(0);
            stationIdsInSheet.add(id);
            HashMap<String, String> langMapping = new HashMap<>();

            for (String enrichedField : enrichedFields) {
                int headerIndex = headerRow.indexOf(enrichedField);
                if (headerIndex >= 0 && headerIndex < row.size())
                    langMapping.put(enrichedField, (String) row.get(headerIndex));
            }
            mapping.put(id, langMapping);
        }
    }

    private List<String> extractHeaderRow(List<List<Object>> rows) {
        List<Object> headerObjectRow = rows.remove(0);
        // transform object list top string list
        return headerObjectRow.stream().map(object -> Objects.toString(object, null).trim())
                .collect(Collectors.toList());
    }

    private void enrichMetadata(StationList stations) {
        for (StationDto station : stations) {
            for (String enrichedField : enrichedFields) {
                if (mapping.get(station.getId()) != null && mapping.get(station.getId()).get(enrichedField) != null) {
                    String newMetadata = mapping.get(station.getId()).get(enrichedField);
                    station.getMetaData().put(enrichedField, newMetadata);
                }
            }
        }
    }

    private void resetValues() {
        stationIdsInSheet.clear();
        mapping.clear();
        metadataColumnIndexes.clear();
        enrichedFields.clear();
    }
}
