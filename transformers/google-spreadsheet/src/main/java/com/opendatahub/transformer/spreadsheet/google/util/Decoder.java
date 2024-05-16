package com.opendatahub.transformer.spreadsheet.google.util;

import java.io.ByteArrayInputStream;
import java.io.ByteArrayOutputStream;
import java.util.Base64;
import java.util.zip.GZIPInputStream;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.stereotype.Component;

@Component
public class Decoder {
    private Logger logger = LoggerFactory.getLogger(Decoder.class);

    public String decodePayload(String raw) throws Exception {
        logger.debug("Decoding payload from base64");
        try{
            var gzip = Base64.getUrlDecoder().decode(raw);

            logger.debug("Decoding payload from gzip");
            var baos = new ByteArrayOutputStream();
            new GZIPInputStream(new ByteArrayInputStream(gzip)).transferTo(baos);
            String sheet = new String(baos.toByteArray());
            return sheet;
        } catch (Exception e) {
            logger.debug("Dumping raw payload: {}", raw);
            throw e;
        }
    }
}
