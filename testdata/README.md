# Frozen synthetic regression references

These files contain synthetic activities, not private activity recordings. The
`.bin` suffix distinguishes them from ignored user `.fit` files.

The eight CLI cases were captured from the original Python regression suite.
HTTP responses and four transformation outputs were captured from Flask before
retirement. `provenance.json` records their origin, Garmin Python FIT
SDK 21.214.0, and the sanitized files’ SHA-256 hashes. `http.json` contains
inspection expectations and transformation fields; `cases.json` describes CLI
operations. The profile name table remains derived from that same SDK version.

During retirement, recorder serials in File ID and Device Info records were
replaced with consistent dummy values and FIT checksums were repaired. Record
comparison verified that all other bytes were unchanged. Every sanitized CLI and
HTTP expected result was checked against the original Python implementation
before deletion. Numeric products and firmware versions were retained. The initial Fittools commit contains only these sanitized references.

## Maintenance

There is deliberately no `just reference` recipe. Do not regenerate expected
outputs using the Go transformer: that would allow a regression to redefine its
own expected result. To add a case, construct a synthetic input using an official
SDK encoder or documented raw FIT records, establish the expected result through
independent decoding and reviewed field/byte changes, and record the rationale,
SDK version, source and hashes in the manifest. Run `just check` afterward.
Existing golden replacements require the same review and an explanation of why
the previous expectation is no longer correct. Use dummy identities only.

To update `internal/fit/profile.json`, compare the manufacturer, Garmin product,
sport and sub-sport tables against an identified official Garmin SDK version.
Review name formatting and the project's product-name overrides (4315, 3121,
4953), retain the numeric keys, and update THIRD_PARTY_NOTICES.md with that SDK's
version and source. A decoder dependency upgrade does not itself update the
frozen profile table.

`web/scripts/validation.mjs` verifies fixture hashes and FIT integrity using
Garmin's JavaScript SDK; every CLI and HTTP output must also independently decode.
The JS SDK 21.218.0 does not decode compressed-timestamp records. The compressed
input `05-pauses.input.bin` receives SDK integrity checking and Go parser coverage;
its transformed output expands the timestamp and must pass SDK decoding. This
retains the previous independent output-validation responsibility.

## Original regression coverage

| Original Python scenario | Native/browser replacement |
|---|---|
| Decimal, hexadecimal and base-36 serials | `TestFirmwareAndSerial` |
| Firmware hundredths and rejection | `TestFirmwareAndSerial` |
| Numeric File ID identity | `TestIdentityRejections`, `TestHTTP` |
| Missing Device Info permits inspection but rejects rewriting | `TestIdentityRejections` |
| Byte-identical reversal and paired sensor identity | `TestGolden/01-identity`, `TestGolden/02-identity`, `TestFirmwareRoundTripPreservesComponents` |
| Firmware changes only on primary recorder | `TestGolden/00-identity`, `TestFirmwareRoundTripPreservesComponents` |
| Mismatched source rejected | `TestIdentityRejections` |
| Continuous mode removes rest metadata/controls and retains samples | `TestGolden/03-continuous` |
| Unrelated/private records and timer accounting | `TestGolden/07-pauses` |
| Existing pause spanning a rest stays stopped | `TestGolden/06-pauses` |
| Activity timer sums rewritten sessions | `TestGolden/04-pauses` |
| Compressed record after removed anchor remains valid | `TestGolden/05-pauses` |
| Index, presets and identity inspection | Browser runtime preset tests, `TestHTTP`, HTTP verifier |
| Friendly activity names and summary values | `TestHTTP`, frozen HTTP inspection contract |
| Rest downloads and sample counts | `TestHTTP`, CLI/HTTP verifiers, browser downloads |
| Detected source identity and preserved firmware | `TestHTTP`, `web-identity.output.bin` contract |
| HTTP firmware override, component preservation and reversal | `TestHTTP`, HTTP verifier |
| Invalid FIT and identity errors | `TestErrorsAndLimits`, CLI/HTTP verifiers, browser validation |
| Oversized uploads | `TestErrorsAndLimits` |

`TestBigEndianDeveloperAndCompressed` additionally checks big-endian fields,
developer data and compressed timestamps; other server tests check temporary-file
cleanup, admission limits and cancellation. The CLI verifier checks unchanged
inputs, same-file aliases and preservation of existing output after failure.
