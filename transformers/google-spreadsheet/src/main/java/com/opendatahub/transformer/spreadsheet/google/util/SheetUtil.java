// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package com.opendatahub.transformer.spreadsheet.google.util;

import java.util.ArrayList;
import java.util.List;

import org.apache.commons.lang3.StringUtils;
import org.springframework.stereotype.Component;

import com.google.api.services.sheets.v4.model.CellData;
import com.google.api.services.sheets.v4.model.GridData;
import com.google.api.services.sheets.v4.model.RowData;
import com.google.api.services.sheets.v4.model.Sheet;

@Component
public class SheetUtil {
    public List<List<Object>> sheetAsList(Sheet sheet) {
        List<List<Object>> values = new ArrayList<>();

        for (GridData gridData : sheet.getData()) {
            for (RowData rowData : gridData.getRowData()) {
                List<Object> row = new ArrayList<>();
                boolean isEmpty = true;
                for (CellData cellData : rowData.getValues()) {
                    String val = cellData.getFormattedValue();
                    if (!StringUtils.isBlank(val)) {
                        isEmpty = false;
                    }
                    row.add(val);
                }
                if (!isEmpty) {
                    values.add(row);
                }
            }
        }

        return values;
    }
}
