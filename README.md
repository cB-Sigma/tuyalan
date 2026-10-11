# tuyalan

The local LAN protocol of Tuya-based Wi-Fi devices, versions 3.1 to 3.5, in pure Go; not affiliated with Tuya.

[![Go Reference](https://pkg.go.dev/badge/github.com/cB-Sigma/tuyalan.svg)](https://pkg.go.dev/github.com/cB-Sigma/tuyalan) [![test](https://github.com/cB-Sigma/tuyalan/actions/workflows/test.yml/badge.svg)](https://github.com/cB-Sigma/tuyalan/actions/workflows/test.yml) [![Licence: Apache-2.0](https://img.shields.io/badge/licence-Apache--2.0-blue.svg)](LICENSE) [![Go](https://img.shields.io/badge/go-1.22-00ADD8.svg)](go.mod)

It speaks to a device with its local key: 55AA frames with CRC32 or
HMAC-SHA256, 6699 frames with AES-GCM, the 3.4 and 3.5 session key
negotiation, reading and setting data points, status pushes and
heartbeats, the UDP discovery announcements and REQ_DEVINFO requests, a
TCP scan that can try a list of keys, gateway sub-devices and their
online list, a persistent device client that reconnects and caches,
helpers for bulb, cover, countdown and infrared remote data points
(including the head/key and Pronto forms), and, in `contrib`, typed
helpers for the products tinytuya's community classes cover (thermostat,
air conditioner, heat pump, blanket, fan, presence sensor, doorbell, LED
controller, energy meters, a solar inverter, IR and RF blasters). It
does not use the Tuya cloud. Standard library only, no cgo. It was written for SigmaM2M,
a oneM2M platform, whose Tuya adapter is built on it.

## Features

| Feature | Status |
|---|---|
| Protocol 3.1 (clear queries, signed and encrypted CONTROL), 3.2 (3.3 framing, null-value queries) | yes |
| Protocol 3.3 (55AA, CRC32, AES-ECB with the local key) | yes |
| Protocol 3.4 (55AA, HMAC-SHA256, session key) and 3.5 (6699, AES-GCM, session key) | yes |
| DP_QUERY / DP_QUERY_NEW, CONTROL / CONTROL_NEW with return codes and sequence numbers, status pushes, heartbeats | yes |
| Discovery announcements on 6666, 6667 and 7000: decode, encode, listen (`Hear`, `ListenDiscovery`) | yes |
| Asking 3.5 devices to announce themselves: REQ_DEVINFO to UDP 7000, repeated while listening (`Discover`, `RequestDevInfo`, `EncodeDevInfoRequest`, `ParseDevInfoRequest`) | yes |
| TCP scan of port 6668 over addresses, CIDR blocks and ranges with a concurrency limit (`Scan`, `ExpandTargets`); telling a found device's version with its local key (`Identify`) | yes |
| Gateways' sub-devices by cid: query, control, null-value query (`Conn.Sub`, `Sub`), the cid of answers and pushes (`CID`) | yes |
| UPDATE_DPS requests (`Conn.UpdateDPS`, `Sub.UpdateDPS`) | yes |
| Bulb helpers for both layouts tinytuya tells apart (from data point 1 and from 20) and white-only bulbs: detection, on, white, brightness, colour temperature, RGB and HSV colours in both hex formats, scenes, countdown, state (`DetectBulb`, `BulbLayout`, `BulbV1`, `BulbV2`, `BulbDimmer`) | yes |
| Cover helpers for the eight value schemes tinytuya knows, with detection and position (`Cover`, `DetectCoverScheme`, `DefaultCover`) | yes |
| Countdown timer data points (`Countdown`, `CountdownDP`) | yes |
| Trying a list of local keys against a found device, as tinytuya's force scan does (`IdentifyWithKeys`) | yes (3.3 to 3.5 prove the key; a 3.1 device answers in clear, so its key is not verified) |
| A gateway's sub-device online list over LAN_EXT_STREAM (`Conn.SubdevQuery`, `ParseSubdevStatus`) | yes (3.4 and 3.5 gateways; 3.3 gateways are reported not to answer) |
| Finding a "device22" device's data points by probing ranges (`Conn.DetectAvailableDPS`) | yes |
| Persistent device client: one open session, heartbeats, reconnection, a data point cache, change and connection callbacks (`NewDevice`, `Device`) | yes |
| Infrared remote codes: pulses to and from the base64 and hex forms IR blasters use, NEC and Samsung encoding and decoding (`IRPulsesToBase64`, `IRNECToPulses`, `IRPulsesToNEC` and others) | yes |
| IR head/key form (the packer, the expander, `IRBuildHead`) and Pronto codes (`IRPulsesToHeadKey`, `IRHeadKeyToPulses`, `IRProntoToPulses`, `IRPulsesToPronto`, `IRProntoToHeadKey`) | yes (the packer is tinytuya's heuristic, followed exactly so the codes agree) |
| `contrib`: typed data point helpers for the products of tinytuya's Contrib classes: metering socket, temperature controller, two-channel energy meter, SORIA micro-inverter frames, air conditioner, towel rail, inverter heat pump, electric blanket, floor fan, 24 V thermostat (setpoints by resolution and unit, binary schedule, remote sensor lists), presence sensor, video doorbell, Colorful-X7 LED controller, IR blasters of both control types, RF blasters | yes (each a State reader and setters returning data point maps; no I/O) |
| Scenes stored on a gateway (SCENE_EXECUTE) | no (the command number is known; no implementation shows its request or answer; not guessed) |
| The cloud API, fetching local keys, pairing | no (needs a Tuya IoT Platform developer account; see Not yet) |

## Choosing a Go Tuya LAN library

Compared from each project's source at the version named, on
2026-10-12 (tinytuya 1.20.0 is still the latest on PyPI, and its GitHub
master has the same library code; tiny-go v1.0.1 is still its latest
tag; py60800/tuya has had no commit since 2025-09-29). Check the projects
themselves before relying on a row.

| | this module | MXVXID/tiny-go v1.0.1 | py60800/tuya (2025-09-29) | tinytuya 1.20.0 (Python) |
|---|---|---|---|---|
| Protocol versions | 3.1 to 3.5 | 3.1 to 3.5 | 3.1 (its README) | 3.1 to 3.5 |
| Discovery: listen on 6666, 6667, 7000 | yes | yes | yes | yes |
| Active 3.5 discovery (REQ_DEVINFO), TCP force scan | yes | yes | no | yes |
| Trying a list of local keys against a found device | yes (`IdentifyWithKeys`) | yes (`ForceScan`) | no | yes (force scan) |
| Gateway sub-devices, timers, bulb and cover helpers | yes (data point maps for your `Control`; no device classes) | yes | switches only | yes |
| Sub-device online list (LAN_EXT_STREAM) | yes, request and answer parsed (`SubdevQuery`, `ParseSubdevStatus`) | request only (`SubdevQuery`; the answer is returned raw) | no | request only (`subdev_query`; the answer is returned raw) |
| device22 data point discovery | yes (`DetectAvailableDPS`, every range) | yes (`DetectAvailableDps`) | no | yes (`detect_available_dps`; on the run that switches a device to device22 it skips that first range, found in our interop run) |
| Persistent session: reconnect, heartbeat, status cache, change callbacks | yes (`Device`) | yes (`Monitor`, `CachedStatus`) | a device manager | yes (persistent socket, `cached_status`, monitor loop in its examples) |
| Infrared remote codes (base64, hex, NEC, Samsung, head/key, Pronto) | yes (pure functions) | no | no | yes (`IRRemoteControlDevice`) |
| Product helpers of tinytuya's Contrib classes (thermostat, air conditioner, heat pump, doorbell, presence sensor, LED controller, meters, inverter, IR and RF blasters and so on) | yes (`contrib`: data point maps, all 15 classes) | no | no | yes (device classes) |
| Gateway scenes (SCENE_EXECUTE) | no | no | no | no (the command number only) |
| Tuya cloud client and local-key wizard | no (needs a developer account; see Not yet) | yes | no | yes |
| Device simulator for tests | yes, every version, separate module | a loopback fake in its tests | no | no |
| Interop evidence | against tinytuya both ways, every version (below) | states byte parity with tinytuya | none stated | the reference most others follow |
| API shape | a `Conn` per device; frames, codec and session key exported | a port of tinytuya's API | device manager | Python |
| Go version | 1.22 | 1.26.5 | no go.mod (GOPATH layout) | n/a |
| Dependencies | none | none | none | cryptography |
| Licence | Apache-2.0 | MIT | Apache-2.0 | MIT |

Pick this module when you want a tested protocol layer under your own
device model, with a persistent client, product helpers and a simulator
for your tests. Pick tiny-go when you want tinytuya's API shape in Go,
including the cloud wizard. Pick tinytuya itself outside Go: it is the
most widely used implementation and the source of most of what is known
about the protocol.
Binozo/GoTuya (Go, last change 2026-05-15) was also checked: it has no
session key code, so it speaks 3.3 only, and it carries no licence file.
The hass-localtuya integration (Python, GPL-3.0) was read for the
sub-device online list's answer fields only.

## Install

```
go get github.com/cB-Sigma/tuyalan@latest
```

Requires Go 1.22 or later.

## Quick start

```go
import "github.com/cB-Sigma/tuyalan"

heard, _ := tuyalan.Hear(ctx, "", tuyalan.DiscoveryPorts, 6*time.Second)
h := heard[deviceID]
c, err := tuyalan.Dial(ctx, h.Addr(), h.Version, deviceID, []byte(localKey), 5*time.Second)
_ = c.Query()
m, _ := c.Recv()
dps, _ := tuyalan.DPS(m.Payload)
err = c.Control(map[string]any{"1": true})
```

Asking 3.5 devices to announce themselves, scanning, a gateway's
sub-device and a bulb:

```go
heard, _ := tuyalan.Discover(ctx, tuyalan.DiscoverOptions{}) // listens and sends REQ_DEVINFO
open, _ := tuyalan.Scan(ctx, []string{"192.168.1.0/24"}, tuyalan.ScanOptions{})
id, _ := tuyalan.Identify(ctx, open[0], deviceID, []byte(localKey), 3*time.Second)

sub := c.Sub("a4c1380000000001") // c is a session with the gateway
_ = sub.Control(map[string]any{"1": true})

layout, _ := tuyalan.DetectBulb(dps)
colour, _ := layout.SetRGB(255, 120, 0)
_ = c.Control(colour)
```

Finding which of your keys a scanned device holds, a gateway's
sub-device online list, and a persistent client:

```go
id, which, _ := tuyalan.IdentifyWithKeys(ctx, open[0], deviceID, keys, 3*time.Second)

_ = c.SubdevQuery() // then read LAN_EXT_STREAM answers with tuyalan.ParseSubdevStatus

dev := tuyalan.NewDevice(tuyalan.DeviceOptions{Addr: addr, Version: "3.4", DevID: deviceID,
	LocalKey: []byte(localKey), OnChange: func(dps map[string]any) { /* a push */ }})
_ = dev.Open(ctx) // reconnects and sends heartbeats until Close
_ = dev.Set(map[string]any{"1": true})
```

A product helper and an IR code:

```go
import "github.com/cB-Sigma/tuyalan/contrib"

speed, _ := contrib.DefaultFan.SetSpeed(4)
_ = dev.Set(speed)

head, key, _ := tuyalan.IRPulsesToHeadKey(tuyalan.IRNECToPulses(4, 8), 0.1, 38)
send, _ := contrib.IRSendHeadKey(1, head, key) // a control type 1 blaster
_ = dev.Set(send)
```

The local key comes from the Tuya cloud (the device's details) or from
the brand app; this module does not fetch it. `Example` in
example_test.go shows the whole flow (it needs a device, so it is not
run); `ExampleParseBroadcast` runs. [tuyalan-sim](https://github.com/cB-Sigma/tuyalan-sim)
runs the same flow against a simulated plug.

## Documentation

API reference: https://pkg.go.dev/github.com/cB-Sigma/tuyalan

| Package | What it does |
|---|---|
| `tuyalan` (module root) | `Dial` (a session for one version; negotiates 3.4 and 3.5), `Conn` (`Query`, `QueryByControl`, `Control`, `ControlSeq`, `Heartbeat`, `Recv`), `DPS`; frames (`Encode55AA`, `Decode55AA`, `Encode6699`, `Decode6699`, `ReadFrame`), `Codec`, `SessionKey`; discovery (`ParseBroadcast`, `EncodeBroadcast`, `ListenDiscovery`, `Hear`, `Discover`, `RequestDevInfo`, `EncodeDevInfoRequest`, `ParseDevInfoRequest`); scanning (`Scan`, `ExpandTargets`, `Identify`, `IdentifyWithKeys`); gateways (`Conn.Sub`, `Sub`, `CID`, `UpdateDPS`, `Conn.SubdevQuery`, `ParseSubdevStatus`); `Conn.DetectAvailableDPS`; the persistent client (`NewDevice`, `Device`: `Open`, `Status`, `Set`, `Cache`, `Connected`, `Close`); data point helpers (`BulbLayout`, `DetectBulb`, `ColourHexRGB`, `ColourHexHSV`, `ParseColourHex`, `Cover`, `DetectCoverScheme`, `Countdown`, `CountdownDP`); infrared codes (`IRPulsesToBase64`, `IRBase64ToPulses`, `IRPulsesToHex`, `IRHexToPulses`, `IRNECToPulses`, `IRPulsesToNEC`, `IRSamsungToPulses`, `IRPulsesToSamsung`, `IRWidthEncodeToPulses`, `IRPulsesToWidthEncoded`, `IRBuildHead`, `IRPulsesToHeadKey`, `IRHeadKeyToPulses`, `IRProntoToPulses`, `IRPulsesToPronto`, `IRProntoToHeadKey`) |
| `contrib` | product helpers: `Socket`, `TemperatureController`, `ReadDualMeter`, SORIA frames (`DecodeSoriaFull`, `DecodeSoriaRealtime`, `DecodeSoriaCircuits`), `Climate`, `TowelRail`, `HeatPump`, `Blanket`, `Fan`, the thermostat (`ThermostatStateOf`, `Thermostat`, `ThermostatSchedule`, `ParseThermostatSensors`), `Presence`, `Doorbell`, `LEDController`, IR blasters (`IRControlType`, `IRSendCode`, `IRSendHeadKey`, `IRCommand`, `IRLearned`), RF blasters (`RFStudy`, `RFSendButton`, `RFSendKeys`), each with a `Default...` layout |
| `interop` (nested module, tests only) | this client against a device built on tinytuya's frame, cipher, session-key and UDP code; the helpers, the IR conversions and the `contrib` package against tinytuya's bulb, cover, IR remote and Contrib classes, in a container |

Changes per version are in [CHANGELOG.md](CHANGELOG.md).

## Testing

```
go test -race ./...
cd interop && SIGMA_TUYALAN_INTEROP=1 go test -v ./...   # needs Docker
```

The unit tests check frames, ciphers and session keys against vectors
computed independently (PROVENANCE.md). Tests carry `// Covers: SIG-...`
comments: requirement IDs of the SigmaM2M project's compliance report,
with no meaning outside it; the tests added in v0.2.0 and v0.3.0 carry
none yet (they are not claimed in that report). The session tests
against the simulated device, including scanning, `Identify`,
`IdentifyWithKeys`, `Discover`, gateways and their online list,
`DetectAvailableDPS`, the persistent `Device` (with a dropped network)
the helpers driving simulated bulbs and covers, and the `contrib`
helpers driving a simulated fan, air conditioner, thermostat and IR
blasters, are in tuyalan-sim. The IR and `contrib` helpers are checked
here against values worked out by hand or with Python's struct module,
not with tinytuya.

`interop/` is a nested module with its own go.mod. Its tests build an
image from `interop/peer` (python:3.12.10-slim with tinytuya 1.20.0 from
PyPI) and are skipped unless `SIGMA_TUYALAN_INTEROP=1` is set and Docker
answers.

### Interop tested against

- tinytuya 1.20.0 (2026-10-11, python:3.12.10-slim in Docker Desktop
  on arm64; rerun on that image for v0.2.0, after v0.1.0 had run on
  python:3.14-slim): this client against a device whose frames, ciphers,
  discovery announcements and session key are tinytuya's own code
  (`interop/peer/device.py`; which command answers what is the script's).
  Passed for 3.1, 3.2, 3.3, 3.4 and 3.5: the session key negotiation
  (3.4, 3.5), reading every data point (3.2 by null-value CONTROL_NEW),
  a CONTROL or CONTROL_NEW that the device decoded with tinytuya's code
  and acknowledged with our sequence number, the status push after it, a
  heartbeat, a push started at the device, and the version's discovery
  announcement read by `ParseBroadcast`. A wrong local key fails the 3.4
  and 3.5 negotiation.
- Same image, v0.2.0 additions, every version: sub-device query,
  control and UPDATE_DPS requests and the gateway's UPDATE_DPS have the
  command and the JSON keys (with the cid, ctype and protocol values) of
  the requests tinytuya's own `generate_payload` builds for a child
  device (3.2 sub-devices are queried by null-value CONTROL_NEW, as
  tinytuya does); the tinytuya-built gateway answered them, and its
  answers and pushes carry the cid that `CID` reads. `Discover`'s
  REQ_DEVINFO (3.4 and 3.5 device scripts) was decoded by tinytuya's
  `decrypt_udp` as command 0x25 from "app", and the announcement sent
  back, packed by tinytuya, was heard through Docker's UDP port mapping.
  The helpers agree with tinytuya's `BulbDevice` and `CoverDevice` on
  ten colours (both hex formats, read back both ways), the three bulb
  layouts, bulb detection on five sample data point sets, and the values
  of all eight cover schemes.
- Same image, v0.3.0 additions (2026-10-12, Docker Engine 29.4.3 on
  arm64; the whole suite, v0.1.0 and v0.2.0 tests included, was rerun
  and passed): `SubdevQuery` has the command (0x40) and JSON keys of
  tinytuya's own `subdev_query` request, and the script, answering with
  tinytuya's frame and cipher code, was read by `ParseSubdevStatus` for
  3.3, 3.4 and 3.5 (the answer's field names are LocalTuya's, not
  tinytuya's, which returns the answer raw). `IdentifyWithKeys` picked
  the right key after two wrong ones for 3.3, 3.4 and 3.5 and found no
  match with only wrong keys. `DetectAvailableDPS` found the script's
  data points (3.3, 3.5). The persistent `Device` read, set, heard a push
  and sent heartbeats (3.3, 3.4, 3.5). The IR helpers agree with
  tinytuya's `IRRemoteControlDevice` on base64 and hex packing of two
  pulse lists, NEC (address and command, and a raw 32-bit code) and
  Samsung encoding, and NEC and Samsung decoding. One test flaked on its
  first run and was fixed in the test, not the library: Docker's port
  proxy can connect a closed warm-up session to the container after the
  next one, and the script pushes on its newest session.
- Same image, later on 2026-10-12, the IR head/key, Pronto and
  `contrib` additions (the whole suite rerun and passed): on 68 seeded
  inputs (clean and jittered NEC and Samsung codes, repeated frames, odd
  lengths, short and random learned-like lists, at fudge 0.1 and 38 kHz
  and at 0.2 and 36 kHz), `IRPulsesToHeadKey` gives the head and key
  tinytuya's `pulses_to_head_key` gives, and `IRHeadKeyToPulses` expands
  them to tinytuya's pulses; Pronto both ways and `IRProntoToHeadKey`
  agree on 10 codes, and `IRBuildHead` on 3 heads. The `contrib` helpers
  were compared with tinytuya's 15 Contrib classes, whose sockets were
  replaced by a recorder (so the controls are tinytuya's own
  `generate_payload` output, sent nowhere): 119 of 119 calls agree,
  including every refusal and the exact JSON text of the IR and RF
  commands; the state readers agree on the samples of 9 classes; the
  thermostat's schedule (encoded in C and F, decoded, and a 4-period
  schedule), its 3 sensor lists (decoded, re-encoded, a sensor renamed)
  and its resolution detection (4 cases), and the SORIA inverter's 4
  frames, agree with tinytuya's decoders. (The simulator tests added
  with the `contrib` helpers found that the persistent `Device.Status`
  could return a change push still in flight instead of the query's
  answer; fixed before release.) One deliberate difference: given a list
  of RF keys, tinytuya's `rf_send_key` names each one key1, so only the
  last survives; `RFSendKeys` numbers them key1, key2 and so on.
- tinytuya 1.20.0's client against the simulator, in
  [tuyalan-sim](https://github.com/cB-Sigma/tuyalan-sim).
- Not yet: a real Tuya device (needs hardware: a cheap 3.3 plug, a 3.5
  device, a 3.4 Zigbee gateway and an IR blaster). No open-source Tuya
  device emulator was found (tinytuya's own tests replace the socket),
  so the device side of these tests is a script on tinytuya's code, not
  a device. `Scan` ran only against the simulator.

## Not yet

| What | Why |
|---|---|
| Tuya cloud client (OpenAPI with HMAC signing) and fetching local keys | needs a Tuya IoT Platform developer account to build and test against; not done without one |
| A run against real devices (3.3 and 3.5 plugs, a gateway, an IR blaster) | needs hardware |
| Scenes stored on a gateway (SCENE_EXECUTE) | no implementation or public description shows the request or the answer; not guessed |
| Verifying a 3.1 device's key in `IdentifyWithKeys` | a 3.1 device answers its query in clear; tinytuya checks the key by provoking an encrypted CONTROL error, which needs the error's exact form from a real 3.1 device |
| Checking the `contrib` layouts against the real products | each comes from one community report about one product; needs the hardware |
| tinytuya's `pulses_to_space_encoded_head_key` | an unfinished helper in tinytuya (it prints its input and is not used by its class); left out until it is finished there |
| The thermostat's vacation data point (121) as a structure | tinytuya keeps it as raw bytes and no description of its layout was found; `ThermostatState.Vacation` gives the bytes |

## Related repositories

- [tuyalan-sim](https://github.com/cB-Sigma/tuyalan-sim): a simulated
  Tuya device of every protocol version, as a Go package and a command.

## Status

v0.x: the API may change before v1.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md).

## Security

See [SECURITY.md](SECURITY.md) to report a vulnerability privately.

## Licence

Apache-2.0 (see [LICENSE](LICENSE) and [NOTICE](NOTICE)). Copyright 2026
Abhinav Gautam. Tuya is a trademark of its owner; the name is used only
to say which devices this module talks to.
