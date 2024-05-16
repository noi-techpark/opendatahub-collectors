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
    private static final String PROP_HTTP_HEADERS = "HTTP_HEADERS_";

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
                        .filter(k -> k.startsWith(PROP_HTTP_HEADERS))
                        .forEach(k -> {
                            msg.setHeader(k.substring(PROP_HTTP_HEADERS.length()), env.get(k));
                        });
                })
                .to(httpEndpoint) // actual http call
                // remove all readers to avoid sending them to message queue
                .removeHeaders("*")
                .to("direct:mq")
                .end();
    }
}
