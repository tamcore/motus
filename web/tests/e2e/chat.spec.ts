import { test, expect } from '../fixtures/auth-fixture';

test.describe('Chat page', () => {
  test.beforeEach(async ({ authedPage }) => {
    // Mock /api/server to enable AI and /api/chat with a canned SSE response.
    await authedPage.addInitScript(() => {
      const origFetch = window.fetch;
      window.fetch = async function (input: RequestInfo | URL, init?: RequestInit) {
        const url =
          typeof input === 'string'
            ? input
            : input instanceof URL
              ? input.href
              : input.url;

        if (url.includes('/api/server')) {
          return new Response(
            JSON.stringify({
              id: 1,
              registration: false,
              readonly: false,
              deviceReadonly: false,
              limitCommands: false,
              version: 'test',
              aiEnabled: true,
            }),
            { status: 200, headers: { 'Content-Type': 'application/json' } },
          );
        }

        if (url.includes('/api/chat')) {
          const body = [
            'data: {"type":"tool_call","id":"tc1","name":"get_server_time"}\n\n',
            'data: {"type":"tool_result","id":"tc1","name":"get_server_time","result":{"now":"2025-01-01T00:00:00Z"}}\n\n',
            'data: {"type":"token","delta":"The "}\n\n',
            'data: {"type":"token","delta":"answer is 42."}\n\n',
            'data: {"type":"done"}\n\n',
          ].join('');
          return new Response(body, {
            status: 200,
            headers: { 'Content-Type': 'text/event-stream' },
          });
        }

        return origFetch.apply(globalThis, [input, init] as Parameters<typeof fetch>);
      } as typeof fetch;
    });
  });

  test('shows Chat nav link when aiEnabled', async ({ authedPage }) => {
    await authedPage.goto('/');
    await expect(authedPage.locator('nav a[href="/chat"]')).toBeVisible({ timeout: 10000 });
  });

  test('navigates to /chat route', async ({ authedPage }) => {
    await authedPage.goto('/chat');
    await expect(authedPage.locator('textarea')).toBeVisible({ timeout: 10000 });
  });

  test('sends a message and shows assistant response with tool card', async ({ authedPage }) => {
    await authedPage.goto('/chat');
    const textarea = authedPage.locator('textarea');
    await textarea.waitFor({ state: 'visible', timeout: 10000 });

    await textarea.fill('What time is it?');
    await authedPage.keyboard.press('Enter');

    await expect(authedPage.locator('.user-bubble').first()).toContainText('What time is it?', {
      timeout: 5000,
    });

    await expect(authedPage.locator('details.tool-card summary')).toContainText(
      'get_server_time',
      { timeout: 10000 },
    );

    await expect(authedPage.locator('.assistant-text')).toContainText('answer is 42', {
      timeout: 10000,
    });
  });
});
