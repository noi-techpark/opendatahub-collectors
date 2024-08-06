// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package com.opendatahub.collector.lib.ingress.mq;

import java.net.URI;
import java.net.URISyntaxException;
import java.time.ZoneId;
import java.time.ZonedDateTime;
import java.time.format.DateTimeFormatter;
import java.util.HashMap;

import org.apache.camel.Exchange;
import org.apache.camel.component.springrabbit.SpringRabbitMQConstants;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import com.fasterxml.jackson.core.JsonProcessingException;
import com.fasterxml.jackson.databind.ObjectMapper;

public class WrapperProcessor {
    private static final Logger log = LoggerFactory.getLogger(WrapperProcessor.class);

    public static void process(final Exchange exchange, final String provider) throws JsonProcessingException {
        HashMap<String, Object> map = new HashMap<String, Object>();
        ObjectMapper objectMapper = new ObjectMapper();

        // provider has the same format as any URI.
        // it might specify query params to request some special behaviour
        // EG: mobility/tourism?fastline=true
        // EG: 'provider/collection/...&params'
        URI providerURI = null;
        Boolean validProvider = true;
        try {
            providerURI = new URI(provider).normalize();
        } catch (URISyntaxException e) {
            validProvider = false;
        }

        if (!validProvider || null == providerURI.getPath()) {
            log.error("invalid provider: "+ provider);

            // invalid provider, therefore we put the raw provider and send it anyway. it will hopefully land in deadletter queue
            map.put("provider", provider);
            exchange.getMessage().removeHeaders("*");
            exchange.getMessage().setBody(objectMapper.writeValueAsString(map));
            return;
        }

        // We start encapsulating the payload in a new message where we have
        // {provider: ..., timestamp: ..., rawdata: ...}
        // timestamp indicates when we received the message
        // provider is the provided which sent the message
        // rawdata is the data sent
        map.put("provider", providerURI.toString());
        String payload = exchange.getIn().getBody(String.class);
        map.put("rawdata", payload);
        map.put("timestamp", ZonedDateTime.now(ZoneId.of("UTC")).format(DateTimeFormatter.ISO_INSTANT));

        // setting up provider routeKey
        String routeKey = providerURI.getPath().replaceAll("/", ".");
        log.debug("routing to routeKey " +  routeKey);
        log.debug("provider " +  provider);

        exchange.getMessage().removeHeaders("*");
        exchange.getMessage().setHeader(SpringRabbitMQConstants.ROUTING_KEY, routeKey);
        exchange.getMessage().setBody(objectMapper.writeValueAsString(map));
    }
}
