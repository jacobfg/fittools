# Performance measurements

Measured on 19 October 2026 on an Apple Silicon ARM64 host using Apple Container
1.4.1. Both containers used identical runtime resource defaults (including 1 GiB memory). The
baseline was built from the original Python Dockerfile before replacement;
the new image uses the multistage Go/Vite build.

The synthetic input is 1,400,289 bytes and contains 100,000 additional normal
FIT samples plus rest, session, activity, and device messages. Five sequential
pause transformations were measured per implementation. Outputs were identical.
This is an allocation/throughput benchmark, not a representative real-world
activity corpus or a concurrent load test.

| Container metric | Python 3.14.8 | Go 1.27.1 | Change |
|---|---:|---:|---:|
| Compressed image size | 46.00 MB | 2.99 MB | 93.5% smaller |
| Fresh idle container memory | 63.82 MB | 9.28 MB | 85.5% lower |
| Memory after five requests | 77.06 MB | 33.23 MB | 56.9% lower |
| Median HTTP transformation | 498.6 ms | 22.1 ms | 22.5× faster |
| Container CPU per request | 505.9 ms | 37.0 ms | 92.7% lower |

Image sizes are compressed sizes reported by `container image inspect`, not
uncompressed filesystem sizes. Container memory is `memoryUsageBytes`, including
runtime-accounted cache; it is not process RSS. Post-request memory is a snapshot,
not peak memory. CPU is the change in container CPU usage across the five requests
and includes background server work. HTTP latency includes local networking,
upload, parsing, transformation, output, and download.

Native macOS measurements provide peak-process-memory and startup comparisons:

| Native metric | Python 3.9.6 reference | Go 1.27.1 |
|---|---:|---:|
| Median CLI wall time | 870 ms | 20 ms |
| Median CLI CPU time | 860 ms | 20 ms |
| Median CLI peak RSS | 50.86 MB | 36.14 MB |
| Server startup to health | 81.8 ms | 15.6 ms |
| Idle server RSS | 31.20 MB | 12.75 MB |
| Median HTTP transformation | 857.1 ms | 18.6 ms |

CLI values use macOS `/usr/bin/time -l` and include process startup; its timing
precision is limited. Native startup is one sample with health polling every
10 ms. These native measurements use a different Python version from the
container and should not be mixed with the container results. Values use decimal
MB. Results will vary by hardware, filesystem, FIT record layout, and workload.

## Reproduce current Go measurements

```sh
just benchmark
```

This runs five Go benchmark samples with throughput and allocation reporting,
using both a small synthetic activity and the historical workload of 100,000
additional copies of its first normal sample. It measures parsing, transformation
and serialization, excluding process startup and HTTP transport. These numbers
must not be compared directly to historical CLI or container latency/RSS values.

The original Python comparison scripts were retired with the reference
implementation. The historical toolchain and baseline image are not distributed with Fittools,
so those comparison tables cannot be reproduced from this checkout. Current
Go measurements use the benchmark above and need neither Python nor uv.

The runtime parses each request once, retains slices into its input buffer,
buffers FIT output writes, streams downloads from a temporary output file, and
admits one active FIT operation by default. Memory scales with file size and
record count; raising `FIT_MAX_CONCURRENT` increases possible memory use.

## Historical validation completed

- All 19 original Python regression tests passed unchanged.
- Native Go tests and the race detector passed; FIT library coverage was 84.7%
  and HTTP handler coverage 77.2% at measurement time.
- Eight native CLI outputs matched Python goldens exactly and passed independent
  Garmin SDK integrity and decoding checks.
- All 10 desktop/mobile browser tests passed using installed Chrome through
  Playwright. CI installs Playwright Chromium; local `PLAYWRIGHT_CHANNEL=chrome`
  selects an existing Chrome installation.
- Both Linux ARM64 and AMD64 container builds succeeded. ARM64 container health,
  inspection, all three transformations, download headers, and invalid uploads
  were checked against the Python reference. AMD64 was cross-built, not executed.
- Go vet and a five-second parser fuzz run passed. npm audit reported no known
  vulnerabilities for the locked frontend dependency tree.
