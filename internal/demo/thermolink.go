package demo

import "math"

// ThermoLink is a synthetic protocol of a thermostat controller. A frame is
//
//	A5 | type | seq | len (u16 LE) | payload | sum8(all preceding bytes)
//
// and a response carries the request type with the high bit set.

const (
	devicePort = 5020

	frameMagic    = 0xA5
	responseFlag  = 0x80
	typeReadParam = 0x01
	typeSetParam  = 0x02
	typeMeasure   = 0x03
	typeReadLog   = 0x04
	typeHeartbeat = 0x10
)

// Parameter ids of READ_PARAMS / SET_PARAM.
const (
	paramSetpoint    = 1
	paramHysteresis  = 2
	paramMode        = 3
	paramFan         = 4
	paramInterval    = 5
	paramCalibration = 6
)

// paramName maps a parameter id to the name used in the action log.
var paramName = map[int]string{
	paramSetpoint:    "setpoint",
	paramHysteresis:  "hysteresis",
	paramMode:        "mode",
	paramFan:         "fan",
	paramInterval:    "interval",
	paramCalibration: "calibration",
}

// paramValue is one entry of a READ_PARAMS response.
type paramValue struct {
	id, value int
}

type logEntry struct {
	ts, code, value, flags int
}

func u16(v int) []byte { return []byte{byte(v), byte(v >> 8)} }

func u32(v int) []byte { return []byte{byte(v), byte(v >> 8), byte(v >> 16), byte(v >> 24)} }

func frame(typ, seq int, payload []byte) []byte {
	out := []byte{frameMagic, byte(typ), byte(seq)}
	out = append(out, u16(len(payload))...)
	out = append(out, payload...)
	var sum byte
	for _, b := range out {
		sum += b
	}
	return append(out, sum)
}

func concat(parts ...[]byte) []byte {
	var out []byte
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

func readParamsReq(seq int) []byte { return frame(typeReadParam, seq, nil) }

func readParamsResp(seq int, params ...paramValue) []byte {
	payload := []byte{byte(len(params))}
	for _, p := range params {
		payload = append(payload, byte(p.id))
		payload = append(payload, u16(p.value)...)
	}
	return frame(typeReadParam|responseFlag, seq, payload)
}

func setParamReq(seq, param, value int) []byte {
	return frame(typeSetParam, seq, append([]byte{byte(param)}, u16(value)...))
}

func setParamResp(seq, param int) []byte {
	const statusOK = 0
	return frame(typeSetParam|responseFlag, seq, []byte{statusOK, byte(param)})
}

func getMeasurementReq(seq, channel int) []byte {
	return frame(typeMeasure, seq, []byte{byte(channel)})
}

func getMeasurementResp(seq, channel int, temperature, humidity float64, uptime int) []byte {
	payload := []byte{byte(channel)}
	payload = append(payload, u16(jsRound(temperature*10))...)
	payload = append(payload, u16(jsRound(humidity*10))...)
	payload = append(payload, u32(uptime)...)
	return frame(typeMeasure|responseFlag, seq, payload)
}

func readLogReq(seq, count int) []byte { return frame(typeReadLog, seq, []byte{byte(count)}) }

func readLogResp(seq int, entries []logEntry) []byte {
	payload := []byte{byte(len(entries))}
	for _, e := range entries {
		payload = append(payload, u32(e.ts)...)
		payload = append(payload, byte(e.code))
		payload = append(payload, u16(e.value)...)
		payload = append(payload, byte(e.flags))
	}
	return frame(typeReadLog|responseFlag, seq, payload)
}

func heartbeat(seq int) []byte { return frame(typeHeartbeat, seq, nil) }

func heartbeatAck(seq int) []byte { return frame(typeHeartbeat|responseFlag, seq, nil) }

// jsRound rounds half up like JavaScript's Math.round, which the frontend
// fixtures use to encode fixed-point values.
func jsRound(x float64) int { return int(math.Floor(x + 0.5)) }

// prng is mulberry32: a tiny deterministic generator, identical to the one
// the frontend fixtures use.
type prng struct{ state uint32 }

func newPRNG(seed uint32) *prng { return &prng{state: seed} }

func (r *prng) uint32() uint32 {
	r.state += 0x6d2b79f5
	t := r.state
	t = (t ^ t>>15) * (t | 1)
	t ^= t + (t^t>>7)*(t|61)
	return t ^ t>>14
}

// float returns a value in [0, 1).
func (r *prng) float() float64 { return float64(r.uint32()) / 4294967296 }

// logEntries produces count journal records with increasing timestamps.
func logEntries(count int, seed uint32) []logEntry {
	rnd := newPRNG(seed)
	codes := [...]int{1, 2, 3, 7}
	ts := 1_726_000_000
	entries := make([]logEntry, count)
	for i := range entries {
		ts += 300 + int(math.Floor(float64(rnd.float()*600)))
		e := logEntry{ts: ts, code: codes[int(math.Floor(float64(rnd.float()*4)))]}
		e.value = jsRound(float64(float64(15+float64(rnd.float()*12)) * 10))
		if rnd.float() > 0.8 {
			e.flags = 1
		}
		entries[i] = e
	}
	return entries
}

// noise returns length bytes that look like a TLS record header followed by
// random data. The frontend computes it in floating point, where the
// products exceed 2^53 and lose low bits; the same arithmetic is repeated
// here so that the bytes are identical.
func noise(length int, seed int) []byte {
	const modulus = 1 << 32
	x := float64(seed) * 2654435761
	out := make([]byte, length)
	copy(out, []byte{0x16, 0x03, 0x03})
	for i := 3; i < length; i++ {
		x = math.Mod(float64(x*1103515245)+12345, modulus)
		out[i] = byte(uint32(x) >> 16)
	}
	return out
}
