// SPDX-FileCopyrightText: NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package com.opendatahub.transformer.spreadsheet.google.dto;

import com.opendatahub.timeseries.bdp.dto.dto.DataTypeDto;

public class DataTypeWrapperDto {
    private DataTypeDto type;
    private String sheetName;
    public DataTypeWrapperDto(DataTypeDto type, String title) {
        this.type = type;
        this.sheetName = title;
    }
    public DataTypeDto getType() {
        return type;
    }
    public void setType(DataTypeDto type) {
        this.type = type;
    }
    public String getSheetName() {
        return sheetName;
    }
    public void setSheetName(String sheetName) {
        this.sheetName = sheetName;
    }
}
