-- SPDX-FileCopyrightText: NOI Techpark <digital@noi.bz.it>
--
-- SPDX-License-Identifier: AGPL-3.0-or-later

CREATE TEMPORARY TABLE temp_metadata_update AS
SELECT 
    csv.guid, 
    jsonb_build_object(
        'group', csv.group_val,
        'municipality', csv.municipality_val
    ) AS new_json
FROM (
    -- Simulating the CSV data as a subquery
    SELECT '96a42bf6-02bc-40ec-986f-e7489d8815be' AS guid, 'Posti auto riservati alle persone con disabilità' AS group_val, 'Meran - Merano' AS municipality_val
    UNION ALL SELECT '96c669eb-7a2c-4d72-8737-b73abf07232c', 'Posti auto riservati alle persone con disabilità', 'Meran - Merano'
    UNION ALL SELECT 'a5cfa0a1-62e7-4e5f-824c-b8b1847095b5', 'Posti auto riservati alle persone con disabilità', 'Meran - Merano'
    UNION ALL SELECT 'ad144abe-276a-4d6d-9ee8-58a922a1b67d', 'Posti auto riservati alle persone con disabilità', 'Meran - Merano'
    UNION ALL SELECT 'afb8e5ce-7780-489c-8e6b-feee9e22536f', 'Posti auto riservati alle persone con disabilità', 'Meran - Merano'
    UNION ALL SELECT 'b4b00bb3-908c-4c3d-a8b8-9240dd187ca5', 'Posti auto riservati alle persone con disabilità', 'Meran - Merano'
    UNION ALL SELECT 'bca78138-f841-41e3-9922-61bd0c78aa17', 'Posti auto riservati alle persone con disabilità', 'Meran - Merano'
    UNION ALL SELECT 'bcf986ba-2ff8-46c1-9ec1-d4ffea758187', 'Posti auto riservati alle persone con disabilità', 'Meran - Merano'
    UNION ALL SELECT 'd2992b6d-166b-496d-a7c1-ecd1ac8f17bd', 'Posti auto riservati alle persone con disabilità', 'Meran - Merano'
    UNION ALL SELECT 'db7936a9-f814-eb11-9fb4-0003ff1aa9e5', 'Posti auto riservati alle persone con disabilità', 'Meran - Merano'
    UNION ALL SELECT 'dc984b3b-5900-474d-9097-d3721a0f004f', 'Posti auto riservati alle persone con disabilità', 'Meran - Merano'
    UNION ALL SELECT 'e2b2fab4-80f6-4752-bcf6-066ae497fc26', 'Posti auto riservati alle persone con disabilità', 'Meran - Merano'
    UNION ALL SELECT '2bb38981-d516-4c00-89bd-1e651f308454', 'Posti auto riservati alle persone con disabilità', 'Meran - Merano'
    UNION ALL SELECT 'f893b5e1-6f49-4c00-ad64-daebdec9a331', 'Posti auto riservati alle persone con disabilità', 'Meran - Merano'
    UNION ALL SELECT 'cf38af72-af3c-47a9-a153-2053315c2194', 'Posti auto riservati alle persone con disabilità', 'Meran - Merano'
    UNION ALL SELECT 'd28b78b0-9884-4c8d-abdc-d978a86999bc', 'Posti auto riservati alle persone con disabilità', 'Meran - Merano'
    UNION ALL SELECT '388f6bc9-1fa1-464b-b2a7-34dc424de864', 'Posti auto riservati alle persone con disabilità', 'Meran - Merano'
    UNION ALL SELECT '52c6a866-99dc-4100-939f-36f80639a4b3', 'Posti auto riservati alle persone con disabilità', 'Meran - Merano'
    UNION ALL SELECT '5934eea2-4749-4292-98c3-6ae8da2943a5', 'Posti auto riservati alle persone con disabilità', 'Meran - Merano'
    UNION ALL SELECT '5aa9fce7-24fa-4987-af73-0fbd5d9bfdee', 'Posti auto riservati alle persone con disabilità', 'Meran - Merano'
    UNION ALL SELECT 'cf1e5bd0-dd2c-43d9-9bb5-2912076b40cc', 'Posti auto riservati alle persone con disabilità', 'Meran - Merano'
    UNION ALL SELECT '118dafd1-fca6-49c7-838d-7ac5a993cf1e', 'Posti auto riservati alle persone con disabilità', 'Meran - Merano'
    UNION ALL SELECT '23ff2fb3-a6a1-eb11-b566-0050f27dde46', 'Posti auto riservati alle persone con disabilità', 'Meran - Merano'
    UNION ALL SELECT '2f21684e-f5fc-4f00-bda3-a4d36ac072a1', 'Parcheggi zona stazione Bressanone', 'Bressanone'
    UNION ALL SELECT '30ab6c24-0a29-4af0-8152-328982d21d44', 'Posti auto riservati alle persone con disabilità', 'Meran - Merano'
    UNION ALL SELECT 'e3e26add-ee3d-eb11-b9ed-0050f244b601', 'Parcheggi area stazione Bolzano', 'Bolzano - Bozen'
    UNION ALL SELECT 'e7b6b734-78ad-4636-8944-596a2a1be51c', 'Parcheggi zona stazione Bressanone', 'Bressanone'
    UNION ALL SELECT 'e899e385-b517-4187-a7c9-4a67af6a47c2', 'Posti auto riservati alle persone con disabilità', 'Meran - Merano'
    UNION ALL SELECT 'e9c5555d-a7a1-eb11-b566-0050f27dde46', 'Parcheggi zona via delle Corse', 'Meran - Merano'
    UNION ALL SELECT 'ebc2096e-2aa7-4df9-8bc5-eb74a2edef1a', 'Posti auto riservati alle persone con disabilità', 'Meran - Merano'
    UNION ALL SELECT 'f2f27296-9cb1-42c2-989b-1eba37e72e98', 'Parcheggi zona Terme Merano', 'Meran - Merano'
    UNION ALL SELECT 'f7417583-e6fc-4d5f-99c5-7b136d14208d', 'Posti auto riservati alle persone con disabilità', 'Meran - Merano'
    UNION ALL SELECT '9023d6e2-a9b5-480d-aed5-3c9a2378a8ad', 'Posti auto riservati alle persone con disabilità', 'Meran - Merano'
    UNION ALL SELECT '9292d449-7652-4c5c-9750-8dee6e450c60', 'Posti auto riservati alle persone con disabilità', 'Meran - Merano'
    UNION ALL SELECT '935af00d-aa5f-eb11-9889-501ac5928d31', 'Parcheggi zona piazza Vittoria Bolzano', 'Bolzano - Bozen'
    UNION ALL SELECT '9398a35b-ef3d-eb11-b9ed-0050f244b601', 'Parcheggi zona piazza Vittoria Bolzano', 'Bolzano - Bozen'
    UNION ALL SELECT '946c034d-6caf-4cd4-9031-49536aff784b', 'Posti auto riservati alle persone con disabilità', 'Meran - Merano'
    UNION ALL SELECT 'bfdcb5c7-0b5c-4683-9e74-941d12b187b9', 'Posti auto riservati alle persone con disabilità', 'Meran - Merano'
    UNION ALL SELECT 'c5ec56a6-a2a1-eb11-b566-0050f27dde46', 'Parcheggi zona via delle Corse', 'Meran - Merano'
    UNION ALL SELECT '396e4f5b-6003-4f0c-9363-6d3f2dc70dfb', 'Posti auto riservati alle persone con disabilità', 'Meran - Merano'
    UNION ALL SELECT '4a6a4219-dc06-4414-943a-1d89d974ad0d', 'Posti auto riservati alle persone con disabilità', 'Meran - Merano'
    UNION ALL SELECT '4f6720af-86dd-4be1-8734-a3c34a3230e8', 'Posti auto riservati alle persone con disabilità', 'Meran - Merano'
    UNION ALL SELECT '00782fcd-6f2e-4213-9049-123022cf875d', 'Posti auto riservati alle persone con disabilità', 'Meran - Merano'
    UNION ALL SELECT '0129fa9e-04a8-470e-97fb-4f632fb3dace', 'Posti auto riservati alle persone con disabilità', 'Meran - Merano'
    UNION ALL SELECT '0bd4106a-ae26-4284-ae05-ff19ae202749', 'Posti auto riservati alle persone con disabilità', 'Meran - Merano'
    UNION ALL SELECT '0e57d2f4-2bcb-479f-855d-1d2dc7f42ae9', 'Posti auto riservati alle persone con disabilità', 'Meran - Merano'
    UNION ALL SELECT '10e1d6cf-15d2-4a6e-9fd0-cf0442116948', 'Posti auto riservati alle persone con disabilità', 'Meran - Merano'
    UNION ALL SELECT '269db2b5-c77d-4abb-9ae5-42b581a5715f', 'Posti auto riservati alle persone con disabilità', 'Meran - Merano'
    UNION ALL SELECT '50cbfe17-e36d-4d6d-9091-6b25bddd9ba5', 'Posti auto riservati alle persone con disabilità', 'Meran - Merano'
    UNION ALL SELECT '7776d25f-f03d-eb11-b9ed-0050f244b601', 'Parcheggi zona viale Duca D’Aosta Bolzano', 'Bolzano - Bozen'
    UNION ALL SELECT '8f9c152e-418a-4e8c-9d18-c57a21fd5f3d', 'Posti auto riservati alle persone con disabilità', 'Meran - Merano'
    UNION ALL SELECT '9005bc04-f9ed-4ff2-82cf-075cfbd19006', 'Posti auto riservati alle persone con disabilità', 'Meran - Merano'
    UNION ALL SELECT '265a95fd-4bc4-48fd-b971-b1050b5363ec', 'Posti auto riservati alle persone con disabilità', 'Meran - Merano'
    UNION ALL SELECT '7ef36a84-8955-44d2-8a78-e2ba694df095', 'Posti auto riservati alle persone con disabilità', 'Meran - Merano'
    UNION ALL SELECT '8b89e2ac-116e-404f-8378-f6fbec1a09d8', 'Posti auto riservati alle persone con disabilità', 'Meran - Merano'
    UNION ALL SELECT 'e0d545e6-a5a1-eb11-b566-0050f27dde46', 'Parcheggi zona via delle Corse', 'Meran - Merano'
    UNION ALL SELECT '7b61ad3c-a95f-eb11-9889-501ac5928d31', 'Parcheggi zona piazza Vittoria Bolzano', 'Bolzano - Bozen'
    UNION ALL SELECT 'e76fc9bc-3eff-4110-a5f3-a3ef40ae9a27', 'Posti auto riservati alle persone con disabilità', 'Meran - Merano'
) AS csv;

SELECT
    s.stationcode,
    s.origin,
    m.id AS metadata_id,
    m.json AS current_json,
    t.new_json AS proposed_json
FROM 
    station s
JOIN 
    metadata m ON m.id = s.meta_data_id
JOIN 
    temp_metadata_update t ON s.stationcode = t.guid  -- Join on the stationcode and GUID
WHERE 
    s.origin = 'systems'
    AND m.json IS DISTINCT FROM t.new_json; -- Only show records where an update is actually needed

UPDATE metadata m
SET json = t.new_json
FROM station s
JOIN temp_metadata_update t ON s.stationcode = t.guid
WHERE
    s.origin = 'systems'
    AND m.id = s.meta_data_id;