import { writable, get } from "svelte/store";
import { api } from "$lib/api/client";
import type { ServerInfo } from "$lib/types/api";

export const serverInfo = writable<ServerInfo | null>(null);

let loading = false;

export async function loadServerInfo(): Promise<void> {
  if (loading || get(serverInfo) !== null) return;
  loading = true;
  try {
    const info = await api.getServerInfo();
    serverInfo.set(info);
  } catch {
    // Non-fatal — serverInfo stays null; aiEnabled will be treated as false.
  } finally {
    loading = false;
  }
}
