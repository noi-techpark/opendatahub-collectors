<!--
SPDX-FileCopyrightText: 2026 NOI Techpark <digital@noi.bz.it>

SPDX-License-Identifier: CC0-1.0
-->

# Testing pipelines

Transformer tests are **snapshot tests over recorded writer calls**, driven by `github.com/noi-techpark/opendatahub-go-sdk/testsuite` (`LoadInputData`, `LoadOutput`, `WriteOutput`) plus a mock of the write target:

- Timeseries → `github.com/noi-techpark/go-bdp-client/bdpmock`: `bdpmock.MockFromEnv(bdplib.BdpEnv{})` records every `SyncDataTypes`/`SyncStations`/`PushData` call; `bdpmock.CompareBdpMockCalls(t, expected, actual)` compares order-insensitively (records sorted by timestamp, stations by id, numbers normalized).
- Content → `clib/clibmock`: `clibmock.NewContentMock()` records every `Get`/`Post`/`Put`/`PutMultiple`; `clibmock.CompareMockCalls(t, expected, actual)`.

Inputs are provider-shaped JSON in `testdata/in*.json`; expected outputs are the recorded mock calls in `testdata/out*.json`.

**Test fixtures MUST live inside `src/`** (`src/testdata/`, loaded as `testdata/...`), never at the component root. The Dockerfile `test` stage copies only `src/` into `/code`, so `../testdata/...` paths pass locally but fail in the containerized CI test job with `open ../testdata/in.json: no such file`. (Component-root fixtures only work for the few transformers whose workflow runs tests via setup-go directly on the runner, e.g. bike-boxes — don't copy that layout.) The same applies to `resources/`: if code reads `../resources/...`, the test stage must explicitly `COPY resources/. /resources` like parking-offstreet-skidata's Dockerfile does.

## The self-bootstrapping snapshot idiom

First run writes the snapshot, subsequent runs compare — review the generated file before committing it:

```go
func TestTransform_Event1(t *testing.T) {
    var in ParkingEvent
    require.Nil(t, testsuite.LoadInputData(&in, "testdata/in1.json"))

    timestamp, err := time.Parse("2006-01-02", "2025-01-01") // FIXED time: determinism
    require.Nil(t, err)
    raw := rdb.Raw[ParkingEvent]{Rawdata: in, Timestamp: timestamp}

    b := bdpmock.MockFromEnv(bdplib.BdpEnv{})

    // Exercise the same startup flow as main(), then the hot path.
    require.Nil(t, syncDataTypes(b))
    require.Nil(t, syncAllStations(b)) // only for startup-sync transformers
    require.Nil(t, Transform(context.TODO(), b, &raw))

    req := b.(*bdpmock.BdpMock).Requests()

    var out bdpmock.BdpMockCalls
    if err := testsuite.LoadOutput(&out, "testdata/out1.json"); err != nil {
        t.Logf("No snapshot found, generating testdata/out1.json")
        require.Nil(t, testsuite.WriteOutput(req, "testdata/out1.json"))
        t.Log("Snapshot generated. Re-run the test to validate.")
        return
    }
    bdpmock.CompareBdpMockCalls(t, out, req)
}
```

Content variant: swap in `clibmock.NewContentMock()` as the content client, start from an empty `clib.NewCache[T]()`, and pin any `timeNow` package variable to a fixed date (pattern from the official transformer-from-scratch docs; the clib-based a22 reference implementations live upstream).

Determinism rules: fix `Raw.Timestamp`, fix `time.Now` via a `timeNow` variable, and never depend on map iteration order in mapped output (sort slices before returning).

## Beyond snapshots

- **Behavioral unit tests** for mapping edge cases: clamping, sentinel dates, geometry math with tolerance asserts, multi-event sequences against a primed cache. Multi-provider snapshot tests: `transformers/parking-offstreet-skidata/src/main_test.go` (`TestSkidata`, `TestMyBestParking`, `TestStations`).
- **Master-data integrity tests** whenever the transformer ships resource CSVs/JSON: load the *real* files and assert invariants — every child references an existing parent, IDs follow the expected format, no duplicates. Snapshots alone do not catch a broken CSV edit.
- **Integration tests** for servers with auth (pattern: `collectors/rest-push/test/`): a dedicated `test/docker-compose.yml` spins up the app (Dockerfile `test` target), RabbitMQ with `rabbitmq-definitions.json`, and Keycloak with a realm fixture; `run-tests.sh` brings the stack up and propagates the exit code. The workflow's `test` job runs `sh run-tests.sh`.

## Running tests

- Locally: `cd <component>/src && go test ./...`.
- Like CI: `docker run --rm $(docker build -q . -f infrastructure/docker/Dockerfile --target test)` from the component root (this also validates the resources copy paths).

## End-to-end validation

Unit and snapshot tests are not enough: before a pipeline is done it must be validated against the real platform services — data flow through every hop, idempotency on replay, restart resilience. Use the self-contained stack bundled with this skill (`assets/docker-compose.validation.yml`, all images from ghcr.io — no extra repositories needed) and follow **[validation.md](validation.md)** for the full validated procedure. After merging to `main`, verify on the testingmachine environment before promoting the `prod` branch.
