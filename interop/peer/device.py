# Copyright 2026 Abhinav Gautam
# SPDX-License-Identifier: Apache-2.0
#
# A Tuya LAN device for interop tests, built on tinytuya's own frame and
# cipher code (tinytuya.core.message_helper.pack_message/unpack_message,
# tinytuya.core.crypto_helper.AESCipher, and tinytuya's session key
# finalisation), so that a client is checked against tinytuya's reading of
# the protocol, not against ours. The device logic (which command answers
# what) is this script's. It is a test peer, not a device emulator.
#
#   python device.py <version> <device id> <local key> [<cid>,<cid>...]
#
# Listens on TCP 6668 and on UDP 7000 (REQ_DEVINFO requests, decoded with
# tinytuya's udp_helper.decrypt_udp and answered with this version's
# announcement to the request's source). With cids it is a gateway whose
# sub-devices are addressed by "cid" in requests; answers and pushes about
# them carry the cid. Prints "PEER <event> <json>" lines; reads commands
# on standard input: "push <json dps>" sends a status push, "pushsub <cid>
# <json dps>" one of a sub-device, "bcast" prints the discovery
# announcement of this version as hex, "payloads <cid>" prints the
# requests tinytuya itself builds for a sub-device (generate_payload),
# "quit".

import json
import os
import socket
import struct
import sys
import threading
import time
import hmac
from hashlib import md5, sha256

import tinytuya
from tinytuya.core import command_types as CT
from tinytuya.core import header as H
from tinytuya.core.crypto_helper import AESCipher
from tinytuya.core.message_helper import TuyaMessage, pack_message, parse_header, unpack_message
from tinytuya.core.udp_helper import decrypt_udp, udpkey

VERSION, DEV_ID, LOCAL_KEY = sys.argv[1], sys.argv[2], sys.argv[3].encode()
V = float(VERSION)
dps = {"1": False, "2": 100}
SUBS = {c: {"1": False, "19": 0} for c in (sys.argv[4].split(",") if len(sys.argv) > 4 and sys.argv[4] else [])}
lock = threading.Lock()
conns = []


def say(event, obj=None):
    line = "PEER " + event
    if obj is not None:
        line += " " + json.dumps(obj, sort_keys=True)
    print(line, flush=True)


