// Package databento streams real-time US-equity quotes from Databento's Live
// gateway (dataset EQUS.MINI) and republishes them as live store.Quote updates —
// specifically to fix extended-hours (pre/post/overnight) price freshness for thin
// names the free Alpaca IEX feed barely prints. It is a pure additive second writer
// into the same SSE hub + store the Alpaca streamer uses; read-time arbitration (in
// the api layer) prefers Databento for extended sessions. Keyless-inert: with no
// DATABENTO_API_KEY the streamer never starts and the system is byte-identical to
// today (Alpaca-only).
//
// The wire protocol is RAW TCP + binary DBN (NOT WebSocket / JSON). This file is the
// hand-rolled, stdlib-only protocol layer: CRAM authentication, the DBN record
// framing, and decoders for exactly the two record types we consume (bbo-1s + trades)
// plus symbol mappings. Decoders are pure []byte→struct functions, unit-tested against
// byte fixtures (offline, zero billable Live quota) — the correctness backstop that
// makes hand-rolling (vs. a heavy third-party DBN lib) the right call.
package databento

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"strings"
	"time"
)

const (
	// DefaultHost is the EQUS.MINI Live gateway — plaintext TCP, no TLS.
	DefaultHost = "equs-mini.lsg.databento.com:13000"
	// DefaultDataset / DefaultSchema are the EQUS.MINI defaults.
	DefaultDataset = "EQUS.MINI"
	DefaultSchema  = "bbo-1s"

	// dbnVersion is the DBN encoding version this decoder targets. The metadata
	// prelude carries the version at byte 3; we ASSERT it (a bump to v4 would shift
	// record layouts, so fail loudly rather than mis-parse).
	dbnVersion = 3

	// DBN record types (RecordHeader byte index 1).
	rtypeTrade         = 0x00 // TradeMsg (mbp-0)
	rtypeBbo           = 0xC3 // bbo-1s / cbbo
	rtypeError         = 0x15 // ErrorMsg
	rtypeSymbolMapping = 0x16 // SymbolMappingMsg
	rtypeSystem        = 0x17 // SystemMsg (heartbeat = code 0)

	// priceScale: a fixed-point i64 price → float = raw / 1e9.
	priceScale = 1_000_000_000.0
	// undefPrice is the i64 "no value" sentinel; treat as null (never a real price).
	undefPrice = int64(0x7FFFFFFFFFFFFFFF)
)

// cramResponse computes the CRAM `auth=` value for a challenge + a 32-char API key:
// hex(sha256(challenge|key)) + "-" + bucket, where bucket = the last 5 chars of the key.
// The raw key is NEVER sent — the hex digest is the only proof of possession.
func cramResponse(challenge, apiKey string) (string, error) {
	if len(apiKey) != 32 {
		return "", fmt.Errorf("databento: API key must be 32 chars, got %d", len(apiKey))
	}
	sum := sha256.Sum256([]byte(challenge + "|" + apiKey))
	return hex.EncodeToString(sum[:]) + "-" + apiKey[27:32], nil
}

// authenticate performs the CRAM handshake on a fresh connection: read greeting +
// challenge, send the auth line, confirm success. The reader is left positioned for
// the subscription replies + the metadata header.
func authenticate(conn net.Conn, r *bufio.Reader, apiKey, dataset string) error {
	if _, err := r.ReadString('\n'); err != nil { // greeting line — ignore
		return fmt.Errorf("databento: read greeting: %w", err)
	}
	line, err := r.ReadString('\n') // "cram=<challenge>\n"
	if err != nil {
		return fmt.Errorf("databento: read challenge: %w", err)
	}
	challenge, ok := strings.CutPrefix(strings.TrimRight(line, "\r\n"), "cram=")
	if !ok {
		return fmt.Errorf("databento: unexpected challenge line %q", strings.TrimRight(line, "\r\n"))
	}
	resp, err := cramResponse(challenge, apiKey)
	if err != nil {
		return err
	}
	auth := fmt.Sprintf("auth=%s|dataset=%s|encoding=dbn|compression=none|ts_out=0|client=Tickwind/1.0 Go\n", resp, dataset)
	if _, err := conn.Write([]byte(auth)); err != nil {
		return fmt.Errorf("databento: write auth: %w", err)
	}
	res, err := r.ReadString('\n')
	if err != nil {
		return fmt.Errorf("databento: read auth result: %w", err)
	}
	if !fieldEquals(res, "success", "1") {
		return fmt.Errorf("databento: auth failed: %s", strings.TrimRight(res, "\r\n"))
	}
	return nil
}

// fieldEquals reports whether a pipe-delimited "k=v|k2=v2" line has key==val.
func fieldEquals(line, key, val string) bool {
	for _, f := range strings.Split(strings.TrimRight(line, "\r\n"), "|") {
		if k, v, ok := strings.Cut(f, "="); ok && k == key {
			return v == val
		}
	}
	return false
}

// subscribe sends one subscription line for a schema + symbol set (raw_symbol). The
// gateway closes the session if ANY symbol fails to resolve, so callers pass a clean
// US-equity set. Tickwind's set is ≤30 so a single line never exceeds the 500/line cap.
func subscribe(conn net.Conn, schema string, symbols []string) error {
	line := fmt.Sprintf("schema=%s|stype_in=raw_symbol|symbols=%s|snapshot=0|is_last=1\n",
		schema, strings.Join(symbols, ","))
	_, err := conn.Write([]byte(line))
	return err
}

