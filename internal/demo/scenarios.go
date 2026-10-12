package demo

import (
	"net/netip"
	"time"

	"github.com/AVGsync/protolab/internal/model"
)

const (
	second = time.Second
	ms     = time.Millisecond
	us     = time.Microsecond
)

var (
	day1Start = time.Date(2026, 9, 14, 10, 0, 0, 0, time.UTC)
	day2Start = time.Date(2026, 9, 15, 9, 30, 0, 0, time.UTC)
	day3Start = time.Date(2026, 9, 15, 16, 0, 0, 0, time.UTC)

	clientIP = netip.MustParseAddr("192.168.10.12")
	device   = netip.AddrPortFrom(netip.MustParseAddr("192.168.10.50"), devicePort)
	website  = netip.AddrPortFrom(netip.MustParseAddr("93.184.216.34"), 443)
)

func client(port uint16) netip.AddrPort { return netip.AddrPortFrom(clientIP, port) }

// File names of the generated captures.
const (
	day1File = "tc200_actions_2026-09-14.pcapng"
	day2File = "tc200_extra_2026-09-15.pcapng"
	day3File = "tc200_defects.pcap"
)

// exchange is one request and its response. The Split fields list byte
// offsets at which the message is cut into separate TCP segments.
type exchange struct {
	req, resp           []byte
	reqSplit, respSplit []int
	pause               time.Duration // before the request; 40 ms when zero
}

// segmentRef points at the index-th payload packet of one direction.
type segmentRef struct {
	dir   model.Direction
	index int
}

// defects describes what went wrong while the traffic was recorded.
type defects struct {
	// drop removes a segment from the capture file.
	drop *segmentRef
	// truncate marks a segment that does not fit into the snaplen. The cut is
	// made by the file writer; the mark is checked against the frame size.
	truncate *segmentRef
	// retransmit sends a copy of a segment slightly later; with conflict the
	// copy carries a different byte in the middle of the payload.
	retransmit         *segmentRef
	retransmitConflict bool
	// reorder swaps the arrival times of a segment and the one after it.
	reorder *segmentRef
	// dropLeading removes the first packets of the connection from the file.
	dropLeading int
	// badChecksum gives all packets sent in that direction a wrong TCP
	// checksum, as a capture on the sending host with checksum offloading does.
	badChecksum model.Direction
	// noClose omits FIN and the final ACK.
	noClose bool
}

type sessionSpec struct {
	key       string
	client    netip.AddrPort
	server    netip.AddrPort
	start     time.Duration // from the start of the capture
	exchanges []exchange
	defects   defects
	clientISN uint32 // 0 derives the number from the session seed
	serverISN uint32
}

// nonTCPSpec is a frame that is not TCP/IPv4 at a given moment of the capture.
type nonTCPSpec struct {
	at    time.Duration
	frame []byte
}

type captureSpec struct {
	fileName string
	pcapng   bool
	start    time.Time
	snapLen  uint32
	sessions []sessionSpec
	nonTCP   []nonTCPSpec
}

