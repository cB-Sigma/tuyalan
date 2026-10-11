# Copyright 2026 Abhinav Gautam
# SPDX-License-Identifier: Apache-2.0
#
# Prints, as one "PEER contrib <json>" line, what tinytuya's Contrib
# device classes send and read, so the tuyalan contrib helpers can be
# compared with them. Each class gets a recorder in place of its socket:
# every control is captured as the data points tinytuya's own
# generate_payload put in it (or null when the class refused the call),
# and status() returns a sample. Nothing is sent anywhere.
#
#   python contrib.py

import base64
import json
import random
import struct

import tinytuya
from tinytuya.Contrib.AtorchTemperatureControllerDevice import AtorchTemperatureControllerDevice
from tinytuya.Contrib.BlanketDevice import BlanketDevice
from tinytuya.Contrib.ClimateDevice import ClimateDevice
from tinytuya.Contrib.ColorfulX7Device import ColorfulX7Device
from tinytuya.Contrib.DoorbellDevice import DoorbellDevice
from tinytuya.Contrib.FloorFanDevice import FloorFanDevice
from tinytuya.Contrib.InverterHeatPumpDevice import InverterHeatPumpDevice
from tinytuya.Contrib.IRRemoteControlDevice import IRRemoteControlDevice
from tinytuya.Contrib.PresenceDetectorDevice import PresenceDetectorDevice
from tinytuya.Contrib.RFRemoteControlDevice import RFRemoteControlDevice
from tinytuya.Contrib.SocketDevice import SocketDevice
from tinytuya.Contrib.SoriaInverterDevice import SoriaInverterDevice
from tinytuya.Contrib.ThermostatDevice import ThermostatDevice
from tinytuya.Contrib.TowelRailHeaterDevice import TowelRailHeaterDevice
from tinytuya.Contrib.WiFiDualMeterDevice import WiFiDualMeterDevice

out = {"tinytuya": tinytuya.__version__, "calls": {}, "reads": {}}


def device(cls, sample=None, **kw):
    kw.setdefault("version", 3.3)
    d = cls("contribtest00001", "127.0.0.1", "0123456789abcdef", **kw)
    d.captured = []

    def capture(payload, *args, **kwargs):
        if payload is not None:
            d.captured.append(json.loads(payload.payload).get("dps"))
        return None
    d._send_receive = capture
    if sample is not None:
        d.status = lambda *a, **k: {"dps": dict(sample)}
    return d


def call(name, d, fn):
    d.captured = []
    try:
        fn()
    except Exception as e:  # tinytuya refuses some values by raising
        out["calls"][name] = {"refused": repr(e)}
        return
    if not d.captured:
        out["calls"][name] = None
    elif len(d.captured) == 1:
        out["calls"][name] = d.captured[0]
    else:
        out["calls"][name] = {"several": d.captured}


# Socket and temperature controller (read only).
SOCKET = {"1": True, "18": 245, "19": 553, "20": 2301}
s = device(SocketDevice, SOCKET)
out["reads"]["socket"] = {"sample": SOCKET, "energy": s.get_energy_consumption(), "state": s.get_state()}
ATORCH = {"101": "socket", "102": 235, "103": True, "108": 1200, "109": 27612, "110": 23012, "111": 15, "118": "c"}
a = device(AtorchTemperatureControllerDevice, ATORCH)
out["reads"]["atorch"] = {"sample": ATORCH, "energy": a.get_energy_consumption(), "temp": a.get_temp(),
                          "state": a.get_state(), "total": a.get_total_power()}

# Two-channel meter.
DUAL = {"1": 12345, "2": 67, "101": 1234, "102": "FORWARD", "104": "REVERSE", "105": 22, "110": 98,
        "111": 5001, "112": 2305, "113": 5500, "115": 1256, "116": 1000, "129": 10}
m = device(WiFiDualMeterDevice, DUAL)
out["reads"]["dual"] = {"sample": DUAL, "values": {k: m.get_value(k) for k in DUAL}}

