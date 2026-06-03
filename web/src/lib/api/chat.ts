import { getAuthHeaders, setCsrfToken } from "./headers";
import type { ChatEvent, ChatMessage } from "$lib/types/api";

const API_BASE = "/api";

export async function* streamChat(
  userText: string,
  signal: AbortSignal,
): AsyncIterable<ChatEvent> {
  const authHeaders = await getAuthHeaders("POST");
  const response = await fetch(`${API_BASE}/chat`, {
    method: "POST",
    credentials: "include",
    signal,
    headers: {
      "Content-Type": "application/json",
      ...authHeaders,
    },
    body: JSON.stringify({ message: { role: "user", content: userText } }),
  });

  const token = response.headers.get("X-CSRF-Token");
  if (token) setCsrfToken(token);

  if (!response.ok) {
    throw new Error(`Chat request failed: ${response.status}`);
  }

  const reader = response.body!.getReader();
  const decoder = new TextDecoder();
  let buffer = "";

  try {
    while (true) {
      const { done, value } = await reader.read();
      if (done) break;

      buffer += decoder.decode(value, { stream: true });
      const parts = buffer.split("\n\n");
      buffer = parts.pop() ?? "";

      for (const part of parts) {
        const line = part.trim();
        if (!line.startsWith("data: ")) continue;
        const json = line.slice(6);
        try {
          yield JSON.parse(json) as ChatEvent;
        } catch {
          // Skip malformed events.
        }
      }
    }
  } finally {
    reader.cancel();
  }
}

export async function fetchHistory(): Promise<ChatMessage[]> {
  const authHeaders = await getAuthHeaders("GET");
  const response = await fetch(`${API_BASE}/chat/history`, {
    credentials: "include",
    headers: authHeaders,
  });
  if (!response.ok) return [];
  const data = await response.json();
  return (data.messages as ChatMessage[]) ?? [];
}

export async function clearHistory(): Promise<void> {
  const authHeaders = await getAuthHeaders("DELETE");
  await fetch(`${API_BASE}/chat/history`, {
    method: "DELETE",
    credentials: "include",
    headers: authHeaders,
  });
}
