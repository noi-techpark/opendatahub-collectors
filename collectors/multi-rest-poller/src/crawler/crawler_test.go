package crawler

import (
	"testing"

	crawler_testing "opendatahub.com/multi-rest-poller/crawler/testing"
)

func TestExample2(t *testing.T) {
	mockTransport := crawler_testing.NewMockRoundTripper(map[string]string{
		"https://www.onecenter.info/api/DAZ/GetFacilities":                   "testdata/example2/facilities_1.json",
		"https://www.onecenter.info/api/DAZ/FacilityFreePlaces?FacilityID=2": "testdata/example2/facility_id_2.json",
	})

	craw := NewApiCrawler("testing/example2.yaml")
	craw.SetClientRoundTripper(mockTransport)

	craw.Run()
}