# Climate.
cl = device(ClimateDevice, {"1": True, "2": 24, "3": 27, "4": "cold", "5": "auto", "19": "c", "22": 0, "25": False, "30": True, "101": "cold"})
for name, fn in [("climate.temp", lambda: cl.set_target_temperature(22.5)), ("climate.mode", lambda: cl.set_operating_mode("hot")),
                 ("climate.mode.bad", lambda: cl.set_operating_mode("fan")), ("climate.fan", lambda: cl.set_fan_speed("middle")),
                 ("climate.fan.bad", lambda: cl.set_fan_speed("turbo")), ("climate.timer", lambda: cl.set_timer(5)),
                 ("climate.timer.bad", lambda: cl.set_timer(25)), ("climate.unit", lambda: cl.set_temperature_unit("f")),
                 ("climate.on", lambda: cl.turn_on())]:
    call(name, cl, fn)

# Towel rail.
TOWEL = {"1": True, "16": 215, "24": 198, "2": "hot", "111": 15}
tr = device(TowelRailHeaterDevice, TOWEL)
out["reads"]["towel"] = {"sample": TOWEL, "json": tr.status_json()}
for name, fn in [("towel.temp", lambda: tr.set_target_temperature(21.25)), ("towel.temp2", lambda: tr.set_target_temperature(21.35)),
                 ("towel.mode", lambda: tr.set_operating_mode("eco")), ("towel.mode.bad", lambda: tr.set_operating_mode("max")),
                 ("towel.timer", lambda: tr.set_timer(90)), ("towel.timer.bad", lambda: tr.set_timer(45)),
                 ("towel.timer.big", lambda: tr.set_timer(510))]:
    call(name, tr, fn)

# Heat pump.
HP = {"1": True, "102": 26, "103": True, "104": 80, "105": "warm", "106": 28, "107": 18, "108": 40, "115": 0, "116": 0, "117": False}
hp = device(InverterHeatPumpDevice, HP)
out["reads"]["heatpump"] = {"sample": HP, "on": hp.is_on(), "unit": hp.get_unit().value, "inlet": hp.get_inlet_water_temp(),
                            "target": hp.get_target_water_temp(), "min": hp.get_lower_limit_target_water_temp(),
                            "max": hp.get_upper_limit_target_water_temp(), "capacity": hp.get_heating_capacity_percent(),
                            "mode": hp.get_mode(raw=True), "fault": hp.get_fault(raw=True), "silence": hp.is_silence_mode()}
from tinytuya.Contrib.InverterHeatPumpDevice import TemperatureUnit
for name, fn in [("heatpump.unit.f", lambda: hp.set_unit(TemperatureUnit.FAHRENHEIT)), ("heatpump.target", lambda: hp.set_target_water_temp(30)),
                 ("heatpump.target.bad", lambda: hp.set_target_water_temp(45)), ("heatpump.silence.on", lambda: hp.set_silence_mode(True)),
                 ("heatpump.silence.off", lambda: hp.set_silence_mode(False))]:
    call(name, hp, fn)

# Blanket.
BL = {"14": "level_3", "15": "level_1", "16": "4h", "17": "12h", "18": "02:10", "19": "00:00"}
bl = device(BlanketDevice, BL)
out["reads"]["blanket"] = {"sample": BL, "body": bl.get_body_level(), "feet": bl.get_feet_level(),
                           "bodytime": bl.get_body_time(), "feettime": bl.get_feet_time()}
for name, fn in [("blanket.body", lambda: bl.set_body_level(6)), ("blanket.feet", lambda: bl.set_feet_level(0)),
                 ("blanket.body.bad", lambda: bl.set_body_level(7)), ("blanket.bodytime", lambda: bl.set_body_time(8)),
                 ("blanket.feettime.bad", lambda: bl.set_feet_time(13))]:
    call(name, bl, fn)

# Floor fan.
FAN = {"1": True, "2": "nature", "3": 4, "5": True, "22": "2h"}
ff = device(FloorFanDevice, FAN)
out["reads"]["fan"] = {"sample": FAN, "json": ff.status_json()}
ffe = device(FloorFanDevice, {})
out["reads"]["fan.empty"] = {"power": ffe.get_power(), "mode": ffe.get_mode(), "speed": ffe.get_speed(),
                             "osc": ffe.get_oscillation(), "timer": ffe.get_timer()}
for name, fn in [("fan.on", lambda: ff.set_power(True)), ("fan.mode", lambda: ff.set_mode("sleep")), ("fan.mode.bad", lambda: ff.set_mode("turbo")),
                 ("fan.speed", lambda: ff.set_speed(5)), ("fan.speed.bad", lambda: ff.set_speed(6)), ("fan.osc", lambda: ff.set_oscillation(False)),
                 ("fan.timer", lambda: ff.set_timer("12h")), ("fan.timer.bad", lambda: ff.set_timer("13h"))]:
    call(name, ff, fn)

