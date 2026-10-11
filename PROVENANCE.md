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

## Sources

| Area (files) | Followed | Read for facts only (no code taken) |
|---|---|---|
| Frames, integrity, encryption (tuyalan.go) | none published | tinytuya (MIT): frame layouts of 55AA and 6699, command numbers, CRC32/HMAC rules, AES-ECB and AES-GCM use, the discovery key string |
| Sessions and payloads (session.go) | none published | tinytuya (MIT), tinytuya/core/XenonDevice.py: version headers and which commands carry them, the 3.4/3.5 negotiation and session key, the 3.1 MD5 signature (characters 8 to 23) and base64 ECB for CONTROL only, 3.2 as 3.3 framing with null-value queries |
| Discovery (discovery.go, listen.go) | none published | tinytuya (MIT): ports 6666, 6667 and 7000 and the announcement JSON |
| Interop peer (interop/peer/device.py) | our own script | calls tinytuya 1.20.0's pack_message, unpack_message, parse_header, AESCipher and session key finalisation; installed from PyPI at test time |

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
| interop/ | tinytuya 1.20.0's code, at test time |

All device IDs, local keys and addresses are made-up demo values.
