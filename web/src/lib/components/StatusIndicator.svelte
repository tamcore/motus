<script lang="ts">
	import type { DeviceStatus, NotificationLog } from '$lib/types/api';

	export let status: DeviceStatus | NotificationLog['status'] = 'offline';
	export let showLabel = false;

	const KINDS: Record<typeof status, DeviceStatus | 'idle'> = {
		online: 'online',
		offline: 'offline',
		unknown: 'unknown',
		sent: 'online',
		queued: 'idle',
		failed: 'offline'
	};
	$: kind = KINDS[status] ?? 'unknown';
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
	.status-unknown {
		background-color: var(--text-tertiary);
	}

	.label {
		font-size: var(--text-sm);
		color: var(--text-secondary);
		text-transform: capitalize;
	}
</style>