# Presence sensor.
PR = {"1": "presence", "2": 7, "3": 0, "4": 600, "6": "check_success", "9": 154, "101": 2, "102": 30, "104": 120}
pr = device(PresenceDetectorDevice, PR)
out["reads"]["presence"] = {"sample": PR, "json": json.loads(pr.status_json())}
for name, fn in [("presence.sens", lambda: pr.set_sensitivity(5)), ("presence.near", lambda: pr.set_near_detection(30)),
                 ("presence.far", lambda: pr.set_far_detection(450))]:
    call(name, pr, fn)

# Doorbell.
db = device(DoorbellDevice, {})
for name, fn in [("door.indicator", lambda: db.set_basic_indicator(False)), ("door.vol.low", lambda: db.set_volume(1)),
                 ("door.vol.high", lambda: db.set_volume(14)), ("door.vol", lambda: db.set_volume(7)),
                 ("door.area", lambda: db.set_motion_area(10, 20, 50, 60)), ("door.area.over", lambda: db.set_motion_area(60, 50, 50, 60)),
                 ("door.area.clamp", lambda: db.set_motion_area(-5, 120, 200, -3)), ("door.areaswitch", lambda: db.set_motion_area_switch(True))]:
    call(name, db, fn)

# LED controller.
x7 = device(ColorfulX7Device, {}, version=3.3)
for name, fn in [("x7.on", lambda: x7.switch_on()), ("x7.off", lambda: x7.switch_off()), ("x7.mode", lambda: x7.set_mode("music")),
                 ("x7.mode.bad", lambda: x7.set_mode("disco")), ("x7.colour", lambda: x7.set_color(10, 200, 30)),
                 ("x7.countdown", lambda: x7.set_countdown(3600)), ("x7.countdown.bad", lambda: x7.set_countdown(90000)),
                 ("x7.seg", lambda: x7.set_segments_number(64)), ("x7.seg.bad", lambda: x7.set_segments_number(0)),
                 ("x7.leds", lambda: x7.set_leds_PerSegment(150)), ("x7.order", lambda: x7.set_rgb_order("ORDER_GRB")),
                 ("x7.work", lambda: x7.set_work_mode("DYNAMIC")), ("x7.rgb", lambda: x7.set_color_rgb(255, 16, 1)),
                 ("x7.bright", lambda: x7.set_brightness(55)), ("x7.speed", lambda: x7.set_speed(100)),
                 ("x7.dyn", lambda: x7.set_dynamic_mode(180)), ("x7.dyn.bad", lambda: x7.set_dynamic_mode(181)),
                 ("x7.music", lambda: x7.set_music_mode(22)), ("x7.sens", lambda: x7.set_sensitivity(0)),
                 ("x7.musiccolour", lambda: x7.set_music_RGBColor(1, 2, 3)), ("x7.brand", lambda: x7.set_led_brand("DMX512")),
                 ("x7.screen", lambda: x7.set_screen_mode(30)), ("x7.dot", lambda: x7.set_fallingDot_color(200, 100, 50))]:
    call(name, x7, fn)

# IR blasters (both control types) and RF.
ir1 = device(IRRemoteControlDevice, {}, control_type=1)
ir2 = device(IRRemoteControlDevice, {}, control_type=2)
CODE = IRRemoteControlDevice.pulses_to_base64(IRRemoteControlDevice.nec_to_pulses(4, 8))
HEAD, KEY = IRRemoteControlDevice.pulses_to_head_key(IRRemoteControlDevice.nec_to_pulses(4, 8))
out["ir"] = {"code": CODE, "head": HEAD, "key": KEY}
for t, d in ((1, ir1), (2, ir2)):
    for name, fn in [("send", lambda: d.send_button(CODE)), ("key", lambda: d.send_key(HEAD, KEY)),
                     ("study", lambda: d.study_start()), ("exit", lambda: d.study_end())]:
        call("ir%d.%s" % (t, name), d, fn)
