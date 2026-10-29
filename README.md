# Fittools

A Go web application and CLI for repairing Garmin FIT activities, with a static
Vite + TypeScript + Tailwind UI. One binary serves the UI and performs all FIT
operations. No Python, Node, database, or external services are needed at runtime.

- **Turn rests into pauses:** remove Ultra Run rest samples, preserve wall-clock
  gaps, and insert conventional FIT timer events.
- **Keep it continuous:** remove rest metadata and timer controls while retaining
  samples, allowing compatible services to calculate moving time from GPS.
- **Change recording device:** update recording-device identity and optional
  firmware without altering paired sensors.
- **Inspect:** view device identity, firmware, activity type, distance, elapsed
  time, and timer time without changing the file.

The transformer operates on raw FIT records. Unknown/private messages, developer
fields, and unaffected bytes are retained. Compressed timestamps are expanded
only when necessary. Identity changes preserve local component firmware. For a
byte-identical reversal, restore the exact original recorder firmware as well as
its identity; preset firmware is not necessarily the original version.

## Container

```sh
docker build -t fittools .
docker run --rm -p 127.0.0.1:8080:8080 fittools
```

Apple Container on ARM64:

```sh
container system start
container build --arch arm64 -t fittools .
container run --arch arm64 --rm -p 127.0.0.1:8080:8080 fittools
```

Open <http://127.0.0.1:8080>. The Dockerfile builds the frontend in a Node stage,
compiles a static Go executable in another, and copies it into a `scratch` image.
The runtime runs as UID/GID 10001 and has a writable `/tmp`; no shell or package
manager is included. `/fittools healthcheck` checks `/healthz` without curl.

External health monitors can use `GET /healthz` or `HEAD /healthz` on port 8080.
Both return HTTP 200 while the server is running; GET returns `ok` followed by a
newline. Responses are not cached, and checks remain available during FIT processing.
Linux ARM64 and AMD64 builds are supported with Docker Buildx:

```sh
docker buildx build --platform linux/amd64,linux/arm64 -t fittools --output type=oci,dest=fit-images.tar .
```

Uploads and generated files live in a request-specific temporary directory and
are deleted after the download finishes or fails. The original is never modified.
The maximum multipart request size is 100 MiB, including form overhead. One FIT
operation runs at a time by default; additional requests receive JSON HTTP 503
with `Retry-After: 1`. Set `FIT_MAX_CONCURRENT` to a positive integer to increase
parallelism at the cost of memory. There is no retained upload history or queue.

Keep the published port bound to localhost: this tool has no authentication.
The UI processes files on the server, not in the browser.

### Publish a release image

After merging the release changes into `main`, push a stable semantic-version tag
without a `v` prefix:

```sh
git switch main
git pull --ff-only
git tag -a 0.1.1 -m "Release 0.1.1"
git push origin 0.1.1
```

The publish workflow checks that the tagged commit belongs to `main`, then builds
one multi-architecture image for `linux/amd64` and `linux/arm64` and publishes:

- `ghcr.io/jacobfg/fittools:0.1.1`
- `ghcr.io/jacobfg/fittools:latest`

`latest` is updated by each successful release build. Tags with a `v` prefix,
prerelease suffix, or build metadata are not published. Authentication uses the
workflow's `GITHUB_TOKEN` with `packages: write`; no registry password secret is
needed. For anonymous pulls, set the GHCR package visibility to public after its
first publication.

```sh
docker run --rm -p 127.0.0.1:8080:8080 ghcr.io/jacobfg/fittools:0.1.1
```

## Local build and development

Requires Go 1.25.0+, Node 24+ with npm, and `just`. Dependencies are locked in
`web/package-lock.json`; the Go runtime uses only the standard library.

```sh
just build
./bin/fittools serve
```

Local serving defaults to `127.0.0.1:8080`. `FIT_UI_HOST` overrides the host;
containers set it to `0.0.0.0`. SIGINT/SIGTERM trigger graceful shutdown.

For frontend development, run the Go server and then `cd web && npm run dev`.
Vite proxies FIT API requests to port 8080. Rebuild the binary to embed frontend
changes in production. Frontend assets, including CSS, are bundled locally.

## Native CLI

```sh
./bin/fittools pauses UltraRun.fit UltraRun-pauses.fit
./bin/fittools continuous UltraRun.fit UltraRun-continuous.fit
./bin/fittools identity UltraRun.fit UltraRun-new-device.fit \
  --from-manufacturer 1 --from-product 4953 --from-serial 100000001 \
  --to-manufacturer 1 --to-product 4315 --to-serial 100000002 \
  --to-firmware 28.05
```

Input/output must differ, including when they refer to the same file through a
symlink or hard link. Output is written to a temporary file and renamed only
when the operation succeeds. Identity options may precede or follow the paths.

Serials accept decimal, `0x` hexadecimal, and base-36 strings such as `ABC123`
or `base36:ABC123`. Firmware uses exact hundredths; omit it to preserve the
source value. Swap source/target identities to reverse an identity change;
restore the original firmware explicitly if it was changed. Firmware overrides
apply only to primary recording-device messages (device index 0 with the source
serial), not to local components. If primary messages originally reported different
firmware versions, a single override cannot reconstruct all original values.
Files altered by older versions that overwrote component firmware need the
original file to recover those component values.

The UI detects source identity automatically. Bundled presets demonstrate
Forerunner 965 (4315), Edge 530 (3121), and Fenix 9 Pro 47mm (4953), using
**dummy serials**. Replace the serial with your device's numeric FIT serial;
human-facing Garmin labels are not necessarily valid FIT serial numbers.

### Runtime device presets

Keep private presets outside Git in `local-config/device-presets.json`:

