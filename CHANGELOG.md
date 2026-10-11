# Changelog

All notable changes to this module. Versions follow semantic versioning;
before v1 the API may change in minor versions.

## v0.2.0 (2026-10-11)

Additions only; the v0.1.0 API is unchanged.

- Active discovery: `Discover` listens like `Hear` and sends
  REQ_DEVINFO (command 0x25 to UDP 7000) at the start and every
  interval, so 3.5 devices announce themselves; `RequestDevInfo`,
  `EncodeDevInfoRequest`, `ParseDevInfoRequest`, `CmdReqDevInfo`,
  `DevInfoPort`.
- Scanning: `Scan` tries TCP 6668 over addresses, CIDR blocks and
  ranges (`ExpandTargets`) with a concurrency limit; `Identify` tells a
  found device's version (and device22 behaviour) with its local key.
- Gateways: `Conn.Sub` and `Sub` (`Query`, `QueryByControl`, `Control`,
  `ControlSeq`, `UpdateDPS`) put the cid where tinytuya does for each
  version; `CID` reads it from answers and pushes.
- `Conn.UpdateDPS` sends UPDATE_DPS.
- Data point helpers: `BulbLayout` with `BulbV1`, `BulbV2`,
  `BulbDimmer` and `DetectBulb`; colour hex in both formats
  (`ColourHexRGB`, `ColourHexHSV`, `ParseColourHex`); `Cover` with the
  eight value schemes and `DetectCoverScheme`; `Countdown` and
  `CountdownDP`.
- `CmdLanExtStream` and `TCPPort` constants.
- Interop: the peer image is python:3.12.10-slim (was
  python:3.14-slim); new tests for sub-device requests against
  tinytuya's own `generate_payload`, REQ_DEVINFO both ways, and the
  helpers against tinytuya's bulb and cover classes. The image is now
  rebuilt (from cache) on every run.

## v0.1.0 (2026-10-11)

First release as a module of its own, split from SigmaM2M: protocol 3.1
to 3.5 (`Dial`, `Conn`, `Codec`, `SessionKey`, the 55AA and 6699
frames), `DPS`, and discovery (`ParseBroadcast`, `EncodeBroadcast`,
`ListenDiscovery`, `Hear`).

- UPDATE_DPS payloads carry no version header, as tinytuya sends them
  (before the split, `Codec.SealPayload` added one).
- The simulator lives in its own module,
  [tuyalan-sim](https://github.com/cB-Sigma/tuyalan-sim), with the
  session tests that drive it.
- Interop tests against tinytuya 1.20.0 in a nested test-only module,
  `interop`.