// startSession tells the gateway to begin streaming (metadata header, then records).
func startSession(conn net.Conn) error {
	_, err := conn.Write([]byte("start_session\n"))
	return err
}

// readMetadata consumes the one-time DBN metadata header that precedes the record
// stream: verify the "DBN" magic + version, then skip the variable section (per-record
// symbol labels come from SymbolMappingMsg records, not this table).
func readMetadata(r *bufio.Reader) error {
	var prelude [8]byte
	if _, err := io.ReadFull(r, prelude[:]); err != nil {
		return fmt.Errorf("databento: read metadata prelude: %w", err)
	}
	if string(prelude[0:3]) != "DBN" {
		return fmt.Errorf("databento: bad metadata magic %q", prelude[0:3])
	}
	if v := prelude[3]; v != dbnVersion {
		return fmt.Errorf("databento: unexpected DBN version %d (decoder targets v%d)", v, dbnVersion)
	}
	length := binary.LittleEndian.Uint32(prelude[4:8])
	if _, err := io.CopyN(io.Discard, r, int64(length)); err != nil {
		return fmt.Errorf("databento: skip metadata body: %w", err)
	}
	return nil
}

// record is one framed DBN record: its type byte + full bytes (header included).
type record struct {
	rtype byte
	body  []byte
}

// nextRecord reads one length-prefixed DBN record. The leading byte is the record
// length in 32-bit WORDS (×4 = bytes); rtype is byte index 1.
func nextRecord(r *bufio.Reader) (record, error) {
	lenByte, err := r.ReadByte()
	if err != nil {
		return record{}, err
	}
	n := int(lenByte) * 4
	if n < 8 { // a RecordHeader is 16 bytes; anything < 8 is corrupt framing
		return record{}, fmt.Errorf("databento: implausible record length %d", n)
	}
	body := make([]byte, n)
	body[0] = lenByte
	if _, err := io.ReadFull(r, body[1:]); err != nil {
		return record{}, fmt.Errorf("databento: read record body: %w", err)
	}
	return record{rtype: body[1], body: body}, nil
}

// recInstrumentID reads RecordHeader.instrument_id (u32 @ offset 4).
func recInstrumentID(body []byte) uint32 { return binary.LittleEndian.Uint32(body[4:8]) }

// Bbo is a decoded bbo-1s record: best bid/ask (+ size) and the interval's last trade.
type Bbo struct {
	InstrumentID uint32
	Last         float64 // last trade price; 0 when undef
	Bid          float64 // best bid; 0 when undef
	Ask          float64 // best ask; 0 when undef
	BidSize      uint32
	AskSize      uint32
	TsRecv       time.Time // interval-end timestamp
}

// decodeBbo decodes an 80-byte bbo-1s record (offsets per the DBN v3 spec).
func decodeBbo(body []byte) (Bbo, bool) {
	if len(body) < 80 {
		return Bbo{}, false
	}
	return Bbo{
		InstrumentID: recInstrumentID(body),
		Last:         price(int64(binary.LittleEndian.Uint64(body[16:24]))),
		BidSize:      binary.LittleEndian.Uint32(body[64:68]),
		AskSize:      binary.LittleEndian.Uint32(body[68:72]),
		Bid:          price(int64(binary.LittleEndian.Uint64(body[48:56]))),
		Ask:          price(int64(binary.LittleEndian.Uint64(body[56:64]))),
		TsRecv:       tsToTime(binary.LittleEndian.Uint64(body[32:40])),
	}, true
}

// Trade is a decoded trade (mbp-0) record.
type Trade struct {
	InstrumentID uint32
	Price        float64
	Size         uint32
	TsRecv       time.Time
}

// decodeTrade decodes a 48-byte trade record.
func decodeTrade(body []byte) (Trade, bool) {
	if len(body) < 48 {
		return Trade{}, false
	}
	return Trade{
		InstrumentID: recInstrumentID(body),
		Price:        price(int64(binary.LittleEndian.Uint64(body[16:24]))),
		Size:         binary.LittleEndian.Uint32(body[24:28]),
		TsRecv:       tsToTime(binary.LittleEndian.Uint64(body[32:40])),
	}, true
}

// decodeSymbolMapping returns (instrument_id, ticker) from a 176-byte SymbolMappingMsg.
// stype_out_symbol is a 71-byte null-padded C-string at offset 89; with stype_in=raw_symbol
// it is the plain ticker (e.g. "AAPL").
func decodeSymbolMapping(body []byte) (uint32, string, bool) {
	if len(body) < 160 {
		return 0, "", false
	}
	sym := cstr(body[89:160])
	if sym == "" {
		return 0, "", false
	}
	return recInstrumentID(body), sym, true
}

// price converts a fixed-point i64 to float, mapping the UNDEF sentinel to 0 (null).
func price(raw int64) float64 {
	if raw == undefPrice {
		return 0
	}
	return float64(raw) / priceScale
}

// tsToTime converts DBN nanoseconds-since-epoch to a UTC time (0 → zero time).
func tsToTime(ns uint64) time.Time {
	if ns == 0 {
		return time.Time{}
	}
	return time.Unix(0, int64(ns)).UTC()
}

// cstr reads a NUL-terminated string from a fixed buffer.
func cstr(b []byte) string {
	if i := bytes.IndexByte(b, 0); i >= 0 {
		return string(b[:i])
	}
	return string(b)
}
