# Copyright 2026 Abhinav Gautam
# SPDX-License-Identifier: Apache-2.0
#
# Prints, as one "PEER vectors <json>" line, what tinytuya's own bulb and
# cover classes compute for a set of inputs, so the tuyalan helpers can be
# compared with them: colour hex values in both formats, colours read
# back, the bulb data point layouts, bulb detection from sample data
# points, and the cover value schemes.
#
#   python vectors.py

import json
import random

import tinytuya
from tinytuya import BulbDevice, CoverDevice
from tinytuya.Contrib.IRRemoteControlDevice import IRRemoteControlDevice as IR

COLOURS = [(255, 0, 0), (0, 255, 0), (0, 0, 255), (10, 20, 200), (255, 128, 0), (200, 10, 100), (1, 2, 3), (255, 255, 255), (0, 0, 0), (128, 64, 32)]
SAMPLES = {
    "v2": {"20": True, "21": "white", "22": 750, "23": 278, "24": "00f003e803e8", "25": "000e0d00", "26": 0},
    "v2white": {"20": True, "21": "white", "22": 60},
    "v1": {"1": True, "2": "colour", "3": 255, "4": 0, "5": "ff00000000ffff"},
    "dimmer": {"1": True, "2": 25, "3": 0},
    "notbulb": {"1": True, "20": True, "21": "white"},
}

out = {"tinytuya": tinytuya.__version__, "colours": [], "layouts": {}, "detect": {}, "covers": {}}
for r, g, b in COLOURS:
    rgb8 = BulbDevice.rgb_to_hexvalue(r, g, b, "rgb8")
    hsv16 = BulbDevice.rgb_to_hexvalue(r, g, b, "hsv16")
    out["colours"].append({
        "rgb": [r, g, b], "rgb8": rgb8, "hsv16": hsv16,
        "back16": list(BulbDevice.hexvalue_to_rgb(hsv16, "hsv16")),
        "hsv_of_rgb8": list(BulbDevice.hexvalue_to_hsv(rgb8, "rgb8")),
    })
for name, dpset in BulbDevice.DEFAULT_DPSET.items():
    out["layouts"][name] = dpset
for name, dps in SAMPLES.items():
    bd = BulbDevice("bulbvectors00001", "127.0.0.1", "0123456789abcdef", version=3.3)
    bd.detect_bulb(response={"dps": dps})
    out["detect"][name] = {"type": bd.bulb_type, "dpset": bd.dpset}
for k, v in CoverDevice.COVER_TYPES.items():
    out["covers"][str(k)] = {"open": v["open"], "close": v["close"], "stop": v["stop"], "continue": v["continue"]}

# Infrared remote conversions, from tinytuya's IRRemoteControlDevice.
IR_PULSES = [[9000, 4500, 563, 1688, 563, 563], [4500, 4500, 560, 560, 560, 1690, 560, 560]]
out["ir"] = {
    "pulse_sets": IR_PULSES,
    "base64": [IR.pulses_to_base64(p) for p in IR_PULSES],
    "hex": [IR.pulses_to_hex(p) for p in IR_PULSES],
    "nec": {"4,8": IR.nec_to_pulses(4, 8), "raw": IR.nec_to_pulses(0x20DF10EF)},
    "samsung": {"7,2": IR.samsung_to_pulses(7, 2)},
    "nec_decode": IR.pulses_to_nec(IR.nec_to_pulses(4, 8)),
    "samsung_decode": IR.pulses_to_samsung(IR.samsung_to_pulses(7, 2)),
}

# Head/key packing and Pronto, over seeded inputs: clean NEC and Samsung
# codes, the same with jitter (as a learned code has), repeated frames,
# odd lengths and short lists.
rng = random.Random(20261012)
def jitter(p, amount):
    return [max(1, x + rng.randint(-amount, amount)) for x in p]
sets = [
    IR.nec_to_pulses(4, 8), IR.nec_to_pulses(0x20DF10EF), IR.samsung_to_pulses(7, 2),
    IR.nec_to_pulses(0xFF00FF00), IR.nec_to_pulses(0), IR.nec_to_pulses(0xFFFFFFFF),
    IR.nec_to_pulses(4, 8) + IR.nec_to_pulses(4, 8),
    [9000, 4500, 563, 1688, 563, 563, 563],
    [500, 500], [300, 900, 300, 900, 900, 300],
]
for _ in range(12):
    sets.append(jitter(IR.nec_to_pulses(rng.randint(0, 0xFFFFFFFF)), 60))
for _ in range(6):
    sets.append(jitter(IR.samsung_to_pulses(rng.randint(0, 255), rng.randint(0, 255)), 80))
for _ in range(6):
    n = rng.randint(4, 40) * 2
    sets.append([rng.choice([400, 450, 800, 1200, 1600, 2400, 3000]) + rng.randint(-20, 20) for _ in range(n)])
hk = []
for p in sets:
    for fudge, freq in ((0.1, 38), (0.2, 36)):
        r = IR.pulses_to_head_key(p, fudge=fudge, freq=freq)
        item = {"pulses": p, "fudge": fudge, "freq": freq, "head": None, "key": None, "back": None}
        if r:
            item["head"], item["key"] = r
            item["back"] = IR.head_key_to_pulses(r[0], r[1])
        hk.append(item)
pr = []
for p in sets[:10]:
    code = IR.pulses_to_pronto(p)
    item = {"pulses": p, "pronto": code, "back": IR.pronto_to_pulses(code)}
    r = IR.pronto_to_head_key(code)
    item["head"], item["key"] = (r if r else (None, None))
    pr.append(item)
out["headkey"] = {"items": hk, "pronto": pr,
                  "build": [IR.build_head(), IR.build_head(freq=36, bit_time_type=2, timings=[560, 560, 1690, 9000, 4500]),
                            IR.build_head(freq=38.5, timings=[600, 1700], convert_time=False)]}
print("PEER vectors " + json.dumps(out, sort_keys=True), flush=True)
