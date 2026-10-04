import { get, writable } from "svelte/store";
import { streamChat, clearHistory } from "$lib/api/chat";

export interface DisplayMessage {
  role: "user" | "assistant";
  content: string;
  toolCalls?: Array<{ id: string; name: string; result?: unknown; error?: string }>;
}

export const chatMessages = writable<DisplayMessage[]>([]);
export const chatLoading = writable(false);
export const chatError = writable<string | null>(null);

let abortController: AbortController | null = null;

export async function sendMessage(userText: string): Promise<void> {
  if (abortController) {
    abortController.abort();
  }
  abortController = new AbortController();

  chatError.set(null);
  chatLoading.set(true);

  chatMessages.update((msgs) => [
    ...msgs,
    { role: "user", content: userText },
    { role: "assistant", content: "" },
  ]);
  const assistantIdx = get(chatMessages).length - 1;

  const patchAssistant = (patch: (msg: DisplayMessage) => DisplayMessage) =>
    chatMessages.update((msgs) => msgs.map((m, i) => (i === assistantIdx ? patch(m) : m)));

  try {
    // Send only the new user message — server reconstructs history from Redis.
    const stream = streamChat(userText, abortController.signal);

    for await (const event of stream) {
      if (event.type === "token") {
        patchAssistant((msg) => ({ ...msg, content: msg.content + event.delta }));
      } else if (event.type === "tool_call") {
        patchAssistant((msg) => ({
          ...msg,
          toolCalls: [...(msg.toolCalls ?? []), { id: event.id, name: event.name }],
        }));
      } else if (event.type === "tool_result") {
        patchAssistant((msg) => ({
          ...msg,
          toolCalls: (msg.toolCalls ?? []).map((tc) =>
            tc.id === event.id ? { ...tc, result: event.result, error: event.error } : tc,
          ),
        }));
      } else if (event.type === "error") {
        chatError.set(event.message);
        break;
      } else if (event.type === "done") {
        break;
      }
    }
  } catch (err: unknown) {
    if (err instanceof Error && err.name !== "AbortError") {
      chatError.set(err.message);
    }
  } finally {
    chatLoading.set(false);
    abortController = null;
  }
}

export async function newConversation(): Promise<void> {
  try {
    await clearHistory();
  } catch (err) {
    chatError.set(err instanceof Error ? err.message : "Failed to clear the conversation");
    return;
  }
  chatMessages.set([]);
  chatError.set(null);
}
