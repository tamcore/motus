package demo

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"time"
)

const commandReadTimeout = 500 * time.Millisecond

// rconfResult is the sanitised ST-901/4G rconf reply.
// PW→0000, U1→12025550100 (202-555-0100 per NANP), APN→internet,
// IP→203.0.113.1 (RFC-5737 TEST-NET-3).
const rconfResult = "ST-901/4G,ID:%s,PW:0000,U1:12025550100,U2:,U3:," +
	"MODE:GPRS,DAILY:OFF,POWER ALARM:OFF,ACCSMS:OFF,ACCCALL:OFF," +
	"GEO FENCE:OFF,OVER SPEED:OFF,VOICE:ON,SHAKE ALARM:OFF,SLEEPON," +
	"APN:internet,,,IP:203.0.113.1:5013," +
	"GPRS UPLOAD TIME 1:5,GPRS UPLOAD TIME 2:300,TIME ZONE:0.0"

// runCommandReader reads inbound data from the server connection and sends
// back simulated SMS responses for recognised commands.
//
// It owns all reads from w.conn for the lifetime of connCtx. The route loop
// must not read from the connection concurrently.
func runCommandReader(ctx context.Context, w *connWriter, imei string) {
	buf := make([]byte, 4096)
	acc := []byte{}
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		_ = w.conn.SetReadDeadline(time.Now().Add(commandReadTimeout))
		n, err := w.conn.Read(buf)
		if n > 0 {
			slog.Debug("commandreader recv",
				slog.String("device", imei),
				slog.String("hex", fmt.Sprintf("%x", buf[:n])),
				slog.String("text", string(buf[:n])))
			acc = append(acc, buf[:n]...)
			acc = dispatchTokens(w, imei, acc)
		}
		if err != nil {
			// Deadline/timeout errors are expected — the short deadline is used
			// only to make the read ctx-cancellable. Continue polling.
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				// Flush any accumulated bytes that have no '#' or '\n' terminator.
				// Traccar sends raw commands (e.g. "rconf", "FACTORY") with no
				// trailing delimiter — we detect them on the first timeout after
				// the bytes arrive.
				s := string(acc)
				if len(acc) > 0 &&
					strings.IndexByte(s, '#') < 0 &&
					strings.IndexByte(s, '\n') < 0 {
					token := strings.TrimSpace(s)
					slog.Debug("commandreader flush",
						slog.String("device", imei),
						slog.String("token", token))
					resp := generateResponse(imei, token)
					if resp != "" {
						_ = w.WriteString(resp)
					}
					acc = acc[:0]
				}
				continue
			}
			// Any other error (EOF, connection reset, etc.) is fatal.
			return
		}
	}
}

// dispatchTokens splits acc on either '#' (H02 frame terminator) or '\n'
// (raw command line terminator), calls generateResponse for each complete
// token, writes any non-empty response via w, and returns the remaining
// unterminated tail bytes.
//
// Traccar sends H02 acks as "*HQ,...#" and raw commands (rconf, FACTORY,
// setspeed,N, etc.) as plain text lines terminated by "\r\n" or "\n".
func dispatchTokens(w *connWriter, imei string, acc []byte) []byte {
	for {
		s := string(acc)
		hashIdx := strings.IndexByte(s, '#')
		nlIdx := strings.IndexByte(s, '\n')

		if hashIdx < 0 && nlIdx < 0 {
			break
		}

		var token string
		if hashIdx >= 0 && (nlIdx < 0 || hashIdx < nlIdx) {
			// H02 frame ends with '#'
			token = s[:hashIdx]
			acc = acc[hashIdx+1:]
		} else {
			// Raw command ends with '\n' (strip trailing '\r' if present)
			token = strings.TrimRight(s[:nlIdx], "\r")
			acc = acc[nlIdx+1:]
		}

		// When the server sends a raw command without a newline terminator
		// (e.g. "setspeed,80") immediately before an H02-framed message
		// (e.g. "*HQ,...,V4,...#"), the two can arrive in the same TCP read
		// and be accumulated as "setspeed,80*HQ,...,V4,...". Split the token
		// at the first "*HQ," that is not at position 0 and dispatch the raw
		// prefix first.
		if hqPos := strings.Index(token, "*HQ,"); hqPos > 0 {
			rawToken := strings.TrimSpace(token[:hqPos])
			if rawToken != "" {
				if rawResp := generateResponse(imei, rawToken); rawResp != "" {
					_ = w.WriteString(rawResp)
				}
			}
			token = token[hqPos:]
		}

		resp := generateResponse(imei, token)
		if resp != "" {
			_ = w.WriteString(resp)
		}
	}
	return acc
}

// generateResponse maps an inbound server token to the simulated SMS reply
// that a real ST-901/4G device would send back. Returns "" to send nothing.
func generateResponse(imei, token string) string {
	trimmed := strings.TrimSpace(token)

	// rconf — return full device configuration.
	if trimmed == "rconf" {
		return buildSMSResponse(imei, fmt.Sprintf(rconfResult, imei))
	}

	// Discard V4 server acknowledgements silently.
	if strings.Contains(token, ",V4,") {
		return ""
	}

	// *HQ,<id>,reset# — device reboot.
	if strings.HasSuffix(trimmed, ",reset") {
		return buildSMSResponse(imei, "Reboot OK")
	}

	// *HQ,<id>,locate# — single position request; no SMS reply needed.
	if strings.HasSuffix(trimmed, ",locate") {
		return ""
	}

	// *HQ,<id>,time,<n># — reporting interval.
	if strings.Contains(trimmed, ",time,") {
		parts := strings.Split(trimmed, ",")
		if len(parts) >= 4 {
			n := parts[len(parts)-1]
			return buildSMSResponse(imei, "Interval: "+n+"s")
		}
	}

	// setphone,1,<phone> — SOS number.
	if strings.HasPrefix(trimmed, "setphone,1,") {
		phone := strings.TrimPrefix(trimmed, "setphone,1,")
		return buildSMSResponse(imei, "SOS1: "+phone)
	}

	// setspeed,<n> — speed alarm.
	if strings.HasPrefix(trimmed, "setspeed,") {
		val := strings.TrimPrefix(trimmed, "setspeed,")
		if val == "0" {
			return buildSMSResponse(imei, "Speed alarm: disabled")
		}
		return buildSMSResponse(imei, "Speed alarm: "+val+"km/h")
	}

	// FACTORY — factory reset.
	if strings.EqualFold(trimmed, "factory") {
		return buildSMSResponse(imei, "Factory reset, please wait...")
	}

	// stockade,on,... — geofence enable.
	if strings.HasPrefix(strings.ToLower(trimmed), "stockade,on") {
		return buildSMSResponse(imei, "Geofence enabled")
	}

	// stockade,off — geofence disable.
	if strings.HasPrefix(strings.ToLower(trimmed), "stockade,off") {
		return buildSMSResponse(imei, "Geofence disabled")
	}

	// Any other *HQ command — generic OK.
	if strings.HasPrefix(trimmed, "*HQ,") {
		return buildSMSResponse(imei, "OK")
	}

	return ""
}

// buildSMSResponse formats the simulated SMS reply in H02 format.
func buildSMSResponse(imei, result string) string {
	return fmt.Sprintf("*HQ,%s,SMS,%s#\r\n", imei, result)
}