// day1 is the main research capture: one action of the client per session.
func day1() captureSpec {
	return captureSpec{
		fileName: day1File,
		pcapng:   true,
		start:    day1Start,
		snapLen:  262144,
		sessions: []sessionSpec{
			{
				key:    "read-params",
				client: client(51001), server: device,
				start: 5 * second,
				exchanges: []exchange{
					{req: readParamsReq(1), resp: readParamsResp(1, paramValue{paramSetpoint, 22}, paramValue{paramHysteresis, 1}, paramValue{paramMode, 1}, paramValue{paramFan, 2})},
				},
			},
			{
				key:    "set-21",
				client: client(51002), server: device,
				start: 70 * second,
				// The client sequence numbers wrap around 2^32 in the middle of the
				// first request.
				clientISN: 0xFFFFFFFC,
				exchanges: []exchange{
					{req: setParamReq(1, paramSetpoint, 21), resp: setParamResp(1, paramSetpoint)},
					{req: readParamsReq(2), resp: readParamsResp(2, paramValue{paramSetpoint, 21}, paramValue{paramHysteresis, 1}, paramValue{paramMode, 1}, paramValue{paramFan, 2})},
				},
			},
			{
				key:    "set-23",
				client: client(51003), server: device,
				start:     150 * second,
				exchanges: []exchange{{req: setParamReq(1, paramSetpoint, 23), resp: setParamResp(1, paramSetpoint)}},
			},
			{
				key:    "set-18",
				client: client(51004), server: device,
				start: 220 * second,
				// The request crosses a TCP segment boundary.
				exchanges: []exchange{{req: setParamReq(1, paramSetpoint, 18), reqSplit: []int{4}, resp: setParamResp(1, paramSetpoint)}},
			},
			{
				key: "set-mode",
				// The same client port as "set-23": a distinct connection, never merged.
				client: client(51003), server: device,
				start:     290 * second,
				exchanges: []exchange{{req: setParamReq(1, paramMode, 2), resp: setParamResp(1, paramMode)}},
			},
			{
				key:    "meas-1",
				client: client(51006), server: device,
				start: 360 * second,
				// Two messages in one TCP segment (heartbeat + request).
				exchanges: []exchange{
					{
						req:  concat(heartbeat(1), getMeasurementReq(2, 1)),
						resp: concat(heartbeatAck(1), getMeasurementResp(2, 1, 22.4, 41.0, 86_412)),
					},
				},
			},
			{
				key:    "meas-2",
				client: client(51007), server: device,
				start:     420 * second,
				exchanges: []exchange{{req: getMeasurementReq(1, 2), resp: getMeasurementResp(1, 2, 19.8, 55.5, 86_473)}},
			},
			{
				key:    "read-log",
				client: client(51008), server: device,
				start: 480 * second,
				exchanges: []exchange{
					{req: readLogReq(1, 20), resp: readLogResp(1, logEntries(20, 7)), respSplit: []int{64, 128}},
				},
				// The second segment of the response arrives after the third and is
				// then sent once more.
				defects: defects{
					retransmit: &segmentRef{model.S2C, 1},
					reorder:    &segmentRef{model.S2C, 1},
				},
			},
			{
				key:    "tls-noise",
				client: client(51544), server: website,
				start: 200 * second,
				// The client sequence numbers start 256 below 2^32, so the first
				// request, a single segment, crosses the wrap-around point.
				clientISN: 0xFFFFFF00,
				exchanges: []exchange{
					{req: noise(517, 1), resp: noise(1380, 2), respSplit: []int{700}},
					{req: noise(126, 3), resp: noise(240, 4), pause: 90 * ms},
				},
			},
		},
		nonTCP: day1NonTCP(),
	}
}

// day2 is an additional recording with new parameter values.
func day2() captureSpec {
	return captureSpec{
		fileName: day2File,
		pcapng:   true,
		start:    day2Start,
		snapLen:  262144,
		sessions: []sessionSpec{
			{
				key:    "set-interval",
				client: client(52001), server: device,
				start:     10 * second,
				exchanges: []exchange{{req: setParamReq(1, paramInterval, 600), resp: setParamResp(1, paramInterval)}},
			},
			{
				key:    "set-calibration",
				client: client(52002), server: device,
				start:     60 * second,
				exchanges: []exchange{{req: setParamReq(1, paramCalibration, -15), resp: setParamResp(1, paramCalibration)}},
			},
			{
				key:    "meas-outdoor",
				client: client(52003), server: device,
				start:     120 * second,
				exchanges: []exchange{{req: getMeasurementReq(1, 3), resp: getMeasurementResp(1, 3, -5.5, 80.0, 167_020)}},
			},
			{
				key:    "set-25",
				client: client(52004), server: device,
				start:     180 * second,
				exchanges: []exchange{{req: setParamReq(1, paramSetpoint, 25), resp: setParamResp(1, paramSetpoint)}},
			},
			{
				key:    "read-params-2",
				client: client(52005), server: device,
				start: 240 * second,
				exchanges: []exchange{
					{
						req: readParamsReq(1),
						resp: readParamsResp(1,
							paramValue{paramSetpoint, 25}, paramValue{paramHysteresis, 1}, paramValue{paramMode, 2},
							paramValue{paramFan, 2}, paramValue{paramInterval, 600}, paramValue{paramCalibration, -15}),
					},
				},
			},
		},
	}
}

