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

import tinytuya
from tinytuya import BulbDevice, CoverDevice

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
print("PEER vectors " + json.dumps(out, sort_keys=True), flush=True)