```sh
mkdir -p local-config
cp web/device-presets.json local-config/device-presets.json
# Edit local-config/device-presets.json with your device details.
docker run --rm -p 127.0.0.1:8080:8080 \
  --mount type=bind,src="$(pwd)/local-config",dst=/config,readonly \
  -e FIT_DEVICE_PRESETS_FILE=/config/device-presets.json fittools
```

Apple Container uses the same environment variable with
`--volume "$(pwd)/local-config:/config:ro"`. Mount the directory so atomic file
replacement by editors is visible. The container runs as UID 10001; ensure it can
read the mounted file. `local-config/` is excluded from Git and image build context.

The server reads the JSON file each time the page requests `GET /device-presets`.
After adding a device, refresh the browser; no rebuild or restart is needed.
Without `FIT_DEVICE_PRESETS_FILE`, the binary serves bundled dummy examples.
A configured file that is missing or invalid produces a visible loading error;
custom device entry remains available. Presets are sent to the browser, so keep
this unauthenticated tool bound to localhost as described above.

For a local server:

```sh
FIT_DEVICE_PRESETS_FILE="$PWD/local-config/device-presets.json" ./bin/fittools serve
```

Each array entry requires a unique `id` (other than `custom`), display `name`,
numeric `manufacturer`, `product`, and nonzero `serial`, a `firmware` string
(or an empty string to preserve firmware), and a `note`. Keep firmware as a string
so values such as `"6.30"` retain formatting. An empty array disables presets.
Manufacturer/product ranges and serial/firmware values use the same validation
as identity transformations. Edit the private copy, not the tracked example.

## HTTP compatibility

Routes remain `POST /transform`, `POST /inspect-identity`, and
`POST /activity-details`. Upload a multipart `fit_file`; transformations also
use `action=pauses|continuous|identity` and optional identity fields
`manufacturer`, `product`, `serial`, and `firmware`.
Inspection JSON and attachment filename suffixes are preserved. Errors now
consistently return JSON `{ "error": "message" }`, including transform failures
and HTTP 413, rather than rendering the old HTML form.

## Tests and frozen references

Build, run, test, and validate with Go, Node/npm, and just; Python and uv are not
required. Install Playwright Chromium once, or use installed Chrome:

```sh
just build
just test
cd web && npx playwright install chromium
cd ..
just test-browser
just check
# Alternatively: PLAYWRIGHT_CHANNEL=chrome just check
just verify-http http://127.0.0.1:8080  # an already-running server or container
```

`just check` runs Go race tests, vet, CLI golden verification, and desktop/mobile
browser tests including live HTTP contract verification. Garmin's JavaScript SDK
independently checks FIT integrity and decodes every transformation output. It is
a development dependency and is not bundled into the browser or production image.
CI also builds Linux ARM64/AMD64 images and checks the running container, including
runtime preset edits without rebuilding.

`testdata/` contains frozen synthetic fixtures and provenance. All 19 original
regression scenarios are mapped to native/browser coverage in
[testdata/README.md](testdata/README.md), including byte-identical identity reversal,
private records, compressed timestamps, timer accounting, sensor identity and
component firmware. That document describes intentional fixture/profile updates
and the independent decoder's compressed-input limitation. Automatic reference
regeneration has been retired. See [third-party notices](THIRD_PARTY_NOTICES.md)
for profile provenance and licensing.

```sh
# Optional parser fuzzing:
go test ./internal/fit -fuzz=FuzzParse -fuzztime=30s
```

Real FIT activities are ignored by Git because they can contain private health
and location data.

## Performance

See [PERFORMANCE.md](PERFORMANCE.md) for historical comparisons and limitations.
Run native Go throughput/allocation measurements with:

```sh
just benchmark
```

The benchmark includes a small synthetic activity and a 100,000-added-sample
workload. It needs no Python, server or private files. For deployments with tight
memory limits, measure representative activities and set container memory and
concurrency accordingly.

## Scheduled dependency updates

The Renovate Bot workflow runs daily at 06:04 UTC and supports manual runs with
optional debug logging. It follows the [container-scheduler setup](https://github.com/jacobfg/container-scheduler/blob/main/.github/workflows/renovatebot.yml).
Stable minor and patch updates to Go, Go modules (including indirect modules),
Dockerfile images, and frontend npm packages are grouped into one maintenance
PR. Renovate updates the README Go requirement, runs `go mod tidy`, and maintains
`web/package-lock.json`. Existing PRs are refreshed; no updates means no new PR.
Major upgrades appear in the Dependency Dashboard issue and require checkbox
approval before Renovate creates a PR. Merging remains manual. GitHub Actions
and the pinned Renovate action/engine versions are maintained manually.

Before running it, add an Actions secret named `RENOVATE_TOKEN` containing a
dedicated GitHub PAT authorized for `jacobfg/fittools`, with contents and
pull-request and Issues write access, and the read permissions required by
[Renovate](https://docs.renovatebot.com/modules/platform/github/). A fine-grained
token must explicitly include this repository. The dedicated token allows
Renovate PRs to trigger CI; do not substitute `GITHUB_TOKEN`. The workflow checks
repository access before starting. Scheduled runs begin after merge to `main`.

The workflow loads `.github/renovate.json5` only as repository configuration;
do not also pass it as the action's global `configurationFile`, which duplicates
custom managers. Branch-update failures are promoted to errors so a rejected
push fails the Action. The access preflight only confirms repository visibility;
a successful read does not prove the PAT can push. If logs show a push rejected
with HTTP 403, verify the PAT's repository selection, Contents read/write and
Pull requests read/write permissions, and any repository rules. Issues read/write
is required for the Dependency Dashboard. Update `RENOVATE_TOKEN` if replacing
the PAT, then rerun the workflow.
