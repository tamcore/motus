import { getStoredAuthToken } from "$lib/auth-token-store";
import type { Position } from "$lib/types/api";

function normalizeSpeed(pos: Position): Position {
  return pos.speed != null ? { ...pos, speed: pos.speed * 1.852 } : pos;
}

// Parse the streaming JSON array response line-by-line.
//
// Go's json.Encoder writes each position as a single \n-terminated line, so
// the body looks like:
//   [{"id":1,...}\n,{"id":2,...}\n,...,{"id":N,...}\n]
//
// We strip the array-bracket / leading-comma wrappers and parse each small
// JSON object individually, avoiding a single giant JSON.parse call on the
// full response body (which blocks the main thread for several seconds and
// risks OOM for large datasets like "All time").
//
// maxPositions: if > 0, reservoir-sample the stream down to that many items.
// The reservoir gives a statistically uniform sample — callers that only
// render a fraction of positions (e.g. heatmap → 10k) get identical visual
// output with far less memory pressure.
//
// onProgress receives the number of NEW positions parsed since the last call
// (a delta, not a cumulative total), so callers can do `count += delta`.
export async function streamPositions(
  params: { deviceId?: number; from?: string; to?: string; limit?: number },
  onProgress: (delta: number) => void,
  maxPositions = 0,
): Promise<Position[]> {
  const query = new URLSearchParams();
  if (params.deviceId) query.set("deviceId", String(params.deviceId));
  if (params.from) query.set("from", params.from);
  if (params.to) query.set("to", params.to);
  if (params.limit) query.set("limit", String(params.limit));

  const headers: Record<string, string> = { "Content-Type": "application/json" };
  const authToken = await getStoredAuthToken();
  if (authToken) headers["X-Auth-Token"] = authToken;

  const response = await fetch(`/api/positions?${query}`, {
    credentials: "include",
    headers,
  });

  if (!response.ok) {
    throw new Error(`HTTP ${response.status}: ${await response.text()}`);
  }

  const reader = response.body!.getReader();
  const decoder = new TextDecoder();
  const positions: Position[] = [];
  let accumulated = "";
  let lineStart = 0;
  let totalParsed = 0;

  while (true) {
    const { done, value } = await reader.read();
    if (done) break;

    accumulated += decoder.decode(value, { stream: true });

    let newlineIdx: number;
    let delta = 0;

    while ((newlineIdx = accumulated.indexOf("\n", lineStart)) !== -1) {
      const line = accumulated.slice(lineStart, newlineIdx).trim();
      lineStart = newlineIdx + 1;

      if (!line) continue;

      // Strip JSON array wrapper characters: leading '[', leading ',', trailing ']'
      let json = line;
      if (json.charCodeAt(0) === 44 /* , */) json = json.slice(1);
      if (json.charCodeAt(0) === 91 /* [ */) json = json.slice(1);
      if (json.charCodeAt(json.length - 1) === 93 /* ] */) json = json.slice(0, -1);
      if (!json || json.charCodeAt(0) !== 123 /* { */) continue;

      try {
        const pos = normalizeSpeed(JSON.parse(json) as Position);
        totalParsed++;
        delta++;

        if (maxPositions <= 0 || positions.length < maxPositions) {
          positions.push(pos);
        } else {
          // Reservoir sampling (Algorithm R): replace a random earlier entry
          const j = Math.floor(Math.random() * totalParsed);
          if (j < maxPositions) {
            positions[j] = pos;
          }
        }
      } catch {
        // Malformed line — skip silently
      }
    }

    if (delta > 0) onProgress(delta);

    // Compact the accumulated string periodically to prevent unbounded growth
    if (lineStart > 65536) {
      accumulated = accumulated.slice(lineStart);
      lineStart = 0;
    }
  }

  return positions;
}
