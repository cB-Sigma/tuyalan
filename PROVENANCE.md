# Provenance

## How the code was written

Written for SigmaM2M, a oneM2M platform, in Go. No code was ported or
copied from another implementation, and no source file of another
project was translated.

Tuya does not publish its LAN protocol. The facts this module uses were
taken from public descriptions of the protocol, chiefly the tinytuya
project (MIT): the 55AA and 6699 frame layouts, the command numbers,
CRC32 and HMAC-SHA256 integrity, AES-ECB payload encryption with the
local key, the AES-GCM framing of 3.5, the version headers, the 3.4 and
3.5 session key negotiation, the 3.1 signature rule, the "device22"
query behaviour and the discovery key. These are interface facts that
every implementation must use. The Go types, the codec and session
structure, the error handling, the discovery listener and the simulator
are our own design.

History:

- The 3.3 to 3.5 code came with SigmaM2M's first commit (2026-09-27).
- 3.1 and 3.2 were added on 2026-10-05, after reading tinytuya's
  tinytuya/core/XenonDevice.py for the facts; the 3.1 signature test
  vector was computed independently with Python's hashlib.
- listen.go was written for this module on 2026-10-06.
- 2026-10-11: the simulator, its command and the session tests that
  drive it moved to the tuyalan-sim module (with `git mv`), so the
  library does not depend on its simulator. The interop module was added
  and run against tinytuya 1.20.0. One difference was found and fixed:
  UPDATE_DPS payloads no longer carry a version header, as tinytuya
  sends them (`TestPayloadVersionHeaders`).
- 2026-10-11, v0.2.0: active.go, scan.go, gateway.go and dp.go were
  written for this module, after reading tinytuya 1.20.0 (installed from
  PyPI into the interop image and read there) for behaviour only:
  tinytuya/scanner.py (how the REQ_DEVINFO request is formed and where
  it is sent, the force scan's connect-then-probe approach and its
  parallelism), tinytuya/core/XenonDevice.py (where a child device's
  requests carry the cid, per version; the UPDATE_DPS request; how 3.2
  devices are queried), tinytuya/core/udp_helper.py (trailing NUL bytes
  in app datagrams), tinytuya/BulbDevice.py (the three bulb layouts, the
  detection rule, the two colour hex formats and value ranges, scene
  selection), tinytuya/CoverDevice.py (the eight cover value schemes and
  the detection order) and tinytuya/core/Device.py (the countdown data
  point guess). The Go API (data point maps rather than device classes,
  `Discover`, `Scan`, `Identify`, `Sub`) and all code are our own; the
  colour conversion follows the published algorithm of Python's
  colorsys, written out in Go. The interop peer was moved to
  python:3.12.10-slim and rerun on it.
- 2026-10-11/12, v0.3.0: device.go, detect.go and ir.go were written for
  this module, and `IdentifyWithKeys` (scan.go) and `SubdevQuery` /
  `ParseSubdevStatus` (gateway.go) added, after reading for behaviour
  only: tinytuya 1.20.0's sdist from PyPI and its GitHub master
  (tinytuya/core/XenonDevice.py: the subdev_query request, the
  detect_available_dps ranges, the device22 switch; tinytuya/scanner.py:
  the force scan trying a key list; tinytuya/Contrib/IRRemoteControlDevice.py:
  the little-endian pulse packing, the NEC and Samsung bit orders and
  timings), hass-localtuya (xZetsubou, GPL-3.0, master on 2026-10-11:
  core/pytuya/__init__.py, only for the field names of a gateway's answer
  to the sub-device online query: "online", "offline", "nearby" in
  "data", and that 3.3 gateways do not answer it), and tiny-go v1.0.1
  (MIT; its Monitor and ForceScan, for the comparison only). No code was
  copied or translated; the persistent client's design (one reader per
  session dispatching to waiters, a supervisor that reconnects) is our
  own.
- 2026-10-12, v0.3.0 continued: ir_headkey.go and the contrib package
  were written after reading tinytuya 1.20.0 (sdist from PyPI) for
  behaviour: Contrib/IRRemoteControlDevice.py (the head layout, the key's
  symbol and bit-field rules, the packer's steps and tie-breaking, the
  Pronto conversions, the type 1 and type 2 command data points) and the
  other 14 Contrib classes (AtorchTemperatureController, Blanket,
  Climate, ColorfulX7, Doorbell, FloorFan, InverterHeatPump,
  PresenceDetector, RFRemoteControl, Socket, SoriaInverter, Thermostat,
  TowelRailHeater, WiFiDualMeter: their data point numbers, value
  ranges, scalings, refusals, the thermostat's schedule and sensor record
  layouts, the SORIA frame layout), plus core/XenonDevice.py for the
  removal of spaces from every payload. The packer is reproduced step for
  step, because a heuristic agrees with tinytuya only if it makes the same
  choices; the Go is written anew (Python's insertion-ordered dicts are
  modelled with explicit order, and Python's round() with round half to
  even). The Go types (layout structs, State readers, setters returning
  maps) are our own design.

## Sources

