package com.opendatahub.transformer.spreadsheet.google;

public interface SpreadsheetTransformer {

    public void syncData(String raw) throws Exception;

}