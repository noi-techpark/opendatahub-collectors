package crawler

import (
	"testing"

	"github.com/stretchr/testify/require"

	crawler_testing "opendatahub.com/multi-rest-poller/crawler/testing"
)

func TestExample2(t *testing.T) {
	mockTransport := crawler_testing.NewMockRoundTripper(map[string]string{
		"https://www.onecenter.info/api/DAZ/GetFacilities":                    "testdata/example2/facilities_1.json",
		"https://www.onecenter.info/api/DAZ/FacilityFreePlaces?FacilityID=2":  "testdata/example2/facility_id_2.json",
		"https://www.onecenter.info/api/DAZ/FacilityFreePlaces?FacilityID=s3": "testdata/example2/facility_id_s3.json",
		"https://www.onecenter.info/api/DAZ/FacilityFreePlaces?FacilityID=s4": "testdata/example2/facility_id_s4.json",
		"https://www.onecenter.info/api/DAZ/Locations/l1":                     "testdata/example2/location_id_l1.json",
		"https://www.onecenter.info/api/DAZ/Locations/l2":                     "testdata/example2/location_id_l2.json",
		"https://www.onecenter.info/api/DAZ/Locations/l3":                     "testdata/example2/location_id_l3.json",
	})

	craw := NewApiCrawler("testing/example2.yaml")
	craw.SetClientRoundTripper(mockTransport)

	err := craw.Run()
	require.Nil(t, err)
}