| Area (files) | Followed | Read for facts only (no code taken) |
|---|---|---|
| Frames, integrity, encryption (tuyalan.go) | none published | tinytuya (MIT): frame layouts of 55AA and 6699, command numbers, CRC32/HMAC rules, AES-ECB and AES-GCM use, the discovery key string |
| Sessions and payloads (session.go) | none published | tinytuya (MIT), tinytuya/core/XenonDevice.py: version headers and which commands carry them, the 3.4/3.5 negotiation and session key, the 3.1 MD5 signature (characters 8 to 23) and base64 ECB for CONTROL only, 3.2 as 3.3 framing with null-value queries |
| Discovery (discovery.go, listen.go) | none published | tinytuya (MIT): ports 6666, 6667 and 7000 and the announcement JSON |
| Active discovery and scanning (active.go, scan.go) | none published | tinytuya 1.20.0 (MIT), scanner.py and core/udp_helper.py: REQ_DEVINFO as a 6699 datagram under the discovery key, command 0x25, no return code, JSON naming the app and its address, sent to UDP 7000; TCP 6668 probed in parallel |
| Gateways and UPDATE_DPS (gateway.go) | none published | tinytuya 1.20.0 (MIT), core/XenonDevice.py: the cid beside "dps" for 3.1 to 3.3, at the top and in "data" with ctype 0 for 3.4 and 3.5, the cid in answers at the top or in "data"; UPDATE_DPS as a dpId list |
| Data point helpers (dp.go) | Python's colorsys conversion (an algorithm, written out in Go) | tinytuya 1.20.0 (MIT), BulbDevice.py, CoverDevice.py, core/Device.py: data point numbers of the bulb layouts, value ranges, the two hex colour formats, scene forms, detection rules, the cover value schemes and detection order, the countdown guess |
| Key list, sub-device online list, device22 probing (scan.go, gateway.go, detect.go) | none published | tinytuya 1.20.0 (MIT), scanner.py and core/XenonDevice.py: trying keys in turn, the LAN_EXT_STREAM command and its `subdev_online_stat_query` request with an empty cid list and no version header, the data point ranges; hass-localtuya (GPL-3.0): the answer's "online", "offline" and "nearby" lists |
| Persistent client (device.go) | none | tinytuya and tiny-go only for the feature list (reconnect, heartbeat, cache, callbacks) |
| Infrared codes (ir.go) | none published | tinytuya 1.20.0 (MIT), Contrib/IRRemoteControlDevice.py: 16-bit little-endian pulse packing in base64 and hex, the optional leading "1", NEC and Samsung timings, bit order and complement checks, the 75 % to 125 % start-mark window |
| IR head/key and Pronto (ir_headkey.go) | none published | tinytuya 1.20.0 (MIT), Contrib/IRRemoteControlDevice.py: the head's fields, units and symbol list, the key's send count, symbol runs and bit fields, the two head types' bit pairs, the packer's merging (pairwise, 10 % default), lettering, pair counting, symbol assignment and choice of the shorter reading, the Pronto word layout and its 0.241246 time-base factor |
| Product helpers (contrib/) | none published | tinytuya 1.20.0 (MIT), the 15 Contrib classes: data point numbers, enumerated values, ranges and clamps, scalings, the doorbell's motion area text, the thermostat's 7-byte schedule periods and 52-byte sensor records, the SORIA frames' prefix detection and tags, the IR and RF command texts |
| Interop peer (interop/peer/device.py, vectors.py, contrib.py) | our own scripts | call tinytuya 1.20.0's pack_message, unpack_message, parse_header, AESCipher, decrypt_udp, session key finalisation, generate_payload of a child device and of a gateway's LAN_EXT_STREAM request, BulbDevice/CoverDevice class methods and tables, IRRemoteControlDevice's static conversions, and the 15 Contrib classes with a recorder in place of their socket (contrib.py); installed from PyPI at test time |

No specification or vendor text is reproduced; comments describe the
protocol in our own words.

## Notes for reviewers

These are the places closest to a public source. Each is a protocol
fact that an implementation must match to interoperate:

- The command constants in tuyalan.go: the numbers are protocol facts,
  and the names follow the protocol's command names as tinytuya spells
  them, in Go's case.
- `UDPKey` in tuyalan.go: the public string the discovery key is the MD5
  of.
- `Sig31` and the 3.1 branch of `SealPayload` in session.go: the
  signature input and character range.
- `SessionKey` in tuyalan.go: the 3.4 (XOR, then ECB) and 3.5 (XOR, then
  GCM with the client nonce's first 12 bytes as IV) derivations.

tinytuya is MIT; the project owner decided on 2026-10-06 that no
line-by-line comparison is needed, the notes above being the record.

## Test vectors and where they come from

| Test | Source |
|---|---|
| tuyalan_test.go | frames built by hand from the layouts above, with the CRC32 computed independently with Python's zlib; AES from FIPS-197 appendix C.1; ECB and the session keys checked with openssl; the discovery key with md5 |
| legacy_test.go (TestLegacyCodec) | the 3.1 signature vector, computed independently with Python's hashlib |
| v020_test.go (colours) | computed independently with Python's colorsys on 2026-10-11, not with tinytuya |
| v020_test.go (other) | built by hand from the facts above |
| v030_test.go (IR) | computed independently with Python's struct and base64 modules and the NEC algorithm on 2026-10-11, and the head and Pronto values by hand on 2026-10-12, not with tinytuya |
| contrib/contrib_test.go | worked out by hand from the layouts on 2026-10-12, not with tinytuya |
| interop/ | tinytuya 1.20.0's code, at test time (python:3.12.10-slim) |

All device IDs, local keys and addresses are made-up demo values.