class Session:
    def __init__(self, sock):
        self.sock = sock
        self.key = LOCAL_KEY
        self.seq = 100
        self.nonce = None
        self.dev_nonce = None
        self.wlock = threading.Lock()

    def header(self):
        return VERSION.encode() + H.PROTOCOL_3x_HEADER

    def seal(self, cmd, plain):
        """The payload of a frame this device sends."""
        if V >= 3.5:
            if cmd in (CT.STATUS, CT.CONTROL, CT.CONTROL_NEW) and plain:
                return self.header() + plain
            return plain
        if V >= 3.4:
            if cmd in (CT.STATUS, CT.CONTROL, CT.CONTROL_NEW) and plain:
                plain = self.header() + plain
            return AESCipher(self.key).encrypt(plain, False) if plain or cmd == CT.SESS_KEY_NEG_RESP else b""
        if V >= 3.2:
            if not plain:
                return b""
            ct = AESCipher(self.key).encrypt(plain, False)
            if cmd == CT.STATUS:
                return self.header() + ct
            return ct
        # 3.1: status pushes are signed and encrypted, other answers clear
        if cmd == CT.STATUS:
            b64 = AESCipher(self.key).encrypt(plain)
            sig = md5(b"data=" + b64 + b"||lpv=3.1||" + self.key).hexdigest()[8:24].encode()
            return b"3.1" + sig + b64
        return plain

    def send(self, cmd, plain, seq=None, retcode=0):
        if seq is None:
            self.seq += 1
            seq = self.seq
        payload = self.seal(cmd, plain)
        if V >= 3.5:
            data = pack_message(TuyaMessage(seq, cmd, retcode, payload, 0, True, H.PREFIX_6699_VALUE, True), hmac_key=self.key)
        else:
            body = struct.pack(">I", retcode) + payload
            data = pack_message(TuyaMessage(seq, cmd, retcode, body, 0, True, H.PREFIX_55AA_VALUE, None),
                                hmac_key=self.key if V >= 3.4 else None)
        with self.wlock:
            self.sock.sendall(data)

    def open(self, msg):
        """The plain payload of a frame the client sent."""
        p = msg.payload
        if V >= 3.5:
            pass
        elif V >= 3.4:
            p = AESCipher(self.key).decrypt(p, False, decode_text=False) if p else p
        elif V >= 3.2:
            if p[:3] == VERSION.encode():
                p = p[15:]
            p = AESCipher(self.key).decrypt(p, False, decode_text=False) if p else p
        else:
            if p[:3] == b"3.1":
                b64 = p[19:]
                sig = md5(b"data=" + b64 + b"||lpv=3.1||" + self.key).hexdigest()[8:24].encode()
                if sig != p[3:19]:
                    raise ValueError("3.1 signature does not match")
                p = AESCipher(self.key).decrypt(b64, True, decode_text=False)
        if V >= 3.4 and p[:3] == VERSION.encode():
            p = p[15:]
        return p

    def serve(self):
        self.pending = b""
        while True:
            got = self.read_frame_pending()
            if got is None:
                return
            data, hdr = got
            key = self.key if V >= 3.4 else None
            msg = unpack_message(data, hmac_key=key, header=hdr, no_retcode=True)
            if not msg.crc_good:
                say("bad-integrity", {"cmd": msg.cmd})
                return
            p = self.open(msg)
            self.handle(msg, p)

    def read_frame_pending(self):
        buf = self.pending
        self.pending = b""
        while True:
            if len(buf) >= 18:
                try:
                    hdr = parse_header(buf)
                except Exception as e:
                    say("bad-frame", {"error": str(e)})
                    return None
                if len(buf) >= hdr.total_length:
                    self.pending = buf[hdr.total_length:]
                    return buf[:hdr.total_length], hdr
            chunk = self.sock.recv(4096)
            if not chunk:
                return None
            buf += chunk

    def handle(self, msg, p):
        cmd = msg.cmd
        if cmd == CT.SESS_KEY_NEG_START:
            self.nonce = p[:16]
            self.dev_nonce = os.urandom(16)
            proof = hmac.new(LOCAL_KEY, self.nonce, sha256).digest()
            self.send(CT.SESS_KEY_NEG_RESP, self.dev_nonce + proof)
            say("negotiate-start")
            return
        if cmd == CT.SESS_KEY_NEG_FINISH:
            want = hmac.new(LOCAL_KEY, self.dev_nonce, sha256).digest()
            if p[:32] != want:
                say("negotiate-bad-proof")
                self.sock.close()
                return
            # tinytuya's own session key derivation, run on the device side.
            t = tinytuya.Device(DEV_ID, "127.0.0.1", LOCAL_KEY.decode(), version=V)
            t.local_nonce, t.remote_nonce = self.nonce, self.dev_nonce
            t._negotiate_session_key_generate_finalize()
            self.key = t.local_key
            say("negotiated", {"session_key": self.key.hex()})
            return
        if cmd == CT.HEART_BEAT:
            say("heartbeat")
            self.send(CT.HEART_BEAT, b"", seq=msg.seqno)
            return
        try:
            req = json.loads(p.decode()) if p else {}
        except Exception:
            say("bad-json", {"cmd": cmd, "hex": p.hex()})
            return
        data = req.get("data") if isinstance(req.get("data"), dict) else {}
        cid = req.get("cid") or data.get("cid")
        if cid and cid not in SUBS:
            say("unknown-cid", {"cmd": cmd, "cid": cid})
            self.send(cmd, b"", seq=msg.seqno, retcode=1)
            return
        target = SUBS[cid] if cid else dps
        if cmd in (CT.DP_QUERY, CT.DP_QUERY_NEW):
            say("query", {"cmd": cmd, "req": req})
            if V >= 3.2 and V < 3.3 and cmd == CT.DP_QUERY and not cid:
                self.send(cmd, b"json obj data unvalid", seq=msg.seqno)
                return
            with lock:
                body = {"devId": DEV_ID, "dps": dict(target), "t": int(time.time())}
            if cid:
                body["cid"] = cid
            self.send(cmd, json.dumps(body).encode(), seq=msg.seqno)
            return
        if cmd == CT.UPDATEDPS:
            say("update", {"cmd": cmd, "req": req})
            self.send(cmd, b"", seq=msg.seqno)
            with lock:
                vals = {str(k): target[str(k)] for k in req.get("dpId", []) if str(k) in target}
            if vals:
                self.push(vals, cid)
            return
        if cmd in (CT.CONTROL, CT.CONTROL_NEW):
            asked = req.get("dps") or data.get("dps") or {}
            say("control", {"cmd": cmd, "dps": asked, "protocol": req.get("protocol"), "req": req})
            query, changed = [], {}
            with lock:
                for k, v in asked.items():
                    if v is None:
                        query.append(k)
                    elif k in target:
                        target[k] = v
                        changed[k] = v
            self.send(cmd, b"", seq=msg.seqno)
            if query:
                with lock:
                    self.push({k: target[k] for k in query if k in target}, cid)
            if changed:
                self.push(changed, cid)
            return
        say("unknown", {"cmd": cmd})

    def push(self, values, cid=None):
        if V >= 3.4:
            body = {"protocol": 4, "t": int(time.time()), "data": {"dps": values}}
            if cid:
                body["data"]["cid"] = cid
        else:
            body = {"devId": DEV_ID, "dps": values, "t": int(time.time())}
            if cid:
                body["cid"] = cid
        self.send(CT.STATUS, json.dumps(body).encode())


