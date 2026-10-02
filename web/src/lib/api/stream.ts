import { getStoredAuthToken } from "$lib/auth-token-store";
import type { Position } from "$lib/types/api";

function normalizeSpeed(pos: Position): Position {
  return pos.speed != null ? { ...pos, speed: pos.speed * 1.852 } : pos;
}

// Fetch a position range for the heatmap. The API returns one JSON array; pass
// `limit` to let the server sample large ranges instead of transferring every
// position. onProgress receives the number of positions loaded.
export async function streamPositions(
  params: { deviceId?: number; from?: string; to?: string; limit?: number },
  onProgress: (delta: number) => void,
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

  const positions = ((await response.json()) as Position[]).map(normalizeSpeed);
  onProgress(positions.length);
  return positions;
}
