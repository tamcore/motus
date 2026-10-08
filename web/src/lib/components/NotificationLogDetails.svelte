<script lang="ts">
	import type { NotificationLog } from '$lib/types/api';
	import { formatLogTime, logChanges, logSubject } from '$lib/utils/notificationLog';

	export let log: NotificationLog;

	$: subject = logSubject(log);
	$: changes = logChanges(log);
</script>

{#if subject || changes.length > 0}
	<div class="log-event" data-testid="log-event">
		{#if subject}
			<div class="log-subject">
				{subject}
				{#if log.eventTime}
					<span class="log-event-time">@ {formatLogTime(log.eventTime)}</span>
				{/if}
			</div>
		{/if}
		{#if changes.length > 0}
			<ul class="log-changes">
				{#each changes as change}
					<li>
						<span class="change-label">{change.label}:</span>
						{#if change.from !== undefined}
							<span class="change-from">{change.from}</span>
							<span class="change-arrow" aria-label="changed to">→</span>
						{/if}
						<span class="change-to">{change.to}</span>
					</li>
				{/each}
			</ul>
		{/if}
	</div>
{:else if log.eventId == null}
	<div class="log-event log-event-missing">Triggering event no longer available</div>
{/if}

<style>
	.log-event {
		display: flex;
		flex-direction: column;
		gap: var(--space-1);
		font-size: var(--text-sm);
	}
	.log-subject {
		color: var(--text-primary);
		font-weight: var(--font-medium);
	}
	.log-event-time {
		color: var(--text-secondary);
		font-weight: normal;
		font-size: var(--text-xs);
	}
	.log-changes {
		list-style: none;
		margin: 0;
		padding: 0;
		display: flex;
		flex-wrap: wrap;
		gap: var(--space-1) var(--space-3);
	}
	.change-label {
		color: var(--text-secondary);
	}
	.change-from {
		color: var(--text-secondary);
		text-decoration: line-through;
	}
	.change-arrow {
		color: var(--text-tertiary);
	}
	.change-to {
		color: var(--text-primary);
		font-weight: var(--font-medium);
	}
	.log-event-missing {
		color: var(--text-tertiary);
		font-style: italic;
	}
</style>
