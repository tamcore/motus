<!--
  PullToRefresh.svelte — mobile pull-down-to-refresh gesture.

  Wraps page content and detects a downward pull when scrolled to top.
  Calls the current page's refresh handler from the refreshHandler store.
  Only activates on touch devices; invisible on desktop.
-->
<script lang="ts">
	import { onMount } from 'svelte';
	import { refreshHandler } from '$lib/stores/refresh';

	const THRESHOLD = 60;
	const MAX_PULL = 120;

	let container: HTMLElement;
	let startY = 0;
	let pullDistance = 0;
	let refreshing = false;
	let pulling = false;

	function onTouchStart(e: TouchEvent) {
		if (refreshing || !$refreshHandler) return;
		if (container.scrollTop > 0) return;
		startY = e.touches[0].clientY;
		pulling = true;
	}

	function onTouchMove(e: TouchEvent) {
		if (!pulling || refreshing) return;

		const y = e.touches[0].clientY;
		const delta = y - startY;

		if (delta < 0) {
			pullDistance = 0;
			return;
		}

		// Resist pull with diminishing returns
		pullDistance = Math.min(delta * 0.5, MAX_PULL);

		if (pullDistance > 0) {
			e.preventDefault();
		}
	}

	async function onTouchEnd() {
		if (!pulling) return;
		pulling = false;

		if (pullDistance >= THRESHOLD && $refreshHandler && !refreshing) {
			refreshing = true;
			pullDistance = THRESHOLD;
			try {
				await $refreshHandler();
			} finally {
				refreshing = false;
				pullDistance = 0;
			}
		} else {
			pullDistance = 0;
		}
	}

	onMount(() => {
		// Passive: false needed on touchmove to allow preventDefault
		container.addEventListener('touchmove', onTouchMove, { passive: false });
		return () => {
			container.removeEventListener('touchmove', onTouchMove);
		};
	});
</script>

<!-- svelte-ignore a11y-no-static-element-interactions -->
<div
	class="ptr-container"
	bind:this={container}
	on:touchstart={onTouchStart}
	on:touchend={onTouchEnd}
	on:touchcancel={onTouchEnd}
>
	{#if $refreshHandler}
		<div
			class="ptr-indicator"
			class:ptr-active={pullDistance > 0 || refreshing}
			class:ptr-threshold={pullDistance >= THRESHOLD || refreshing}
			style="transform: translateY({pullDistance - 40}px); opacity: {Math.min(pullDistance / THRESHOLD, 1)}"
		>
			{#if refreshing}
				<div class="ptr-spinner"></div>
			{:else}
				<svg
					class="ptr-arrow"
					class:ptr-arrow-flipped={pullDistance >= THRESHOLD}
					viewBox="0 0 24 24"
					width="24"
					height="24"
					fill="none"
					stroke="currentColor"
					stroke-width="2"
					stroke-linecap="round"
					stroke-linejoin="round"
				>
					<polyline points="7 13 12 18 17 13" />
					<line x1="12" y1="18" x2="12" y2="6" />
				</svg>
			{/if}
		</div>
	{/if}

	<div class="ptr-content" class:ptr-pulling={pullDistance > 0} style={pullDistance > 0 ? `transform: translateY(${pullDistance}px)` : ''}>
		<slot />
	</div>
</div>

<style>
	.ptr-container {
		position: relative;
		overflow-y: auto;
		overflow-x: hidden;
		overscroll-behavior-y: contain;
		height: 100%;
		-webkit-overflow-scrolling: touch;
	}

	.ptr-indicator {
		position: absolute;
		top: 0;
		left: 0;
		right: 0;
		display: flex;
		align-items: center;
		justify-content: center;
		height: 40px;
		z-index: 10;
		pointer-events: none;
		transition: opacity 0.15s ease;
	}

	.ptr-indicator:not(.ptr-active) {
		opacity: 0 !important;
	}

	.ptr-arrow {
		color: var(--text-secondary);
		transition: transform 0.2s ease;
	}

	.ptr-arrow-flipped {
		transform: rotate(180deg);
	}

	.ptr-spinner {
		width: 24px;
		height: 24px;
		border: 3px solid var(--border-color);
		border-top-color: var(--accent-primary);
		border-radius: 50%;
		animation: ptr-spin 0.7s linear infinite;
	}

	@keyframes ptr-spin {
		to {
			transform: rotate(360deg);
		}
	}

	.ptr-content {
		min-height: 100%;
		transition: transform 0.2s ease;
	}

	.ptr-content.ptr-pulling {
		will-change: transform;
	}
</style>
