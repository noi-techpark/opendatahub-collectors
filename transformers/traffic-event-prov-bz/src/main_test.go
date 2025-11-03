// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

// func Test1(t *testing.T) {
// 	var err error
// 	location, err = time.LoadLocation(PROVIDER_TIMEZONE)
// 	tags, err = ReadTags("../resources/tags.json")
// 	annCache = map[string]announcementCache{}
// 	contentClient, err = odhContentClient.NewContentClient(odhContentClient.Config{
// 		BaseURL:      "http://0.0.0.0",
// 		TokenURL:     env.ODH_CORE_TOKEN_URL,
// 		ClientID:     env.ODH_CORE_TOKEN_CLIENT_ID,
// 		ClientSecret: env.ODH_CORE_TOKEN_CLIENT_SECRET,
// 		DisableOAuth: env.ODH_CORE_TOKEN_URL == "",
// 	})

// 	var in = []dto.TrafficEvent{}
// 	err = bdpmock.LoadInputData(&in, "testdata/in.json")
// 	require.Nil(t, err)

// 	timestamp, err := time.Parse("2006-01-02", "2025-01-01")
// 	require.Nil(t, err)

// 	raw := rdb.Raw[[]dto.TrafficEvent]{
// 		Rawdata:   in,
// 		Timestamp: timestamp,
// 	}

// 	err = Transform(context.TODO(), &raw)
// }
