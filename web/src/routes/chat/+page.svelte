<script lang="ts">
	import { onDestroy } from 'svelte';
	import { chatMessages, chatLoading, chatError, sendMessage } from '$lib/stores/chat';
	import MarkdownMessage from '$lib/components/MarkdownMessage.svelte';

	let input = '';
	let messagesEl: HTMLElement;

	$: if ($chatMessages) {
		setTimeout(() => messagesEl?.scrollTo({ top: messagesEl.scrollHeight, behavior: 'smooth' }), 50);
	}

	async function submit() {
		const text = input.trim();
		if (!text || $chatLoading) return;
		input = '';
		await sendMessage(text);
	}

	function onKeydown(e: KeyboardEvent) {
		if (e.key === 'Enter' && !e.shiftKey) {
			e.preventDefault();
			submit();
		}
	}

	onDestroy(() => {});
</script>

<div class="chat-page">
	<div class="messages" bind:this={messagesEl}>
		{#each $chatMessages as msg, i (i)}
			<div class="message {msg.role}">
				{#if msg.role === 'user'}
					<div class="bubble user-bubble">{msg.content}</div>
				{:else}
					<div class="bubble assistant-bubble">
						{#if msg.toolCalls && msg.toolCalls.length > 0}
							{#each msg.toolCalls as tc}
								<details class="tool-card">
									<summary>Used <code>{tc.name}</code></summary>
									{#if tc.error}
										<pre class="tool-error">{tc.error}</pre>
									{:else if tc.result !== undefined}
										<pre class="tool-result">{JSON.stringify(tc.result, null, 2)}</pre>
									{:else}
										<span class="tool-pending">Running…</span>
									{/if}
								</details>
							{/each}
						{/if}
						{#if msg.content}
							<MarkdownMessage content={msg.content} />
						{:else if !msg.toolCalls?.length}
							<span class="typing">▋</span>
						{/if}
					</div>
				{/if}
			</div>
		{/each}
		{#if $chatLoading && $chatMessages[$chatMessages.length - 1]?.role !== 'assistant'}
			<div class="message assistant"><div class="bubble assistant-bubble"><span class="typing">▋</span></div></div>
		{/if}
	</div>

	{#if $chatError}
		<div class="error-banner">{$chatError}</div>
	{/if}

	<form class="input-area" on:submit|preventDefault={submit}>
		<textarea
			bind:value={input}
			on:keydown={onKeydown}
			placeholder="Ask about your devices, trips, or create a geofence…"
			rows="2"
			disabled={$chatLoading}
		></textarea>
		<button type="submit" disabled={$chatLoading || !input.trim()}>
			{$chatLoading ? '…' : 'Send'}
		</button>
	</form>
</div>

<style>
	.chat-page {
		display: flex;
		flex-direction: column;
		height: calc(100vh - var(--nav-height, 83px));
		max-width: 800px;
		margin: 0 auto;
		padding: 1rem;
		gap: 0.75rem;
		overflow: hidden;
	}

	.messages {
		flex: 1;
		overflow-y: auto;
		display: flex;
		flex-direction: column;
		gap: 0.75rem;
	}

	.message {
		display: flex;
	}

	.message.user {
		justify-content: flex-end;
	}

	.bubble {
		max-width: 80%;
		padding: 0.6rem 0.9rem;
		border-radius: 12px;
		word-break: break-word;
	}

	.user-bubble {
		background: var(--color-primary, #2563eb);
		color: #fff;
		white-space: pre-wrap;
	}

	.assistant-bubble {
		background: var(--color-surface-alt, #f3f4f6);
		color: var(--color-text, #111);
	}

	.typing {
		animation: blink 1s step-end infinite;
	}

	@keyframes blink {
		50% { opacity: 0; }
	}

	.tool-card {
		margin-bottom: 0.4rem;
		border: 1px solid var(--color-border, #e5e7eb);
		border-radius: 6px;
		padding: 0.25rem 0.5rem;
		font-size: 0.85rem;
	}

	.tool-card summary {
		cursor: pointer;
	}

	.tool-result, .tool-error {
		font-size: 0.75rem;
		overflow-x: auto;
		margin: 0.25rem 0 0;
	}

	.tool-error {
		color: var(--color-danger, #dc2626);
	}

	.tool-pending {
		font-size: 0.75rem;
		color: var(--color-muted, #6b7280);
	}

	.error-banner {
		background: var(--color-danger-light, #fee2e2);
		color: var(--color-danger, #dc2626);
		padding: 0.5rem 0.75rem;
		border-radius: 6px;
		font-size: 0.875rem;
	}

	.input-area {
		display: flex;
		gap: 0.5rem;
		align-items: flex-end;
	}

	.input-area textarea {
		flex: 1;
		resize: none;
		padding: 0.6rem 0.75rem;
		border: 1px solid var(--color-border, #d1d5db);
		border-radius: 8px;
		font-family: inherit;
		font-size: 0.95rem;
	}

	.input-area button {
		padding: 0.6rem 1.2rem;
		border-radius: 8px;
		background: var(--color-primary, #2563eb);
		color: #fff;
		border: none;
		cursor: pointer;
		font-size: 0.95rem;
	}

	.input-area button:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}
</style>
