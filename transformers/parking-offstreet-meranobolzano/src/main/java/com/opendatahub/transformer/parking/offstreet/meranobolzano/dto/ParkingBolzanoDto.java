package com.opendatahub.transformer.parking.offstreet.meranobolzano.dto;

import java.util.List;

import com.fasterxml.jackson.annotation.JsonIgnoreProperties;

@JsonIgnoreProperties(ignoreUnknown = true)
public class ParkingBolzanoDto {
    public List<Object> data;
    public List<Object> metadata;

    
}
