package com.opendatahub.transformer.parking.offstreet.meranobolzano;

import java.util.ArrayList;
import java.util.List;

import com.opendatahub.timeseries.bdp.dto.dto.DataTypeDto;

public class DataTypes {
    private static final String PARKINGSLOT_TYPEIDENTIFIER = "occupied";

    private static final String PARKINGSLOT_METRIC = "Count";

    private static final String TYPE_UNIT = "";

    public static List<DataTypeDto> getDataTypeList() {
        List<DataTypeDto> dataTypes = new ArrayList<>();
        dataTypes.add(new DataTypeDto(PARKINGSLOT_TYPEIDENTIFIER,TYPE_UNIT,"Occupacy of a parking area",PARKINGSLOT_METRIC));
        return dataTypes;
    }
}
