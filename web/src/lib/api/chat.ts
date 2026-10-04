import { apiFetch, request } from "./client";
import type { ChatEvent, ChatMessage } from "$lib/types/api";

export async function* streamChat(
  userText: string,
  signal: AbortSignal,
): AsyncIterable<ChatEvent> {
  const response = await apiFetch("/chat", {
    method: "POST",
    signal,
    body: JSON.stringify({ message: { role: "user", content: userText } }),
  });

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
  try {
    const data = await request<{ messages?: ChatMessage[] }>("/chat/history");
    return data.messages ?? [];
  } catch (err) {
    console.error("Failed to load chat history:", err);
    return [];
  }
}

export function clearHistory(): Promise<void> {
  return request<void>("/chat/history", { method: "DELETE" });
}
