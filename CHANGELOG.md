# Changelog

All notable changes to this module. Versions follow semantic versioning;
before v1 the API may change in minor versions.

## v0.3.0 (2026-10-12)

Additions only; the v0.2.0 API is unchanged.

- Force scan with a key list: `IdentifyWithKeys` tries each key against
  a found device and returns the identity and the key that worked
  (`ErrNoKeyMatched`).
- Gateways: `Conn.SubdevQuery` sends the sub-device online query
  (LAN_EXT_STREAM, `subdev_online_stat_query`) and `ParseSubdevStatus`
  reads the online, offline and nearby lists of the answer.
- `Conn.DetectAvailableDPS` finds a device22 device's data points by
  asking the ranges tinytuya's `detect_available_dps` uses.
- Persistent client: `NewDevice` / `Device` keeps a session open, sends
  heartbeats, reconnects, caches data points and calls `OnChange` for
  pushes and `OnState` for the connection (`Open`, `Status`, `Set`,
  `Cache`, `Connected`, `Close`; `ErrNotConnected`, `ErrClosed`).
- Infrared remote codes: `IRPulsesToBase64`, `IRBase64ToPulses`,
  `IRPulsesToHex`, `IRHexToPulses`, `IRWidthEncodeToPulses`,
  `IRPulsesToWidthEncoded`, `IRNECToPulses`, `IRPulsesToNEC`,
  `IRSamsungToPulses`, `IRPulsesToSamsung`, `IRTiming`, `NECTiming`,
  `SamsungTiming`, `IRCode`.
- IR head/key form and Pronto codes, following tinytuya's
  IRRemoteControlDevice exactly (its packer's tie-breaking, Python's
  dict order and round-half-to-even included): `IRBuildHead`,
  `IRPulsesToHeadKey`, `IRHeadKeyToPulses`, `IRProntoToPulses`,
  `IRPulsesToPronto`, `IRProntoToHeadKey`, `IRSymbols`.
- New package `contrib`: typed data point helpers (a layout struct with
  a `Default...` value, a State reader, setters returning data point
  maps) for the products of tinytuya's 15 Contrib classes: `Socket`,
  `TemperatureController`, `ReadDualMeter` / `DualMeterPoints`, SORIA
  inverter frames (`SoriaTags`, `DecodeSoriaFull`, `DecodeSoriaRealtime`,
  `DecodeSoriaCircuits`), `Climate`, `TowelRail`, `HeatPump`, `Blanket`,
  `Fan`, the thermostat (`ThermostatStateOf`, `Thermostat` setters by
  resolution and unit, `ThermostatSchedule` with `Bytes` / `Update`,
  `ParseThermostatSensors` / `ThermostatSensorList.B64`), `Presence`,
  `Doorbell`, `LEDController`, IR blasters (`IRControlType`,
  `IRSendCode`, `IRSendHeadKey`, `IRCommand`, `IRLearned`) and RF
  blasters (`RFStudy`, `RFSendButton`, `RFSendKeys`); `Merge`,
  `ErrRange`, `ErrValue`, `ErrResolutionUnknown`. Unlike tinytuya's list
  form, `RFSendKeys` numbers several keys key1, key2 and so on.
- `Device.Status` returns only the query's own answer (DP_QUERY or
  DP_QUERY_NEW; with `QueryDPS`, the status after the query's
  acknowledgement), so a change push still on its way is not mistaken
  for it.
- Examples `ExampleNewDevice` and `ExampleIRNECToPulses`.
- Interop: new tests for all of the above against tinytuya 1.20.0
  (`interop/v030_test.go`, `headkey_test.go`, `contrib_test.go`; the
  peer answers LAN_EXT_STREAM, prints tinytuya's IR conversions, and a
  new script `contrib.py` records what tinytuya's Contrib classes send
  and read).

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
