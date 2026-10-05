import { test, expect } from '../fixtures/auth-fixture';
import { mockFetch } from '../helpers/mock-fetch';

test.describe('Chat page', () => {
  test.beforeEach(async ({ authedPage }) => {
    // AI enabled, a canned SSE answer, and empty history for the onMount fetch.
    await mockFetch(authedPage, [
      {
        match: '/api/server',
        body: {
          id: 1,
          registration: false,
          readonly: false,
          deviceReadonly: false,
          limitCommands: false,
          version: 'test',
          aiEnabled: true,
        },
      },
      { path: '/api/chat/history', method: 'DELETE', status: 204 },
      { path: '/api/chat/history', body: { messages: [] } },
      {
        path: '/api/chat',
        contentType: 'text/event-stream',
        text: [
          'data: {"type":"tool_call","id":"tc1","name":"get_server_time"}\n\n',
          'data: {"type":"tool_result","id":"tc1","name":"get_server_time","result":{"now":"2025-01-01T00:00:00Z"}}\n\n',
          'data: {"type":"token","delta":"The "}\n\n',
          'data: {"type":"token","delta":"answer is 42."}\n\n',
          'data: {"type":"done"}\n\n',
        ].join(''),
      },
    ]);
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

  test('shows loaded history on mount', async ({ authedPage }) => {
    // Override the history GET mock to return a prior conversation.
    await mockFetch(authedPage, [{ path: '/api/chat/history', method: 'GET', body: {
      messages: [
        { role: 'user', content: 'Hello from history' },
        { role: 'assistant', content: 'Hi there!' },
      ],
    } }]);

    await authedPage.goto('/chat');
    await expect(authedPage.locator('.user-bubble').first()).toContainText('Hello from history', {
      timeout: 5000,
    });
    await expect(authedPage.locator('.assistant-text').first()).toContainText('Hi there!', {
      timeout: 5000,
    });
  });

  test('preserves tool call results on history reload', async ({ authedPage }) => {
    await mockFetch(authedPage, [{ path: '/api/chat/history', method: 'GET', body: {
      messages: [
        { role: 'user', content: 'What time is it?' },
        {
          role: 'assistant',
          content: '',
          toolCalls: [{ id: 'tc1', name: 'get_server_time', arguments: '{}' }],
        },
        {
          role: 'tool',
          toolCallId: 'tc1',
          name: 'get_server_time',
          content: '{"now":"2025-01-01T00:00:00Z"}',
        },
        { role: 'assistant', content: 'It is midnight UTC.' },
      ],
    } }]);

    await authedPage.goto('/chat');

    await expect(authedPage.locator('details.tool-card summary')).toContainText(
      'get_server_time',
      { timeout: 5000 },
    );

    await authedPage.locator('details.tool-card').click();
    await expect(authedPage.locator('.tool-result')).toContainText('2025-01-01T00:00:00Z', {
      timeout: 3000,
    });
    await expect(authedPage.locator('.tool-pending')).toHaveCount(0);
  });

  test('renders markdown tables as horizontally scrollable', async ({ authedPage }) => {
    const tableMarkdown =
      '| Device | Status | Location |\\n|--------|--------|----------|\\n| Car | Online | Berlin |\\n';
    await mockFetch(authedPage, [
      {
        path: '/api/chat',
        contentType: 'text/event-stream',
        text: `data: {"type":"token","delta":"${tableMarkdown}"}\n\ndata: {"type":"done"}\n\n`,
      },
    ]);

    await authedPage.goto('/chat');
    const textarea = authedPage.locator('textarea');
    await textarea.waitFor({ state: 'visible', timeout: 10000 });

    await textarea.fill('List my devices');
    await authedPage.keyboard.press('Enter');

    const table = authedPage.locator('.assistant-text table');
    await expect(table).toBeVisible({ timeout: 10000 });
    await expect(table).toHaveCSS('overflow-x', 'auto');
  });

  test('assistant bubble does not overflow on mobile viewport', async ({ authedPage }) => {
    await authedPage.setViewportSize({ width: 390, height: 844 });
    await authedPage.goto('/chat');
    const textarea = authedPage.locator('textarea');
    await textarea.waitFor({ state: 'visible', timeout: 10000 });

    await textarea.fill('Hi');
    await authedPage.keyboard.press('Enter');

    await expect(authedPage.locator('.assistant-text')).toBeVisible({ timeout: 10000 });

    const overflow = await authedPage.evaluate(() => {
      const messages = document.querySelector('.messages') as HTMLElement;
      return messages ? messages.scrollWidth > messages.clientWidth : false;
    });
    expect(overflow).toBe(false);
  });

  test('"New conversation" button clears messages', async ({ authedPage }) => {
    await authedPage.goto('/chat');
    const textarea = authedPage.locator('textarea');
    await textarea.waitFor({ state: 'visible', timeout: 10000 });

    // Send a message so there is something to clear.
    await textarea.fill('Test message');
    await authedPage.keyboard.press('Enter');
    await expect(authedPage.locator('.user-bubble').first()).toContainText('Test message', {
      timeout: 5000,
    });

    // Click "New conversation" — clears messages.
    await authedPage.locator('button.new-chat-btn').click();
    await expect(authedPage.locator('.user-bubble')).toHaveCount(0, { timeout: 3000 });
  });
});
