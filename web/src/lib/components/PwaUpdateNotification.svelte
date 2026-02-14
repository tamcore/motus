<script lang="ts">
	import { fly } from 'svelte/transition';
	import { pwa, showUpdateNotification } from '$lib/stores/pwa';

	function handleUpdate() {
		pwa.applyUpdate();
	}
</script>

{#if $showUpdateNotification}
	<div
		class="update-notification"
		role="alert"
		aria-live="polite"
		transition:fly={{ y: -50, duration: 300 }}
	>
		<div class="update-content">
			<svg class="update-icon" width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
				<path d="M21 12a9 9 0 0 0-9-9 9.75 9.75 0 0 0-6.74 2.74L3 8"/>
				<path d="M3 3v5h5"/>
				<path d="M3 12a9 9 0 0 0 9 9 9.75 9.75 0 0 0 6.74-2.74L21 16"/>
				<path d="M21 21v-5h-5"/>
			</svg>
			<span class="update-text">A new version of Motus is available</span>
		</div>
		<button class="update-btn" on:click={handleUpdate}>
			Update now
		</button>
	</div>
{/if}

<style>
	.update-notification {
		position: fixed;
		top: var(--space-4);
		left: 50%;
		transform: translateX(-50%);
		background-color: var(--bg-secondary);
		border: 1px solid var(--accent-primary);
		border-radius: var(--radius-xl);
		box-shadow: var(--shadow-lg);
		padding: var(--space-3) var(--space-4);
		display: flex;
		align-items: center;
		gap: var(--space-4);
		z-index: 10002;
		max-width: 90vw;
	}

	.update-content {
		display: flex;
		align-items: center;
		gap: var(--space-3);
	}

	.update-icon {
		color: var(--accent-primary);
		flex-shrink: 0;
	}

	.update-text {
		color: var(--text-primary);
		font-size: var(--text-sm);
		white-space: nowrap;
	}

	.update-btn {
		padding: var(--space-2) var(--space-3);
		background-color: var(--accent-primary);
		color: var(--text-inverse);
		border: none;
		border-radius: var(--radius-md);
		font-size: var(--text-sm);
		font-weight: var(--font-medium);
		cursor: pointer;
		transition: background-color var(--transition-fast);
		white-space: nowrap;
	}

	.update-btn:hover {
		background-color: var(--accent-hover);
	}

	.update-btn:active {
		background-color: var(--accent-active);
	}

	@media (max-width: 480px) {
		.update-notification {
			left: var(--space-2);
			right: var(--space-2);
			transform: none;
		}

		.update-text {
			white-space: normal;
			font-size: var(--text-xs);
		}
	}
</style>
