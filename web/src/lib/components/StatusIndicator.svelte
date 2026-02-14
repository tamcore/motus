<script lang="ts">
	export let status: 'online' | 'offline' | 'idle' | 'moving' = 'offline';
	export let showLabel = false;
</script>

<div class="status-indicator" role="status" aria-label="{status}">
	<span class="dot status-{status}"></span>
	{#if showLabel}
		<span class="label">{status}</span>
	{/if}
</div>

<style>
	.status-indicator {
		display: inline-flex;
		align-items: center;
		gap: var(--space-2);
	}

	.dot {
		width: 8px;
		height: 8px;
		border-radius: 50%;
	}

	.status-online {
		background-color: var(--status-online);
	}
	.status-offline {
		background-color: var(--status-offline);
	}
	.status-idle {
		background-color: var(--status-idle);
	}
	.status-moving {
		background-color: var(--status-moving);
	}

	.label {
		font-size: var(--text-sm);
		color: var(--text-secondary);
		text-transform: capitalize;
	}
</style>
