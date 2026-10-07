<!--
SPDX-FileCopyrightText: 2026 NOI Techpark <digital@noi.bz.it>

SPDX-License-Identifier: CC0-1.0
-->

# Deployment: Docker, Helm, GitHub workflows

Every component ships the same skeleton:

```
<component>/
  docker-compose.yml              # local dev (dev target, mounts ./src:/code)
  .env.example                    # checked in; .env is gitignored
  calls.http                      # manual REST-client requests (optional)
  src/                            # Go module
  resources/                      # optional static data baked into the image
  infrastructure/
    docker/Dockerfile             # multi-stage
    docker-compose.build.yml      # used by CI to build+tag the published image
    helm/                         # values overlays (see below)
```

## Dockerfile (multi-stage, uniform)

```dockerfile
FROM golang:1.25-bookworm AS base

FROM base AS build-env
WORKDIR /app
COPY src/. .
# COPY resources/. ./resources        # only if the component has resources/
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -o main

FROM alpine:latest AS build           # published image
WORKDIR /app
COPY --from=build-env /app/main .
# COPY --from=build-env /app/resources /resources
ENTRYPOINT [ "./main"]

FROM base AS dev                      # local development (docker-compose target)
WORKDIR /code
CMD ["go", "run", "."]

FROM base AS test                     # CI test target
WORKDIR /code
COPY src/. .
# COPY resources/. /resources
RUN go mod download
CMD ["go", "test", "./..."]
```

Keep the `go.mod` Go directive ≤ the Dockerfile base Go version.

`infrastructure/docker-compose.build.yml` (CI wrapper, always identical):

```yaml
services:
  app:
    image: ${DOCKER_IMAGE}:${DOCKER_TAG}
    build:
      context: ../
      dockerfile: infrastructure/docker/Dockerfile
      target: build
```

## Helm: one shared chart, per-component values

Everything deploys the in-repo chart **`helm/generic-collector`** — never write a new chart. Values conventions:

- Dedicated components (transformers, bespoke collectors): `infrastructure/helm/values.test.yaml` + `values.prod.yaml`. Typical diff between them: `LOG_LEVEL`, `.com` vs `.testingmachine.eu` hostnames.
- Generic collectors deployed many times: one file per instance, `infrastructure/helm/<dataset>.yaml` (or `<dataset>.test.yaml`/`.prod.yaml`).

Values anatomy:

```yaml
replicaCount: 1
image:
  repository: ghcr.io/noi-techpark/opendatahub-collectors/tr-<name>
  pullPolicy: IfNotPresent
  tag: ""                          # CI overwrites with the git SHA
imagePullSecrets:
  - name: container-registry-r
env:                               # plain env vars
  MQ_EXCHANGE: routed
  ...
envSecret:                         # sensitive values → chart-generated k8s Secret;
  SOME_API_KEY: ""                 # left empty in git, populated by CI `yq` from GitHub secrets
envSecretRef:                      # bind env vars to PRE-EXISTING cluster secrets
  - name: MQ_URI
    secret: rabbitmq-svcbind
    key: uri
```

Standing cluster secrets: `rabbitmq-svcbind` (key `uri`), `oauth-collector` (keys `tokenUri`, `clientId`, `clientSecret` — BDP/timeseries auth), `mongodb-collector-svcbind` (legacy direct-Mongo transformers only), pull secret `container-registry-r`. **Never hardcode a secret value in a values file.** The chart also supports `configMap.files` (used by api-crawler for silky configs, mounted at `configMap.mountPath`).

In-cluster endpoints: `BDP_BASE_URL: http://bdp-core.core.svc.cluster.local`, `RAW_DATA_BRIDGE_ENDPOINT: http://raw-data-bridge.core.svc.cluster.local:2000`, `TELEMETRY_TRACE_GRPC_ENDPOINT: tempo-distributor-discovery.monitoring.svc.cluster.local:4317`.

## GitHub workflow per pipeline component

Name: `.github/workflows/dc-<name>.yml` / `tr-<name>.yml`. Structure: paths filter → `test` → `build` → `deploy-test` → `deploy-prod`. Branch gating: `main` (and optionally a `dev/<feature>` branch) → test environment; `prod` → production. Registry: ghcr.io with the built-in `GITHUB_TOKEN`. Cluster: `aws-main-eu-01` (eu-west-1), namespace `collector`.

Transformer template (pattern of `tr-parking-offstreet-skidata.yml` / `tr-bike-boxes.yml`):

