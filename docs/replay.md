# GPS Message Replay Tool

## Overview

The `motus replay` tool sends GPS protocol messages from log files or pcap captures to a live GPS server, simulating real device traffic. This is useful for:
- Testing GPS protocol handlers
- Demonstrating real-time UI updates
- Load testing with historical data
- Reproducing specific GPS scenarios

## Usage

```bash
motus replay --input=<file> [options]
```

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--input` | (required) | Input file (h02.log or .pcap) |
| `--type` | `h02log` | Input type: `h02log` or `pcap` |
| `--host` | `localhost` | GPS server hostname or IP |
| `--port` | `5013` | GPS server port (5013 for H02, 5093 for WATCH) |
| `--device-id` | (none) | Device unique ID to attribute messages to |
| `--speed` | `0` | Playback speed: 0=instant, 1=realtime, 2=2x, etc. |
| `--loop` | `false` | Loop messages indefinitely |
| `--verbose` | `false` | Show each message sent and server responses |

## Examples

### Basic Replay (instant)

Send messages from h02.log to the GPS server instantly:

```bash
motus replay \
  --input=h02.log \
  --host=localhost \
  --port=5013 \
  --device-id=YOUR_DEVICE_ID
```

### Realtime Playback

Replay at actual speed (5-second intervals between messages):

```bash
motus replay \
  --input=h02.log \
  --host=localhost \
  --port=5013 \
  --device-id=YOUR_DEVICE_ID \
  --speed=1
```

### Fast Playback

Replay at 2x speed:

```bash
motus replay \
  --input=h02.log \
  --host=localhost \
  --port=5013 \
  --device-id=YOUR_DEVICE_ID \
  --speed=2
```

### Loop Continuously

Replay messages in a continuous loop:

```bash
motus replay \
  --input=h02.log \
  --host=localhost \
  --port=5013 \
  --device-id=YOUR_DEVICE_ID \
  --loop
```

### From PCAP File

Extract and replay messages from packet capture:

```bash
motus replay \
  --input=capture_5013.pcap \
  --type=pcap \
  --host=localhost \
  --port=5013 \
  --device-id=YOUR_DEVICE_ID
```

### Test Real-time Updates

```bash
# Start replay in one terminal
motus replay --input=h02.log --host=localhost --port=5013 --speed=1 --verbose

# Open the UI in browser and watch device marker update in real-time
open http://localhost:8080
```

### Load Testing

```bash
# Send all messages instantly and loop
motus replay --input=h02.log --host=localhost --port=5013 --speed=0 --loop
```

### Demo Route Playback

```bash
# Replay at 4x speed to quickly demonstrate route
motus replay --input=h02.log --host=localhost --port=5013 --speed=4
```

## Input Files

### h02.log Format

The tool extracts H02 protocol messages matching the pattern `*HQ,...#`:

```
2026-01-01 12:00:00  INFO: [T57f86696: h02 < 10.42.4.22] *HQ,123456789,V6,120000,A,4948.8999,N,00958.2106,E,000.00,000,010126,FFFFFBFF,262,03,49032,46083637,8949227221106570251F#
```

### PCAP Format

The tool reads pcap files and extracts H02 messages from TCP payload. Make sure the pcap contains H02 protocol traffic.

## How It Works

1. **Extract**: Parses input file and extracts H02 protocol messages
2. **Replace Device ID**: Replaces device ID in messages to match `--device-id`
3. **Connect**: Opens TCP connection to GPS server
4. **Send**: Sends messages with optional delays based on `--speed`
5. **Loop**: Optionally loops back to start when all messages are sent

## Notes

- Messages are sent with `\r\n` line endings
- Playback speed 0 = instant (no delays)
- Playback speed 1 = realtime (5-second intervals)
- Server must be running and accessible
- Monitor the WebSocket in the UI to see real-time position updates