def broadcast():
    body = json.dumps({"ip": "192.168.1.30", "gwId": DEV_ID, "active": 2, "ability": 0, "mode": 0,
                       "encrypt": VERSION != "3.1", "productKey": "keypeer", "version": VERSION}).encode()
    if VERSION == "3.1":
        return pack_message(TuyaMessage(0, 0, 0, struct.pack(">I", 0) + body, 0, True, H.PREFIX_55AA_VALUE, None))
    if VERSION == "3.5":
        return pack_message(TuyaMessage(0, CT.UDP_NEW, 0, body, 0, True, H.PREFIX_6699_VALUE, True), hmac_key=udpkey)
    ct = AESCipher(udpkey).encrypt(body, False)
    return pack_message(TuyaMessage(0, CT.UDP_NEW, 0, struct.pack(">I", 0) + ct, 0, True, H.PREFIX_55AA_VALUE, None))


def devinfo_loop(u):
    while True:
        data, src = u.recvfrom(4096)
        try:
            req = json.loads(decrypt_udp(data))
        except Exception as e:
            say("devinfo-bad", {"error": repr(e), "hex": data.hex()})
            continue
        say("devinfo-request", {"req": req, "cmd": parse_header(data).cmd})
        u.sendto(broadcast(), src)


def child_payloads(cid):
    """The requests tinytuya builds for a sub-device of this gateway."""
    # Port 1: tinytuya probes 3.2 devices for their data points when the
    # version is set; the probe must fail at once rather than reach us.
    gw = tinytuya.Device(DEV_ID, "127.0.0.1", LOCAL_KEY.decode(), version=V, port=1,
                         connection_retry_limit=1, connection_retry_delay=0)
    child = tinytuya.Device("childdevice00001", cid=cid, parent=gw)
    child.set_dpsUsed({"1": None, "19": None})
    out = {}
    for name, cmd, data in (("control", CT.CONTROL, {"1": True}), ("query", CT.DP_QUERY, None), ("update", CT.UPDATEDPS, [19])):
        m = child.generate_payload(cmd, data)
        out[name] = {"cmd": m.cmd, "json": json.loads(m.payload)}
    m = gw.generate_payload(CT.UPDATEDPS, [18, 19, 20])
    out["gw-update"] = {"cmd": m.cmd, "json": json.loads(m.payload)}
    return out


def accept_loop(srv):
    while True:
        c, _ = srv.accept()
        s = Session(c)
        conns.append(s)
        say("connected")
        threading.Thread(target=run_session, args=(s,), daemon=True).start()


def run_session(s):
    try:
        s.serve()
    except Exception as e:
        say("session-error", {"error": repr(e)})
    finally:
        try:
            s.sock.close()
        except Exception:
            pass
        say("closed")


def main():
    srv = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    srv.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
    srv.bind(("0.0.0.0", 6668))
    srv.listen(4)
    threading.Thread(target=accept_loop, args=(srv,), daemon=True).start()
    u = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
    u.bind(("0.0.0.0", 7000))
    threading.Thread(target=devinfo_loop, args=(u,), daemon=True).start()
    say("ready", {"tinytuya": tinytuya.__version__, "version": VERSION})
    for line in sys.stdin:
        parts = line.strip().split(" ", 1)
        if parts[0] == "quit":
            return
        if parts[0] == "push" and conns:
            values = json.loads(parts[1])
            with lock:
                dps.update(values)
            conns[-1].push(values)
            say("pushed", values)
        if parts[0] == "pushsub" and conns:
            cid, js = parts[1].split(" ", 1)
            values = json.loads(js)
            with lock:
                SUBS[cid].update(values)
            conns[-1].push(values, cid)
            say("pushed", values)
        if parts[0] == "payloads":
            say("payloads", child_payloads(parts[1]))
        if parts[0] == "bcast":
            say("bcast", {"hex": broadcast().hex()})


main()