rf = device(RFRemoteControlDevice, {}, control_type=1)
BUTTON = base64.b64encode(json.dumps({"study_feq": "433", "ver": "2", "code": "abc"}).encode()).decode()
BANKS = dict(cmt_bank=[0, 102, 236, 28, 240, 128, 20, 8, 145, 2, 2, 208], system_bank=[174, 224, 53, 0, 0, 244, 16, 226, 66, 32, 0, 129],
             frequency_bank=[66, 113, 206, 28, 66, 91, 28, 28], datarate_bank=[63, 30, 128, 204, 0, 0, 0, 0, 0, 0, 0, 41, 192, 218, 33, 75, 5, 0, 80, 45, 0, 1, 5, 5],
             baseband_bank=[2, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 19, 0, 0, 0, 0, 0, 0, 0, 0, 96, 255, 0, 0, 31, 16], tx_bank=[81, 154, 12, 0, 12, 176, 0, 31, 4, 63, 127])
out["rf"] = {"button": BUTTON, "banks": BANKS}
for name, fn in [("rf.study", lambda: rf.rf_study_start(freq=433)), ("rf.study.short", lambda: rf.rf_study_start(short=True)),
                 ("rf.exit", lambda: rf.rf_study_end(freq=433)), ("rf.exit.short", lambda: rf.rf_study_end(short=True)),
                 ("rf.send", lambda: rf.rf_send_button(BUTTON)), ("rf.send.plain", lambda: rf.rf_send_button("bm90anNvbg==", times=3, delay=1, intervals=2)),
                 ("rf.key", lambda: rf.rf_send_key("ffffc01fa49349", **BANKS)), ("rf.key.bad", lambda: rf.rf_send_key("ff", **dict(BANKS, tx_bank=[1, 2])))]:
    call(name, rf, fn)

# Solar inverter frames.
rng = random.Random(42)
def frame(prefix, tags, noise=3):
    b = bytearray(rng.randrange(256) for _ in range(noise))
    for t, v in tags:
        b += prefix + bytes([t]) + struct.pack(">H", v)
    return base64.b64encode(bytes(b)).decode()
sor = device(SoriaInverterDevice, None, version=3.5)
P = bytes([1, 1, 16])
frames = {
    "21": frame(P, [(0x31, 320), (0x32, 341), (0x33, 94), (0x07, 2305), (0x1a, 139), (0x1e, 310), (0x23, 5001), (0x4a, 98), (0x57, 412), (0x58, 0), (0x02, 12345), (0x00, 77)]),
    "25": frame(bytes([1, 2, 3]), [(0x49, 300), (0x27, 315)]),
    "24": frame(bytes([1, 1, 0x80]), [(2, 1), (0, 0), (1, 5)], noise=0),
    "21b": frame(P, [(0x32, 341)]),
}
out["soria"] = {"frames": frames, "full": sor._decode_full(frames["21"]), "realtime": sor._decode_realtime(frames["25"]),
                "status": sor._decode_status(frames["24"]), "full_none": sor._decode_full(frames["21b"])}

# Thermostat: setters for both resolutions and units, schedule and sensors.
for hires in (False, True):
    for units in ("c", "f"):
        th = device(ThermostatDevice, {}, persist=True)
        th.high_resolution = hires
        th.temp_unit_convert = units
        for mode in ("cool", "heat", "auto"):
            th.mode = mode
            call("th.%s.%s.%s.setpoint" % (hires, units, mode), th, lambda: th.setSetpoint(21.7))
        call("th.%s.%s.cool" % (hires, units), th, lambda: th.setCoolSetpoint(24.33))
        call("th.%s.%s.heat" % (hires, units), th, lambda: th.setHeatSetpoint(68, cf="f"))
        call("th.%s.%s.middle" % (hires, units), th, lambda: th.setMiddleSetpoint(22.5))
th = device(ThermostatDevice, {}, persist=True)
for name, fn in [("th.mode", lambda: th.setMode("heat")), ("th.fan.true", lambda: th.setFan(True)), ("th.fan.false", lambda: th.setFan(False)),
                 ("th.fan.circ", lambda: th.setFan("cycle")), ("th.units", lambda: th.setUnits("f")), ("th.units.x", lambda: th.setUnits("x")),
                 ("th.sched.on", lambda: th.setSchedule(True)), ("th.hold.true", lambda: th.setHold(True)), ("th.hold.false", lambda: th.setHold(False)),
                 ("th.hold.temp", lambda: th.setHold("temphold")), ("th.fanrt", lambda: th.setFanRuntime(25.9))]:
    call(name, th, fn)

