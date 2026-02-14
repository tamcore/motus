<script lang="ts">
	import { createEventDispatcher } from 'svelte';
	import { MAP_OVERLAYS } from '$lib/utils/map-overlays';
	import type { MapOverlay } from '$lib/utils/map-overlays';

	export let selectedOverlayId: string = 'none';
	export let opacity: number = 80;

	const dispatch = createEventDispatcher<{
		change: { overlayId: string; opacity: number };
	}>();

	let expanded = false;

	$: selectedOverlay = MAP_OVERLAYS.find((o) => o.id === selectedOverlayId) || MAP_OVERLAYS[0];
	$: hasActiveOverlay = selectedOverlayId !== 'none';

	function selectOverlay(overlay: MapOverlay) {
		selectedOverlayId = overlay.id;
		dispatch('change', { overlayId: selectedOverlayId, opacity });
	}

	function handleOpacityChange(event: Event) {
		const target = event.target as HTMLInputElement;
		opacity = parseInt(target.value, 10);
		dispatch('change', { overlayId: selectedOverlayId, opacity });
	}

	function togglePanel() {
		expanded = !expanded;
	}

	function handleKeydown(event: KeyboardEvent) {
		if (event.key === 'Escape' && expanded) {
			expanded = false;
		}
	}
</script>

<svelte:window on:keydown={handleKeydown} />

<div class="layer-control" class:expanded>
	<button
		class="layer-toggle"
		class:active={hasActiveOverlay}
		on:click={togglePanel}
		aria-label="Map layers"
		aria-expanded={expanded}
		title="Map layers"
	>
		<svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
			<polygon points="12 2 2 7 12 12 22 7 12 2" />
			<polyline points="2 17 12 22 22 17" />
			<polyline points="2 12 12 17 22 12" />
		</svg>
	</button>

	{#if expanded}
		<div class="layer-panel" role="dialog" aria-label="Map layer selection">
			<div class="panel-header">
				<span class="panel-title">Map Layers</span>
			</div>

			<div class="overlay-list" role="radiogroup" aria-label="Map overlay">
				{#each MAP_OVERLAYS as overlay (overlay.id)}
					<button
						class="overlay-option"
						class:selected={selectedOverlayId === overlay.id}
						on:click={() => selectOverlay(overlay)}
						role="radio"
						aria-checked={selectedOverlayId === overlay.id}
					>
						<span class="overlay-radio" class:checked={selectedOverlayId === overlay.id}></span>
						<span class="overlay-name">{overlay.name}</span>
					</button>
				{/each}
			</div>

			{#if hasActiveOverlay}
				<div class="opacity-control">
					<label for="overlay-opacity" class="opacity-label">
						Opacity: {opacity}%
					</label>
					<input
						id="overlay-opacity"
						type="range"
						min="10"
						max="100"
						step="5"
						value={opacity}
						on:input={handleOpacityChange}
						class="opacity-slider"
					/>
				</div>
			{/if}
		</div>
	{/if}
</div>

<style>
	.layer-control {
		position: absolute;
		top: var(--space-3);
		right: 56px;
		z-index: 500;
	}

	.layer-toggle {
		display: flex;
		align-items: center;
		justify-content: center;
		width: 36px;
		height: 36px;
		background-color: var(--bg-secondary);
		border: 1px solid var(--border-color);
		border-radius: var(--radius-md);
		color: var(--text-secondary);
		cursor: pointer;
		box-shadow: var(--shadow-md);
		transition: all var(--transition-fast);
	}

	.layer-toggle:hover {
		background-color: var(--bg-hover);
		color: var(--text-primary);
	}

	.layer-toggle.active {
		color: var(--accent-primary);
		border-color: var(--accent-primary);
	}

	.layer-panel {
		position: absolute;
		top: calc(100% + var(--space-2));
		right: 0;
		min-width: 220px;
		background-color: var(--bg-secondary);
		border: 1px solid var(--border-color);
		border-radius: var(--radius-lg);
		box-shadow: var(--shadow-lg);
		overflow: hidden;
	}

	.panel-header {
		padding: var(--space-3) var(--space-4);
		border-bottom: 1px solid var(--border-color);
	}

	.panel-title {
		font-size: var(--text-sm);
		font-weight: var(--font-semibold);
		color: var(--text-primary);
	}

	.overlay-list {
		padding: var(--space-2) 0;
	}

	.overlay-option {
		display: flex;
		align-items: center;
		gap: var(--space-3);
		width: 100%;
		padding: var(--space-2) var(--space-4);
		background: none;
		border: none;
		cursor: pointer;
		text-align: left;
		color: var(--text-secondary);
		font-size: var(--text-sm);
		transition: all var(--transition-fast);
	}

	.overlay-option:hover {
		background-color: var(--bg-hover);
		color: var(--text-primary);
	}

	.overlay-option.selected {
		color: var(--accent-primary);
	}

	.overlay-radio {
		width: 14px;
		height: 14px;
		border: 2px solid var(--border-color);
		border-radius: 50%;
		flex-shrink: 0;
		position: relative;
		transition: border-color var(--transition-fast);
	}

	.overlay-radio.checked {
		border-color: var(--accent-primary);
	}

	.overlay-radio.checked::after {
		content: '';
		position: absolute;
		top: 2px;
		left: 2px;
		width: 6px;
		height: 6px;
		border-radius: 50%;
		background-color: var(--accent-primary);
	}

	.overlay-name {
		flex: 1;
		white-space: nowrap;
	}

	.opacity-control {
		padding: var(--space-3) var(--space-4);
		border-top: 1px solid var(--border-color);
	}

	.opacity-label {
		display: block;
		font-size: var(--text-xs);
		color: var(--text-tertiary);
		margin-bottom: var(--space-2);
	}

	.opacity-slider {
		width: 100%;
		height: 4px;
		-webkit-appearance: none;
		appearance: none;
		background: var(--bg-tertiary);
		border-radius: 2px;
		outline: none;
		cursor: pointer;
	}

	.opacity-slider::-webkit-slider-thumb {
		-webkit-appearance: none;
		appearance: none;
		width: 14px;
		height: 14px;
		border-radius: 50%;
		background: var(--accent-primary);
		cursor: pointer;
		border: 2px solid var(--bg-secondary);
		box-shadow: var(--shadow-sm);
	}

	.opacity-slider::-moz-range-thumb {
		width: 14px;
		height: 14px;
		border-radius: 50%;
		background: var(--accent-primary);
		cursor: pointer;
		border: 2px solid var(--bg-secondary);
		box-shadow: var(--shadow-sm);
	}

	@media (max-width: 768px) {
		.layer-control {
			right: 10px;
			top: var(--space-2);
		}

		.layer-panel {
			min-width: 200px;
		}
	}
</style>
