package tuyalan

import (
	"encoding/binary"
	"encoding/json"
	"errors"
)

// Broadcast is a device's discovery announcement.
type Broadcast struct {
	IP         string `json:"ip"`
	GwID       string `json:"gwId"`
	Active     int    `json:"active,omitempty"`
	Ability    int    `json:"ability,omitempty"`
	Mode       int    `json:"mode,omitempty"`
	Encrypt    bool   `json:"encrypt,omitempty"`
	ProductKey string `json:"productKey,omitempty"`
	Version    string `json:"version"`
	// Encrypted reports whether the announcement was encrypted (3.3 and
	// later) rather than plain JSON (3.1).
	Encrypted bool `json:"-"`
}

// ParseBroadcast decodes a discovery datagram: a 55AA frame with plain
// JSON (3.1, port 6666) or JSON under AES-ECB with UDPKey (3.3 and 3.4,
// port 6667), or a 6699 frame under AES-GCM with UDPKey (3.5). The frame
// check is lenient (a CRC or an HMAC under UDPKey), since only the JSON
// matters and the source address is what is trusted anyway.
func ParseBroadcast(b []byte) (Broadcast, error) {
	var out Broadcast
	if len(b) < 4 {
		return out, errors.New("datagram too short")
	}
	var payload []byte
	switch binary.BigEndian.Uint32(b) {
	case Prefix6699:
		m, err := Decode6699(b, UDPKey, true)
		if err != nil {
			return out, err
		}
		payload, out.Encrypted = m.Payload, true
	case Prefix55AA:
		m, err := Decode55AA(b, CRC, nil, true)
		if err != nil {
			m, err = Decode55AA(b, HMAC, UDPKey, true)
		}
		if err != nil {
			return out, err
		}
		payload = m.Payload
		if !json.Valid(payload) {
			p, err := ECBDecrypt(UDPKey, payload)
			if err != nil {
				return out, err
			}
			payload, out.Encrypted = p, true
		}
	default:
		return out, errors.New("not a Tuya discovery datagram")
	}
	if err := json.Unmarshal(payload, &out); err != nil {
		return out, errors.New("discovery payload is not JSON")
	}
	if out.GwID == "" {
		return out, errors.New("discovery payload without gwId")
	}
	return out, nil
}

// EncodeBroadcast builds the datagram a device of the given version sends
// (the sim package uses it).
func EncodeBroadcast(b Broadcast) ([]byte, error) {
	j, err := json.Marshal(b)
	if err != nil {
		return nil, err
	}
	switch b.Version {
	case "3.1":
		return Encode55AA(Message{Cmd: CmdUDP, HasRet: true, Payload: j}, CRC, nil), nil
	case "3.5":
		return Encode6699(Message{Cmd: CmdUDPNew, HasRet: true, Payload: j}, UDPKey)
	default:
		ct, err := ECBEncrypt(UDPKey, j, true)
		if err != nil {
			return nil, err
		}
		return Encode55AA(Message{Cmd: CmdUDPNew, HasRet: true, Payload: ct}, CRC, nil), nil
	}
}
