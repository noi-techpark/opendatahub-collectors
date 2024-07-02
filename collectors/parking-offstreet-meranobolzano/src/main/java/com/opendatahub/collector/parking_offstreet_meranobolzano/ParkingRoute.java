// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package com.opendatahub.collector.parking_offstreet_meranobolzano;

import org.apache.camel.builder.RouteBuilder;
import org.apache.camel.model.dataformat.JsonLibrary;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.stereotype.Component;

@Component
public class ParkingRoute extends RouteBuilder {
    @Autowired
    ParkingClient client;
    
    @Value("${cron.schedule}")
    String schedule;

    @Override
    public void configure() throws Exception {
        client.connect();
        
        from("cron:tab?schedule=" + schedule)
                .routeId("[Route: Parking bolzano poller] ")
                .process(ex -> ex.getMessage().setBody(client.getAllData()))
                .marshal().json(JsonLibrary.Jackson)
                .to("direct:mq")
                .end();
    }
}