// day3 shows the capture defects. The snaplen of 96 bytes leaves room for
// 42 bytes of payload behind the 54 bytes of Ethernet, IPv4 and TCP headers.
func day3() captureSpec {
	return captureSpec{
		fileName: day3File,
		start:    day3Start,
		snapLen:  96,
		sessions: []sessionSpec{
			{
				key:    "lost-segment",
				client: client(53001), server: device,
				start: 2 * second,
				exchanges: []exchange{
					{req: readLogReq(1, 12), resp: readLogResp(1, logEntries(12, 11)), respSplit: []int{40, 80}},
					{req: readParamsReq(2), resp: readParamsResp(2, paramValue{paramSetpoint, 25}, paramValue{paramHysteresis, 1})},
				},
				defects: defects{drop: &segmentRef{model.S2C, 1}},
			},
			{
				key:    "truncated",
				client: client(53002), server: device,
				start:     30 * second,
				exchanges: []exchange{{req: readLogReq(1, 16), resp: readLogResp(1, logEntries(16, 13))}},
				defects:   defects{truncate: &segmentRef{model.S2C, 0}},
			},
			{
				key:    "conflict",
				client: client(53003), server: device,
				start: 60 * second,
				exchanges: []exchange{
					{req: setParamReq(1, paramSetpoint, 24), resp: setParamResp(1, paramSetpoint)},
					{req: readParamsReq(2), resp: readParamsResp(2, paramValue{paramSetpoint, 24})},
				},
				defects: defects{retransmit: &segmentRef{model.C2S, 0}, retransmitConflict: true},
			},
			{
				key:    "no-handshake",
				client: client(53004), server: device,
				start: 90 * second,
				exchanges: []exchange{
					{req: setParamReq(1, paramFan, 3), reqSplit: []int{5}, resp: setParamResp(1, paramFan)},
					{req: getMeasurementReq(2, 1), resp: getMeasurementResp(2, 1, 23.1, 39.5, 190_001)},
				},
				// SYN, SYN-ACK, ACK and the first half of the request were not captured.
				defects: defects{dropLeading: 4, noClose: true},
			},
			{
				key:    "checksum",
				client: client(53005), server: device,
				start:     120 * second,
				exchanges: []exchange{{req: getMeasurementReq(1, 1), resp: getMeasurementResp(1, 1, 23.4, 39.0, 190_031)}},
				defects:   defects{badChecksum: model.C2S},
			},
		},
	}
}

func captureSpecs() []captureSpec { return []captureSpec{day1(), day2(), day3()} }

// actionLog is the experiment log of the first two days. Each session starts
// a fraction of a second after its action.
func actionLog() []model.CreateActionInput {
	setParam := func(at time.Time, param int, value float64, label, result string) model.CreateActionInput {
		return model.CreateActionInput{
			Timestamp: model.EntityTime(at),
			Kind:      "set_param",
			Label:     label,
			Params:    map[string]any{"param": paramName[param], "paramId": float64(param), "value": value},
			Result:    result,
		}
	}
	measurement := func(at time.Time, channel int, temperature, humidity float64, label, result string) model.CreateActionInput {
		return model.CreateActionInput{
			Timestamp: model.EntityTime(at),
			Kind:      "get_measurement",
			Label:     label,
			Params:    map[string]any{"channel": float64(channel)},
			Observed:  map[string]any{"temperature": temperature, "humidity": humidity},
			Result:    result,
		}
	}
	d1 := func(offset time.Duration) time.Time { return day1Start.Add(offset) }
	d2 := func(offset time.Duration) time.Time { return day2Start.Add(offset) }

	return []model.CreateActionInput{
		{
			Timestamp: model.EntityTime(d1(4600 * ms)), Kind: "read_params", Label: "Чтение параметров",
			Params: map[string]any{}, Result: "Уставка 22 °C, гистерезис 1, режим «нагрев», вентилятор 2",
		},
		setParam(d1(69700*ms), paramSetpoint, 21, "Задать уставку 21 °C", "OK, клиент перечитал параметры"),
		setParam(d1(149500*ms), paramSetpoint, 23, "Задать уставку 23 °C", "OK"),
		setParam(d1(219800*ms), paramSetpoint, 18, "Задать уставку 18 °C", "OK"),
		setParam(d1(289600*ms), paramMode, 2, "Режим: охлаждение", "OK"),
		measurement(d1(359700*ms), 1, 22.4, 41.0, "Измерение, канал 1", "22.4 °C, 41.0 %"),
		measurement(d1(419800*ms), 2, 19.8, 55.5, "Измерение, канал 2", "19.8 °C, 55.5 %"),
		{
			Timestamp: model.EntityTime(d1(479600 * ms)), Kind: "read_log", Label: "Чтение журнала (20 записей)",
			Params: map[string]any{"count": float64(20)}, Result: "Показано 20 записей",
		},
		setParam(d2(9700*ms), paramInterval, 600, "Интервал отчёта 600 с", "OK"),
		setParam(d2(59800*ms), paramCalibration, -15, "Калибровка −1.5 °C", "OK (в клиенте −1.5 °C)"),
		measurement(d2(119600*ms), 3, -5.5, 80.0, "Измерение, канал 3 (улица)", "−5.5 °C, 80.0 %"),
		setParam(d2(179700*ms), paramSetpoint, 25, "Задать уставку 25 °C", "OK"),
		{
			Timestamp: model.EntityTime(d2(239600 * ms)), Kind: "read_params", Label: "Чтение параметров",
			Params: map[string]any{}, Result: "6 параметров",
		},
	}
}
