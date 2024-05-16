package com.opendatahub.transformer.spreadsheet.google.util;

import java.util.ArrayList;
import java.util.List;

import org.springframework.stereotype.Component;

import com.google.api.services.sheets.v4.model.CellData;
import com.google.api.services.sheets.v4.model.GridData;
import com.google.api.services.sheets.v4.model.RowData;
import com.google.api.services.sheets.v4.model.Sheet;

@Component
public class SheetUtil {
    public List<List<Object>> sheetAsList(Sheet sheet){
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
}
