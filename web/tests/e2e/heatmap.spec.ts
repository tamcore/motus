import { test, expect } from '../fixtures/auth-fixture';

test.describe('Heatmap', () => {
  test('loads compact points instead of full positions', async ({ authedPage }) => {
    await authedPage.addInitScript(() => {
      const w = window as unknown as { __positionRequests: string[] };
      w.__positionRequests = [];
      const origFetch = window.fetch;
      const json = (body: unknown) =>
        new Response(JSON.stringify(body), { status: 200, headers: { 'Content-Type': 'application/json' } });
      window.fetch = async function (input: RequestInfo | URL, init?: RequestInit) {
        const url = typeof input === 'string' ? input : input instanceof URL ? input.href : input.url;
        if (url.includes('/api/devices')) {
          return json([{ id: 4242, name: 'Heat Device', uniqueId: 'heat-1', status: 'online' }]);
        }
        if (url.includes('/api/positions')) {
          w.__positionRequests.push(url);
          return json([
            { lat: 49.79, lon: 9.95, speed: 10, fixTime: '2026-10-01T10:00:00Z' },
            { lat: 49.8, lon: 9.96, speed: 20, fixTime: '2026-10-01T11:00:00Z' },
          ]);
        }
        return origFetch.apply(globalThis, [input, init] as Parameters<typeof fetch>);
      } as typeof fetch;
    });

    await authedPage.goto('/heatmap');

    await expect(authedPage.locator('.data-count')).toHaveText('2 points');
    const requests = await authedPage.evaluate(
      () => (window as unknown as { __positionRequests: string[] }).__positionRequests,
    );
    expect(requests.length).toBeGreaterThan(0);
    for (const url of requests) {
      const parsed = new URL(url, 'http://x');
      expect(parsed.pathname).toBe('/api/positions/points');
      expect(parsed.searchParams.get('deviceId')).toBe('4242');
      expect(parsed.searchParams.get('limit')).toBe('10000');
    }
  });
});
