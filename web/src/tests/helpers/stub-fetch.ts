import { vi } from "vitest";

export function stubFetch(body: unknown, status = 200) {
  const fetchMock = vi
    .fn()
    .mockResolvedValue(
      status === 204 ? new Response(null, { status }) : new Response(JSON.stringify(body), { status }),
    );
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}
