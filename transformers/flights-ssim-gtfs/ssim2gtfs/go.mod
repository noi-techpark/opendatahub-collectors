module opendatahub.com/ssim2gtfs

go 1.25.5

require (
	github.com/umahmood/haversine v0.0.0-20151105152445-808ab04add26
	gotest.tools/v3 v3.5.2
)

require github.com/zsefvlol/timezonemapper v1.0.0

require (
	github.com/google/go-cmp v0.5.9 // indirect
	github.com/patrickbr/gtfsparser v0.0.0-20240911102057-fc74d7141f00
	github.com/patrickbr/gtfswriter v0.0.0-20241126214321-b6c6255581e4
)

require (
	github.com/klauspost/compress v1.17.7 // indirect
	github.com/valyala/fastjson v1.6.4 // indirect
	opendatahub.com/ssimparser v0.0.0-00010101000000-000000000000
)

replace opendatahub.com/ssimparser => ../ssimparser
