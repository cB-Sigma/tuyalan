# tuyalan

The local LAN protocol of Tuya-based Wi-Fi devices, versions 3.1 to 3.5, in pure Go; not affiliated with Tuya.

[![Go Reference](https://pkg.go.dev/badge/github.com/cB-Sigma/tuyalan.svg)](https://pkg.go.dev/github.com/cB-Sigma/tuyalan) [![test](https://github.com/cB-Sigma/tuyalan/actions/workflows/test.yml/badge.svg)](https://github.com/cB-Sigma/tuyalan/actions/workflows/test.yml) [![Licence: Apache-2.0](https://img.shields.io/badge/licence-Apache--2.0-blue.svg)](LICENSE) [![Go](https://img.shields.io/badge/go-1.22-00ADD8.svg)](go.mod)

It speaks to a device with its local key: 55AA frames with CRC32 or
HMAC-SHA256, 6699 frames with AES-GCM, the 3.4 and 3.5 session key
negotiation, reading and setting data points, status pushes and
heartbeats, the UDP discovery announcements and REQ_DEVINFO requests, a
TCP scan, gateway sub-devices, and helpers for bulb, cover and countdown
data points. It does not use the Tuya cloud. Standard library only, no cgo. It was written for SigmaM2M,
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
| Scenes stored on a gateway (SCENE_EXECUTE), the sub-device online list (LAN_EXT_STREAM) | no (the command numbers are known, the answers are not documented; not guessed) |
| The cloud API, fetching local keys, pairing | no (needs a Tuya IoT Platform developer account; see Not yet) |

## Choosing a Go Tuya LAN library

Compared from each project's source at the version named, on
2026-10-11. Check the projects themselves before relying on a row.

| | this module | MXVXID/tiny-go v1.0.1 | py60800/tuya (2025-09-29) | tinytuya 1.20.0 (Python) |
|---|---|---|---|---|
| Protocol versions | 3.1 to 3.5 | 3.1 to 3.5 | 3.1 (its README) | 3.1 to 3.5 |
| Discovery: listen on 6666, 6667, 7000 | yes | yes | yes | yes |
| Active 3.5 discovery (REQ_DEVINFO), TCP force scan | yes (the scan finds open ports; `Identify` needs the device's key, it does not try a key list) | yes | no | yes |
| Gateway sub-devices, timers, bulb and cover helpers | yes (data point maps for your `Control`; no device classes) | yes | switches only | yes |
| Tuya cloud client and local-key wizard | no (SigmaM2M keeps its own) | yes | no | yes |
| Device simulator for tests | yes, every version, separate module | a loopback fake in its tests | no | no |
| Interop evidence | against tinytuya both ways, every version (below) | states byte parity with tinytuya | none stated | the reference most others follow |
| API shape | a `Conn` per device; frames, codec and session key exported | a port of tinytuya's API | device manager | Python |
| Go version | 1.22 | 1.26.5 | no go.mod (GOPATH layout) | n/a |
| Dependencies | none | none | none | cryptography |
| Licence | Apache-2.0 | MIT | Apache-2.0 | MIT |

Pick this module when you want a small, tested protocol layer under
your own device model, with a simulator for your tests. Pick tiny-go
when you want tinytuya's full feature set in Go, including the cloud
wizard, scanning with a list of keys and device classes in tinytuya's
shape. Pick
tinytuya itself outside Go: it is the most widely used implementation
and the source of most of what is known about the protocol.

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

The local key comes from the Tuya cloud (the device's details) or from
the brand app; this module does not fetch it. `Example` in
example_test.go shows the whole flow (it needs a device, so it is not
run); `ExampleParseBroadcast` runs. [tuyalan-sim](https://github.com/cB-Sigma/tuyalan-sim)
runs the same flow against a simulated plug.

## Documentation

API reference: https://pkg.go.dev/github.com/cB-Sigma/tuyalan

| Package | What it does |
|---|---|
| `tuyalan` (module root) | `Dial` (a session for one version; negotiates 3.4 and 3.5), `Conn` (`Query`, `QueryByControl`, `Control`, `ControlSeq`, `Heartbeat`, `Recv`), `DPS`; frames (`Encode55AA`, `Decode55AA`, `Encode6699`, `Decode6699`, `ReadFrame`), `Codec`, `SessionKey`; discovery (`ParseBroadcast`, `EncodeBroadcast`, `ListenDiscovery`, `Hear`, `Discover`, `RequestDevInfo`, `EncodeDevInfoRequest`, `ParseDevInfoRequest`); scanning (`Scan`, `ExpandTargets`, `Identify`); gateways (`Conn.Sub`, `Sub`, `CID`, `UpdateDPS`); data point helpers (`BulbLayout`, `DetectBulb`, `ColourHexRGB`, `ColourHexHSV`, `ParseColourHex`, `Cover`, `DetectCoverScheme`, `Countdown`, `CountdownDP`) |
| `interop` (nested module, tests only) | this client against a device built on tinytuya's frame, cipher, session-key and UDP code, and the helpers against tinytuya's bulb and cover classes, in a container |

Changes per version are in [CHANGELOG.md](CHANGELOG.md).

## Testing

```
go test -race ./...
cd interop && SIGMA_TUYALAN_INTEROP=1 go test -v ./...   # needs Docker
```

The unit tests check frames, ciphers and session keys against vectors
computed independently (PROVENANCE.md). Tests carry `// Covers: SIG-...`
comments: requirement IDs of the SigmaM2M project's compliance report,
with no meaning outside it; the tests added in v0.2.0 carry none yet
(they are not claimed in that report). The session tests against the
simulated device, including scanning, `Identify`, `Discover`, gateways
and the helpers driving simulated bulbs and covers, are in tuyalan-sim.

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
- tinytuya 1.20.0's client against the simulator, in
  [tuyalan-sim](https://github.com/cB-Sigma/tuyalan-sim).
- Not yet: a real Tuya device (needs hardware: a cheap 3.3 plug and a
  3.5 device). No open-source Tuya device emulator was found (tinytuya's
  own tests replace the socket), so the device side of these tests is a
  script on tinytuya's code, not a device. `Scan` and `Identify` ran
  only against the simulator.

## Not yet

| What | Why |
|---|---|
| Tuya cloud client (OpenAPI with HMAC signing) and fetching local keys | needs a Tuya IoT Platform developer account to build and test against; not done without one |
| A run against real devices (3.3 and 3.5) | needs hardware |
| Scenes stored on a gateway, the sub-device online list | their answers are not described publicly; not guessed |
| Trying a list of keys during a scan, as tinytuya's force scan does | `Identify` takes one device's key; loop over your keys if you need it |

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
