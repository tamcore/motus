<script lang="ts">
	/** A device status or a notification delivery status ('sent', 'queued', 'failed'). */
	export let status = 'offline';
	export let showLabel = false;

	const KINDS: Record<string, 'online' | 'idle' | 'moving'> = {
		online: 'online',
		idle: 'idle',
		moving: 'moving',
		sent: 'online',
		queued: 'idle'
	};
	$: kind = KINDS[status] ?? 'offline';
</script>

<div class="status-indicator" role="status" aria-label="{kind}">
	<span class="dot status-{kind}"></span>
	{#if showLabel}
		<span class="label">{kind}</span>
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
