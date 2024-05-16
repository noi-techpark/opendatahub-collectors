package com.opendatahub.transformer.parking.offstreet.meranobolzano;

import java.util.List;

import org.springframework.beans.factory.annotation.Value;
import org.springframework.stereotype.Service;

import com.opendatahub.timeseries.bdp.client.json.NonBlockingJSONPusher;
import com.opendatahub.timeseries.bdp.dto.dto.DataMapDto;
import com.opendatahub.timeseries.bdp.dto.dto.ProvenanceDto;
import com.opendatahub.timeseries.bdp.dto.dto.RecordDtoImpl;
import com.opendatahub.timeseries.bdp.dto.dto.StationDto;

@Service
public class ODHClient extends NonBlockingJSONPusher {
    @Value("${odh.stationtype}")
    private String stationType;
    @Value("${odh.provenance_name}")
    private String provenanceName;
    @Value("${odh.provenance_version}")
    private String provenanceVersion;
    @Value("${odh.origin}")
    private String defaultOrigin; 
    
    @Override
    public String initStationType() {
        return stationType;
    }

    @Override
    public ProvenanceDto defineProvenance() {
        return defineProvenance(defaultOrigin);
    }
    
    public List<StationDto> getStations(String origin){
        return this.fetchStations(stationType, origin);
    }
    
    private ProvenanceDto defineProvenance(String lineage) {
        return new ProvenanceDto(null, provenanceName, provenanceVersion, lineage);
    }

    public synchronized Object pushData(DataMapDto<? extends RecordDtoImpl> dto, String origin) {
        // Note that this is a synchronized method. 
        // This data collector has two different provenance origins,
        // so we have to swap them out before pushing data
        this.provenance = defineProvenance(origin);
        return super.pushData(dto);
    }
}
