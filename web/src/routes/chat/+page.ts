import { redirect } from "@sveltejs/kit";
import { get } from "svelte/store";
import { loadServerInfo, serverInfo } from "$lib/stores/server";
import type { PageLoad } from "./$types";

export const load: PageLoad = async () => {
  await loadServerInfo();
  if (!get(serverInfo)?.aiEnabled) {
    throw redirect(307, "/");
  }
};
