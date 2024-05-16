package com.opendatahub.transformer.spreadsheet.google;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;
import org.springframework.context.annotation.ComponentScan;

import com.fasterxml.jackson.databind.ObjectMapper;
import com.opendatahub.transformer.lib.listener.MongoService;
import com.opendatahub.transformer.lib.listener.MsgDto;
import com.opendatahub.transformer.lib.listener.TransformerListener;
import com.opendatahub.transformer.spreadsheet.google.util.Decoder;

@ComponentScan({"com.opendatahub.transformer.spreadsheet.google", "com.opendatahub.timeseries.bdp"})
@SpringBootApplication
public class Main {
    private Logger logger = LoggerFactory.getLogger(Main.class);

    @Autowired
    private ObjectMapper objectMapper;

    @Autowired
    private MongoService mongo;
    
    @Autowired
    private Decoder decode;
    
    @Autowired
    private SpreadsheetTransformer collector;

    public static void main(String[] args) {
        SpringApplication.run(Main.class, args);
    }

    @TransformerListener
    public void listen(String msgPayload) throws Exception {
        MsgDto msg = objectMapper.readValue(msgPayload, MsgDto.class); 
        logger.debug("Received new event: {}", msg);
        String raw = mongo.getRawPayload(msg.db, msg.collection, msg.id);
        logger.trace("Raw payload from db: {}", raw);

        String rawSheet = decode.decodePayload(raw);

        logger.trace("Decoded payload: {}", rawSheet);

        collector.syncData(rawSheet);
        logger.debug("All done");
    }
}
