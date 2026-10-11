// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package tuyalan

import (
	"encoding/json"
	"time"
)

// UpdateDPS asks the device to refresh the listed data points
// (UPDATE_DPS; metering plugs refresh 18, 19 and 20 so). The device
// acknowledges it and pushes the values that changed as status pushes.
func (c *Conn) UpdateDPS(ids []int) error {
	return c.send(CmdUpdateDPs, updatePayload(ids, ""))
}

func updatePayload(ids []int, cid string) []byte {
	if ids == nil {
		ids = []int{}
	}
	m := map[string]any{"dpId": ids}
	if cid != "" {
		m["cid"] = cid
	}
	b, _ := json.Marshal(m)
	return b
}

// Sub addresses one sub-device of a gateway (a Zigbee or Bluetooth device
// behind a Wi-Fi gateway) by its node ID ("cid"), over the gateway's
// session. Requests carry the cid in the places tinytuya puts it; answers
// and pushes about the sub-device carry it too (see CID).
type Sub struct {
	c   *Conn
	cid string
}

// Sub returns the sub-device with node ID cid of the gateway c is a
// session with.
func (c *Conn) Sub(cid string) *Sub { return &Sub{c: c, cid: cid} }

// CID is the sub-device's node ID.
func (s *Sub) CID() string { return s.cid }

// Query asks for the sub-device's data points: DP_QUERY with the cid
// (3.1 to 3.3), DP_QUERY_NEW with the cid (3.4, 3.5).
func (s *Sub) Query() error {
	if Legacy(s.c.codec.Version) {
		b, _ := json.Marshal(map[string]any{"cid": s.cid, "t": time.Now().Unix()})
		return s.c.send(CmdDPQuery, b)
	}
	b, _ := json.Marshal(map[string]any{"cid": s.cid})
	return s.c.send(CmdDPQueryNew, b)
}

// QueryByControl asks for the listed data points of the sub-device with
// CONTROL_NEW and null values, the way tinytuya queries sub-devices of a
// 3.2 gateway (it treats every 3.2 device as one that refuses DP_QUERY).
func (s *Sub) QueryByControl(ids []string) error {
	dps := map[string]any{}
	for _, id := range ids {
		dps[id] = nil
	}
	b, _ := json.Marshal(map[string]any{"cid": s.cid, "devId": s.cid, "uid": s.cid, "t": now(), "dps": dps})
	return s.c.send(CmdControlNew, b)
}

// Control sets the sub-device's data points.
func (s *Sub) Control(dps map[string]any) error {
	_, _, err := s.ControlSeq(dps)
	return err
}

// ControlSeq sets the sub-device's data points and returns the command
// and sequence number sent: CONTROL with the cid beside "dps" (3.1 to
// 3.3), CONTROL_NEW with the cid both at the top and in "data", with
// "ctype" 0 (3.4, 3.5).
func (s *Sub) ControlSeq(dps map[string]any) (cmd, seq uint32, err error) {
	t := time.Now().Unix()
	if Legacy(s.c.codec.Version) {
		b, _ := json.Marshal(map[string]any{"cid": s.cid, "t": t, "dps": dps})
		seq, err = s.c.sendSeq(CmdControl, b)
		return CmdControl, seq, err
	}
	b, _ := json.Marshal(map[string]any{"protocol": 5, "t": t, "cid": s.cid,
		"data": map[string]any{"cid": s.cid, "ctype": 0, "dps": dps}})
	seq, err = s.c.sendSeq(CmdControlNew, b)
	return CmdControlNew, seq, err
}

// UpdateDPS asks the gateway to refresh the sub-device's listed data
// points.
func (s *Sub) UpdateDPS(ids []int) error {
	return s.c.send(CmdUpdateDPs, updatePayload(ids, s.cid))
}

// SubdevQuery asks a gateway for its sub-devices' online state: a
// LAN_EXT_STREAM request whose JSON is
// {"reqType":"subdev_online_stat_query","data":{"cids":[]}} (the shape
// tinytuya's subdev_query builds). The gateway answers with a
// LAN_EXT_STREAM frame; ParseSubdevStatus reads it. 3.3 gateways do not
// answer this; 3.4 and 3.5 gateways do.
func (c *Conn) SubdevQuery() error {
	b, _ := json.Marshal(map[string]any{
		"reqType": "subdev_online_stat_query",
		"data":    map[string]any{"cids": []string{}},
	})
	return c.send(CmdLanExtStream, b)
}

// SubdevStatus is a gateway's answer to SubdevQuery: the node IDs of the
// sub-devices it reports online, offline and nearby (an answer may carry
// only some of these, and a gateway with many sub-devices may split them
// across several answers).
type SubdevStatus struct {
	Online  []string
	Offline []string
	Nearby  []string
}

// ParseSubdevStatus reads a LAN_EXT_STREAM answer's payload into a
// SubdevStatus. The lists come from "data.online", "data.offline" and
// "data.nearby" (the fields a gateway reports, as observed by the LocalTuya
// project). ok is false when the payload is not such an answer.
func ParseSubdevStatus(payload []byte) (SubdevStatus, bool) {
	if len(payload) == 0 || payload[0] != '{' {
		return SubdevStatus{}, false
	}
	var m struct {
		Data struct {
			Online  []string `json:"online"`
			Offline []string `json:"offline"`
			Nearby  []string `json:"nearby"`
		} `json:"data"`
	}
	if json.Unmarshal(payload, &m) != nil {
		return SubdevStatus{}, false
	}
	if m.Data.Online == nil && m.Data.Offline == nil && m.Data.Nearby == nil {
		return SubdevStatus{}, false
	}
	return SubdevStatus{Online: m.Data.Online, Offline: m.Data.Offline, Nearby: m.Data.Nearby}, true
}

// CID returns the sub-device node ID a decoded payload is about: a
// top-level "cid" or "data.cid"; "" for the gateway itself or a plain
// device.
func CID(payload []byte) string {
	if len(payload) == 0 || payload[0] != '{' {
		return ""
	}
	var m struct {
		CID  any             `json:"cid"`
		Data json.RawMessage `json:"data"`
	}
	if json.Unmarshal(payload, &m) != nil {
		return ""
	}
	if s, ok := m.CID.(string); ok && s != "" {
		return s
	}
	var d struct {
		CID any `json:"cid"`
	}
	if len(m.Data) > 0 && m.Data[0] == '{' && json.Unmarshal(m.Data, &d) == nil {
		if s, ok := d.CID.(string); ok {
			return s
		}
	}
	return ""
}
