import { writable } from "svelte/store";
import { streamChat } from "$lib/api/chat";

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

  chatMessages.update((msgs) => [...msgs, { role: "user", content: userText }]);

  const assistantIdx: number = await new Promise((resolve) => {
    chatMessages.update((msgs) => {
      resolve(msgs.length);
      return [...msgs, { role: "assistant", content: "" }];
    });
  });

  try {
    // Build wire history from messages before the blank assistant placeholder.
    const history = await new Promise<DisplayMessage[]>((resolve) => {
      chatMessages.subscribe((msgs) => resolve(msgs.slice(0, assistantIdx)))();
    });

    const wire = history.map((m) => ({
      role: m.role as "user" | "assistant",
      content: m.content,
    }));

    const stream = streamChat(wire, abortController.signal);

    for await (const event of stream) {
      if (event.type === "token") {
        chatMessages.update((msgs) => {
          const updated = [...msgs];
          updated[assistantIdx] = {
            ...updated[assistantIdx],
            content: updated[assistantIdx].content + event.delta,
          };
          return updated;
        });
      } else if (event.type === "tool_call") {
        chatMessages.update((msgs) => {
          const updated = [...msgs];
          const msg = { ...updated[assistantIdx] };
          msg.toolCalls = [...(msg.toolCalls ?? []), { id: event.id, name: event.name }];
          updated[assistantIdx] = msg;
          return updated;
        });
      } else if (event.type === "tool_result") {
        chatMessages.update((msgs) => {
          const updated = [...msgs];
          const msg = { ...updated[assistantIdx] };
          msg.toolCalls = (msg.toolCalls ?? []).map((tc) =>
            tc.id === event.id ? { ...tc, result: event.result, error: event.error } : tc,
          );
          updated[assistantIdx] = msg;
          return updated;
        });
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
