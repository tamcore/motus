import { writable } from "svelte/store";
import type { WebSocketMessage } from "$lib/types/api";

/** Maximum length of raw message content included in warning logs. */
const LOG_TRUNCATE_LENGTH = 200;

class WebSocketManager {
  private ws: WebSocket | null = null;
  private reconnectTimer: ReturnType<typeof setTimeout> | null = null;
  private pingInterval: ReturnType<typeof setInterval> | null = null;

  public connected = writable(false);
  public lastMessage = writable<WebSocketMessage | null>(null);

  private startPingInterval() {
    if (this.pingInterval) {
      clearInterval(this.pingInterval);
    }
    this.pingInterval = setInterval(() => {
      if (this.ws && this.ws.readyState === WebSocket.OPEN) {
        this.ws.send(JSON.stringify({ type: "ping" }));
      }
    }, 30000);
  }

  private stopPingInterval() {
    if (this.pingInterval) {
      clearInterval(this.pingInterval);
      this.pingInterval = null;
    }
  }

  connect() {
    if (typeof window === "undefined") return;

    // Guard against duplicate connections - if already open or connecting, skip
    if (
      this.ws &&
      (this.ws.readyState === WebSocket.OPEN ||
        this.ws.readyState === WebSocket.CONNECTING)
    ) {
      if (import.meta.env.DEV) {
        console.log("[WS] Already connected or connecting, skipping");
      }
      return;
    }

    // Clean up any existing dead socket before creating a new one
    if (this.ws) {
      this.ws.onopen = null;
      this.ws.onmessage = null;
      this.ws.onclose = null;
      this.ws.onerror = null;
      this.ws = null;
    }

    const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
    const url = `${protocol}//${window.location.host}/api/socket`;

    if (import.meta.env.DEV) {
      console.log("[WS] Connecting to:", url);
    }
    const ws = new WebSocket(url);
    this.ws = ws;

    ws.onopen = () => {
      // Only update state if this is still the active socket
      if (this.ws !== ws) return;
      this.connected.set(true);
      if (import.meta.env.DEV) {
        console.log("[WS] Connected successfully");
      }

      // Send ping every 30 seconds to keep connection alive
      this.startPingInterval();
    };

    ws.onmessage = (event) => {
      // Only process messages from the active socket
      if (this.ws !== ws) return;
      try {
        const data: WebSocketMessage = JSON.parse(event.data);
        // Normalize speed from knots (Traccar API) to km/h (internal UI unit).
        if (data.positions?.length) {
          data.positions = data.positions.map((pos) =>
            pos.speed != null ? { ...pos, speed: pos.speed * 1.852 } : pos,
          );
        }
        this.lastMessage.set(data);
      } catch (err) {
        // Log malformed messages for debugging but don't crash
        const raw =
          typeof event.data === "string"
            ? event.data.slice(0, LOG_TRUNCATE_LENGTH)
            : "[non-string data]";
        console.warn(
          "[WS] Malformed message (parse failed):",
          err instanceof Error ? err.message : String(err),
          "| raw:",
          raw,
        );
      }
    };

    ws.onclose = (event) => {
      // Only update state and reconnect if this is still the active socket
      if (this.ws !== ws) return;
      this.connected.set(false);
      this.stopPingInterval();
      if (import.meta.env.DEV) {
        console.log(
          "[WS] Disconnected, code:",
          event.code,
          "reason:",
          event.reason,
        );
      }
      this.ws = null;
      this.reconnectTimer = setTimeout(() => this.connect(), 5000);
    };

    ws.onerror = (error) => {
      console.error("[WS] Error:", error);
      ws.close();
    };
  }

  disconnect() {
    this.stopPingInterval();
    if (this.reconnectTimer) {
      clearTimeout(this.reconnectTimer);
      this.reconnectTimer = null;
    }
    if (this.ws) {
      // Null out handlers before closing to prevent stale onclose from firing
      this.ws.onopen = null;
      this.ws.onmessage = null;
      this.ws.onclose = null;
      this.ws.onerror = null;
      this.ws.close();
      this.ws = null;
    }
    this.connected.set(false);
  }
}

export const wsManager = new WebSocketManager();
