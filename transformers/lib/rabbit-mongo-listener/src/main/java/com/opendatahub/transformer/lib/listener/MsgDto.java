package com.opendatahub.transformer.lib.listener;

import com.fasterxml.jackson.annotation.JsonIgnoreProperties;

@JsonIgnoreProperties(ignoreUnknown = true)
public class MsgDto {
    public String id;
    public String db;
    public String collection;
    
    @Override
    public String toString(){
        return String.format("{ id: %s, db: %s, collection: %s}", id, db, collection);
    }
}
