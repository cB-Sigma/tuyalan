# Changelog

All notable changes to this module. Versions follow semantic versioning;
before v1 the API may change in minor versions.

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