```yaml
name: CI/CD tr-<name>
on:
  push:
    paths:
      - "transformers/<name>/src/**"
      - "transformers/<name>/infrastructure/**"
      - ".github/workflows/tr-<name>.yml"
env:
  WORKING_DIRECTORY: transformers/<name>
  DOCKER_IMAGE: ghcr.io/noi-techpark/opendatahub-collectors/tr-<name>
  DOCKER_TAG: ${{ github.sha }}
  KUBERNETES_NAMESPACE: collector
  K8S_NAME: tr-<name>
jobs:
  test:
    runs-on: ubuntu-24.04
    concurrency: tr-<name>-test-run
    steps:
      - uses: actions/checkout@v4
      - name: Run tests
        run: docker run --rm $(docker build -q . -f infrastructure/docker/Dockerfile --target test)
        working-directory: ${{ env.WORKING_DIRECTORY }}
  build:
    runs-on: ubuntu-24.04
    concurrency: tr-<name>-build
    needs: [test]
    steps:
      - uses: actions/checkout@v4
      - uses: noi-techpark/github-actions/docker-build-and-push@v2
        with:
          working-directory: ${{ env.WORKING_DIRECTORY }}/infrastructure
          docker-username: ${{ github.actor }}
          docker-password: ${{ secrets.GITHUB_TOKEN }}
  deploy-test:
    if: github.ref == 'refs/heads/main'
    needs: build
    runs-on: ubuntu-24.04
    concurrency: tr-<name>-test
    environment: test
    env:
      VALUES_YAML: infrastructure/helm/values.test.yaml
    steps:
      - uses: actions/checkout@v4
      - name: Customize values.yaml
        working-directory: ${{ env.WORKING_DIRECTORY }}
        run: |
          yq -i '.image.repository="${{ env.DOCKER_IMAGE }}"' ${{ env.VALUES_YAML }}
          yq -i '.image.tag="${{ env.DOCKER_TAG }}"' ${{ env.VALUES_YAML }}
          yq -i '.image.pullPolicy="IfNotPresent"' ${{ env.VALUES_YAML }}
          # BDP transformers only:
          yq -i '.env.BDP_PROVENANCE_NAME="${{ env.K8S_NAME }}"' ${{ env.VALUES_YAML }}
          yq -i '.env.BDP_PROVENANCE_VERSION="${{ github.sha }}"' ${{ env.VALUES_YAML }}
          # secrets go into envSecret:
          # yq -i '.envSecret.MY_SECRET="${{ secrets.MY_SECRET }}"' ${{ env.VALUES_YAML }}
      - uses: noi-techpark/github-actions/helm-deploy@v2
        with:
          k8s-name: ${{ env.K8S_NAME }}
          k8s-namespace: ${{ env.KUBERNETES_NAMESPACE }}
          chart-path: helm/generic-collector
          values-file: ${{ env.WORKING_DIRECTORY }}/${{ env.VALUES_YAML }}
          aws-access-key-id: ${{ secrets[vars.AWS_KEY_ID] }}
          aws-secret-access-key: ${{ secrets[vars.AWS_KEY_SECRET] }}
          aws-eks-cluster-name: aws-main-eu-01
          aws-region: eu-west-1
  deploy-prod:
    if: github.ref == 'refs/heads/prod'
    # identical to deploy-test with environment: prod and values.prod.yaml
```

Variants observed in the repo:

- **Generic-collector instance workflows** (`dc-api-crawler-*`, `dc-s3-poller-*`, `dc-rest-poller-*`): no `test` job (shared binary); the paths filter negates all sibling instance files then re-includes only this instance's:
  ```yaml
  paths:
    - "collectors/api-crawler/**"
    - "!collectors/api-crawler/infrastructure/helm/*"
    - "!collectors/api-crawler/infrastructure/crawler-config/*"
    - "collectors/api-crawler/infrastructure/helm/<dataset>.yaml"
    - "collectors/api-crawler/infrastructure/crawler-config/<dataset>.silky.yaml"
    - ".github/workflows/dc-api-crawler-<dataset>.yml"
  ```
- **api-crawler** deploy inlines the silky config: `yq -i '.configMap.files["config.yaml"] = load_str("<path>.silky.yaml")'` and puts provider credentials in `.envSecret.*`.
- **Content transformers** inject `ODH_CORE_TOKEN_CLIENT_ID/SECRET` from GitHub secrets into `.envSecret` (dedicated client per data-space, not `oauth-collector`).
- **Manual trigger**: add `workflow_dispatch` with an `environment` choice input and OR-combine the deploy gates:
  ```yaml
  if: >-
    (github.event_name == 'workflow_dispatch' && inputs.environment == 'test') ||
    (github.event_name == 'push' && github.ref == 'refs/heads/main')
  ```
- Multi-line secrets (JSON blobs) via `strenv`: `yq -i '.envSecret.CREDS_JSON = strenv(CREDS)'` with the value passed through the step's `env:`.
- Some older workflows put credentials in plain `.env.*` values — for new work always use `.envSecret`.

## REUSE / licensing

CI (`.github/workflows/reuse.yml`) enforces REUSE compliance on every push. New files need an SPDX header (`SPDX-FileCopyrightText: <year> NOI Techpark <digital@noi.bz.it>` + `SPDX-License-Identifier: CC0-1.0`), a `.license` sidecar for binary/data files, or coverage by a `REUSE.toml` annotation (helm yaml, `*.json`, `*.http`, `go.mod/sum` and `.github/**` are already covered).

## Local development

- Per-component `docker-compose.yml`: `app` service builds the `dev` target, mounts `./src:/code` (+ go module cache volume), reads `.env`; RabbitMQ extends `collectors/lib/docker-compose/docker-compose.rabbitmq.yml` (management UI on 15672). Components talking to port-forwarded cluster services use `network_mode: host` instead.
- Full local platform: use the skill's bundled `assets/docker-compose.validation.yml` (profiles `timeseries`/`content`; all images from ghcr.io, no extra repositories needed — see the validation reference). Endpoints: RabbitMQ UI `http://localhost:15672` (guest/guest), MongoDB `mongodb://localhost:27017/?directConnection=true`, bridge `:2000`, BDP `:8081`, ninja `:8082`, Content API `:8083`. The upstream source of these services is `github.com/noi-techpark/infrastructure-v2`.
- `kind.sh` at the repo root is a commented recipe for deploying a component into a local kind cluster (`kind load docker-image`, `helm upgrade --install -n collector ... ./helm/generic-collector -f <overlay>`, create the `rabbitmq-svcbind` secret manually).
- `calls.http` files: REST-client scratch requests; push collectors include a named Keycloak login request whose token is reused as `Authorization: Bearer {{login.response.body.access_token}}`.
