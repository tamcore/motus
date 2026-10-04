import { writable } from "svelte/store";
import type { WebSocketMessage } from "$lib/types/api";
import { speedToKmh } from "$lib/api/client";

/** Maximum length of raw message content included in warning logs. */
const LOG_TRUNCATE_LENGTH = 200;

class WebSocketManager {
  private ws: WebSocket | null = null;
  private reconnectTimer: ReturnType<typeof setTimeout> | null = null;
  private pingInterval: ReturnType<typeof setInterval> | null = null;

  public connected = writable(false);
  public lastMessage = writable<WebSocketMessage | null>(null);

  private startPingInterval() {
    this.stopPingInterval();
    this.pingInterval = setInterval(() => {
      if (this.ws && this.ws.readyState === WebSocket.OPEN) {
        this.ws.send(JSON.stringify({ type: "ping" }));
      }
    }, 30000);
  }

  /** Drops the current socket, nulling its handlers so a stale onclose cannot fire. */
  private detach(): WebSocket | null {
    const ws = this.ws;
    if (ws) {
      ws.onopen = null;
      ws.onmessage = null;
      ws.onclose = null;
      ws.onerror = null;
    }
    this.ws = null;
    return ws;
  }

  private stopPingInterval() {
    if (this.pingInterval) {
      clearInterval(this.pingInterval);
      this.pingInterval = null;
    }
  }

  connect() {
    // Guard against duplicate connections - if already open or connecting, skip
    if (
      this.ws &&
      (this.ws.readyState === WebSocket.OPEN ||
        this.ws.readyState === WebSocket.CONNECTING)
    ) {
      return;
    }

    // Clean up any existing dead socket before creating a new one
    this.detach();

    const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
    const url = `${protocol}//${window.location.host}/api/socket`;
    const ws = new WebSocket(url);
    this.ws = ws;

    ws.onopen = () => {
      // Only update state if this is still the active socket
      if (this.ws !== ws) return;
      this.connected.set(true);

      // Send ping every 30 seconds to keep connection alive
      this.startPingInterval();
    };

    ws.onmessage = (event) => {
      // Only process messages from the active socket
      if (this.ws !== ws) return;
      try {
        const data: WebSocketMessage = JSON.parse(event.data);
        if (data.positions?.length) data.positions = data.positions.map(speedToKmh);
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

    ws.onclose = () => {
      // Only update state and reconnect if this is still the active socket
      if (this.ws !== ws) return;
      this.connected.set(false);
      this.stopPingInterval();
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
    this.detach()?.close();
    this.connected.set(false);
  }
}

export const wsManager = new WebSocketManager();