# The resolution a status tells.
res = {}
for name, dps in (("low", {"19": 24, "2": "cool"}), ("high", {"108": 2400, "2": "cool"}), ("none", {"2": "cool"}), ("both", {"110": 75, "26": 20})):
    t2 = device(ThermostatDevice, {}, persist=True)
    t2._process_response({"dps": dps})
    res[name] = t2.high_resolution
out["thermo_res"] = res

# Schedule: set periods, encode in C and F, decode.
sched_items = []
for cf in ("c", "f"):
    t3 = device(ThermostatDevice, {}, persist=True)
    t3.temp_unit_convert = cf
    sch = t3.schedule
    vals = [(0, 0, 6 * 60 + 30, 20.3, 24.76), (0, 1, 8 * 60, 17.0, 27.24), (0, 2, 17 * 60 + 15, 21.25, 23.75), (0, 3, 22 * 60, 18.74, 26.26),
            (3, 0, 7 * 60, 68, 75), (6, 2, 12 * 60, -32768, 22), (5, 3, 1439, 99, -99)]
    if cf == "f":
        vals = [(d, p, t, h, c) for d, p, t, h, c in vals[:4]] + [(3, 0, 7 * 60, 64, 77), (6, 2, 12 * 60, -32768, 72), (5, 3, 1439, 50, 90)]
    for d, p, t, h, c in vals:
        sch.setPeriod(d, p, time=t, heatto=h, coolto=c, participation=0xFF)
    sch.setPeriod(1, 4, time=600, heatto=20, coolto=25, participation=2)
    sch.copyDay(0, 2)
    enc = sch.b64()
    t4 = device(ThermostatDevice, {}, persist=True)
    t4.temp_unit_convert = cf
    t4.schedule.update(base64.b64decode(enc))
    dec = [[[pp.participation, pp.time, pp.heatto, pp.coolto] for pp in day] for day in t4.schedule]
    sched_items.append({"cf": cf, "set": vals, "b64": enc, "decoded": dec})
# A 4-period schedule from a device (196 bytes), decoded in C.
short = bytearray()
for d in range(7):
    for p in range(4):
        short += struct.pack(">BHhh", p, 360 + 240 * p + d, 2000 + 50 * p, 2500 - 50 * d)
t5 = device(ThermostatDevice, {}, persist=True)
t5.schedule.update(bytes(short))
out["schedule"] = {"items": sched_items, "short": base64.b64encode(bytes(short)).decode(),
                   "short_decoded": [[[pp.participation, pp.time, pp.heatto, pp.coolto] for pp in day] for day in t5.schedule]}

# Sensor lists.
from tinytuya.Contrib.ThermostatDevice import ThermostatSensorList
lists = []
for n, counted in ((3, True), (2, False), (1, True)):
    raw = bytearray([n] if counted else [])
    for i in range(n):
        name = ("Sensor %d" % i).encode().rjust(30, b"\0") if i % 2 == 0 else ("Room%d" % i).encode().ljust(30, b"\0")
        temp = 0 if i == 1 else rng.randrange(1500, 2800)
        raw += struct.pack(">I30s??h?BBBB?h6s", rng.randrange(1 << 32), name, True, bool(i % 2), rng.randrange(1500, 2800), i != 2,
                           rng.randrange(16), rng.randrange(101), rng.randrange(1, 40), rng.randrange(256), bool(i % 2), temp, bytes(rng.randrange(256) for _ in range(6)))
    b64 = base64.b64encode(bytes(raw)).decode()
    sl = ThermostatSensorList("122", None)
    sl.update(b64)
    sens = [{"id": s.id, "name": s.name, "enabled": s.enabled, "occupied": s.occupied, "temperature": s.temperature,
             "temperature_used": s.temperature_used, "online": s.online, "participation": s.participation, "battery": s.battery,
             "firmware": s.firmware_version, "unknown2": s.unknown2, "averaging": s.averaging} for s in sl]
    reenc = sl.b64()
    # rename the first sensor as the app would
    s0 = sl.sensors[0]
    s0.name = "Kitchen thermometer number two"
    s0.raw_name = s0.name[:20].encode('utf8').rjust(30, b'\0')
    renamed = sl.b64()
    lists.append({"b64": b64, "stated": sl.stated_count, "sensors": sens, "reencoded": reenc, "renamed": renamed})
out["sensors"] = lists

print("PEER contrib " + json.dumps(out, sort_keys=True), flush=True)
