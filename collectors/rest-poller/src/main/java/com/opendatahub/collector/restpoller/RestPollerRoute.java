// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package com.opendatahub.collector.restpoller;

import java.util.Map;

import org.apache.camel.Exchange;
import org.apache.camel.Message;
import org.apache.camel.PropertyInject;
import org.apache.camel.builder.RouteBuilder;
import org.apache.camel.component.http.HttpMethods;
import org.springframework.stereotype.Component;

@Component
public class RestPollerRoute extends RouteBuilder {
    @PropertyInject("env:HTTP_METHOD")
    private String httpMethod;

    @PropertyInject("env:HTTP_ENDPOINT")
    private String httpEndpoint;

    @Override
    public void configure() {
        Map<String, String> env = System.getenv();
        System.out.println(env);

        from("cron:tab?schedule={{env:CRON_SCHEDULE}}")
                .routeId("[Route: Rest poller] ")
                .process(ex -> {
                    Message msg = ex.getMessage();
                    msg.setBody(simple(null));
                    // set HTTP method from config
                    msg.setHeader(Exchange.HTTP_METHOD, HttpMethods.valueOf(httpMethod));
                    // get custom headers from config
                    env.keySet().stream()
                        .filter(k -> k.startsWith("HTTP_HEADERS_") && k.endsWith("_NAME"))
                        .map(k -> k.replaceAll("_NAME$", ""))
                        .forEach(k -> {
                            msg.setHeader(env.get(k + "_NAME"), env.get(k + "_VALUE"));
                        });
                })
                .to(httpEndpoint) // actual http call
                // remove all readers to avoid sending them to message queue
                .removeHeaders("*")
                .to("direct:mq")
                .end();
    }
}
