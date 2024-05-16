package com.opendatahub.transformer.parking.offstreet.meranobolzano;

import java.util.HashMap;
import java.util.Map;
import java.util.stream.Collectors;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.amqp.rabbit.annotation.RabbitListener;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.stereotype.Component;

import com.google.api.services.sheets.v4.model.Spreadsheet;
import com.opendatahub.timeseries.bdp.dto.dto.StationList;
import com.opendatahub.transformer.lib.listener.MongoService;
import com.opendatahub.transformer.lib.listener.MsgDto;

import jakarta.annotation.PostConstruct;


@Component
public class Listener {
    private Logger log = LoggerFactory.getLogger(Listener.class);
    
    @Autowired
    private ODHClient odh;
    
    @Autowired
    private MongoService mongoService;
    
    @Autowired
    private SheetMetadataMapper metaMapper;
    
    @Autowired
    private ParkingMerano parkingMerano;

    @Autowired
    private ParkingBolzano parkingBolzano;
    
    private StationList existingStations;

    @PostConstruct
    public void init() {
        // get station data from cache
        odh.syncDataTypes(DataTypes.getDataTypeList());
        existingStations = getExistingStationsFromOdh();
    }
    
    private StationList getExistingStationsFromOdh() {
        StationList odhStations = new StationList();
        odhStations.addAll(odh.getStations(parkingMerano.origin));
        odhStations.addAll(odh.getStations(parkingBolzano.origin));
        return odhStations;
    }
    
    private void mergeAndSyncStations(StationList stations) {
        synchronized (odh) {
            var enrichedMeta = existingStations.stream()
                .collect(Collectors.toMap(s -> s.getId(), s -> s.getMetaData()));

            stations.forEach(station -> {
                var existing = enrichedMeta.getOrDefault(station.getId(), Map.of());

                Map<String, Object> combined = new HashMap<>();
                combined.putAll(existing);
                combined.putAll(station.getMetaData());

                station.setMetaData(combined);

            });

            odh.syncStations(stations);
        }
    }

    @RabbitListener(queues = "${mq.meta.queue}")
    public void listenMeta(MsgDto msg) throws Exception {
        log.debug("[meta] received msg: {}", msg );
        
        Spreadsheet metaSheet = metaMapper.deserializeJson(
            metaMapper.decodeGzip64(
                mongoService.getRawPayload(msg.db, msg.collection, msg.id)));

        synchronized (odh){
            StationList newExistingStations = getExistingStationsFromOdh();
            metaMapper.mapMetaData(metaSheet, newExistingStations);
            odh.syncStations(newExistingStations);
            existingStations = newExistingStations;
        }
    }

    @RabbitListener(queues = "${mq.merano.queue}")
    public void listenMerano(MsgDto msg) throws Exception {
        log.debug("[merano] received msg: {}", msg );
        
        String json = mongoService.getRawPayload(msg.db, msg.collection, msg.id);
        var meranoStations = parkingMerano.deserializeJson(json);
        
        var odhStations = parkingMerano.mapStations(meranoStations);
        mergeAndSyncStations(odhStations);

        var odhRecords = parkingMerano.mapRecords(meranoStations);
        odh.pushData(odhRecords, parkingMerano.origin);
    }

    @RabbitListener(queues = "${mq.bolzano.queue}")
    public void listenBolzano(MsgDto msg) throws Exception {
        log.debug("[bolzano] received msg: {}", msg );

        String json = mongoService.getRawPayload(msg.db, msg.collection, msg.id);
        var bolzanoDto = parkingBolzano.deserializeJson(json);
        
        var odhStations = parkingBolzano.mapStations(bolzanoDto);
        mergeAndSyncStations(odhStations);

        var odhRecords = parkingBolzano.mapRecords(bolzanoDto);
        odh.pushData(odhRecords, parkingBolzano.origin);
    }
}
