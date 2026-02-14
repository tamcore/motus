<script lang="ts">
	import { createEventDispatcher } from 'svelte';

	export let playing = false;
	export let speed = 1;
	export let totalPositions = 0;
	export let currentIndex = 0;

	const dispatch = createEventDispatcher();
	const speeds = [1, 2, 4, 8];
</script>

<div class="playback-controls">
	<div class="timeline">
		<input type="range" min="0" max={totalPositions - 1} value={currentIndex}
			on:input={(e) => dispatch('timelineChange', e)} class="timeline-slider"
			aria-label="Playback timeline" />
		<div class="timeline-labels">
			<span>{currentIndex + 1} / {totalPositions}</span>
		</div>
	</div>

	<div class="control-buttons">
		<button class="control-btn" on:click={() => dispatch('stop')} title="Stop"
			aria-label="Stop playback">Stop</button>

		{#if playing}
			<button class="control-btn primary" on:click={() => dispatch('pause')}
				title="Pause" aria-label="Pause playback">Pause</button>
		{:else}
			<button class="control-btn primary" on:click={() => dispatch('play')}
				title="Play" aria-label="Start playback">Play</button>
		{/if}

		<div class="speed-selector">
			<span class="speed-label">Speed:</span>
			{#each speeds as s}
				<button class="speed-btn" class:active={speed === s}
					on:click={() => dispatch('speedChange', s)}
					aria-label="Set speed to {s}x">{s}x</button>
			{/each}
		</div>

		<button class="control-btn" on:click={() => dispatch('download')}
			title="Download GPX" aria-label="Download GPX file">GPX</button>
	</div>
</div>

<style>
	.playback-controls { display: flex; flex-direction: column; gap: var(--space-3); }
	.timeline { display: flex; flex-direction: column; gap: var(--space-2); }
	.timeline-slider {
		width: 100%; height: 6px; border-radius: var(--radius-full);
		background: var(--bg-tertiary); outline: none; cursor: pointer;
		-webkit-appearance: none; appearance: none;
	}
	.timeline-slider::-webkit-slider-thumb {
		-webkit-appearance: none; width: 16px; height: 16px;
		border-radius: 50%; background: var(--accent-primary); cursor: pointer;
	}
	.timeline-slider::-moz-range-thumb {
		width: 16px; height: 16px; border: none;
		border-radius: 50%; background: var(--accent-primary); cursor: pointer;
	}
	.timeline-labels {
		display: flex; justify-content: center;
		font-size: var(--text-sm); color: var(--text-secondary);
	}
	.control-buttons { display: flex; align-items: center; gap: var(--space-3); }
	.control-btn {
		padding: var(--space-2) var(--space-4); background-color: var(--bg-tertiary);
		border: 1px solid var(--border-color); border-radius: var(--radius-md);
		color: var(--text-primary); cursor: pointer; font-size: var(--text-sm);
		transition: all var(--transition-fast);
	}
	.control-btn:hover { background-color: var(--bg-hover); }
	.control-btn.primary { background-color: var(--accent-primary); color: var(--text-inverse); border-color: var(--accent-primary); }
	.speed-selector { display: flex; align-items: center; gap: var(--space-2); margin-left: auto; }
	.speed-label { color: var(--text-secondary); font-size: var(--text-sm); }
	.speed-btn {
		padding: var(--space-1) var(--space-3); background-color: var(--bg-tertiary);
		border: 1px solid var(--border-color); border-radius: var(--radius-sm);
		color: var(--text-primary); cursor: pointer; font-size: var(--text-sm);
		transition: all var(--transition-fast);
	}
	.speed-btn:hover { background-color: var(--bg-hover); }
	.speed-btn.active {
		background-color: var(--accent-primary); color: var(--text-inverse);
		border-color: var(--accent-primary);
	}
</style>
