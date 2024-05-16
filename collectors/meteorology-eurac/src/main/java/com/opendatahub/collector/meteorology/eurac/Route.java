package com.opendatahub.collector.meteorology.eurac;

import org.apache.camel.builder.RouteBuilder;
import org.springframework.stereotype.Component;

@Component
public class Route extends RouteBuilder {

    private String stationsUrl = "https://edp-portal.eurac.edu/envdb/metadata";
    private String dailyUrl = "https://edp-portal.eurac.edu/envdb/climate_daily?id=eq.%STATION_ID%&select=date,tmin,tmax,tmean,prec";
    private String monthlyUrl = "https://edp-portal.eurac.edu/envdb/climatologies?order=id";

    private String odhStations = "https://mobility.api.dev.testingmachine.eu/v2/flat,node/MeteoStation?where=and(sorigin.eq.EURAC,sactive.eq.true)&select=smetadata.id";

    @Override
    public void configure() {
        // stations
        from("cron:stations?schedule={{env:CRON_STATIONS}}")
                .routeId("meteorology.eurac.stations")
                .to(stationsUrl)
                .removeHeaders("*")
                .process(e -> {
                    log.info("Stations...");
                    e.getMessage().setHeader("route_key", "stations");
                })
                .to("direct:mq");

        // monthly
        from("cron:monthly?schedule={{env:CRON_MONTHLY}}")
                .routeId("meteorology.eurac.monthly")
                .to(monthlyUrl)
                .removeHeaders("*")
                .process(e -> {
                    log.info("Monthly...");
                    e.getMessage().setHeader("route_key", "monthly");
                })
                .to("direct:mq");

        // // daily
        // from("cron:tab?schedule={{env:CRON_DAILY}}")
        // .routeId("meteorology.eurac.daily")
        // .to(odhStations)
        // .to(dailyUrl)
        // .removeHeaders("*")
        // .to("direct:mq");
    }
}
