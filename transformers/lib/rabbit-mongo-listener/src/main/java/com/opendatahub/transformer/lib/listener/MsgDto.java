// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

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
