<script lang="ts">
	export let width: string = '100%';
	export let height: string = '1rem';
	export let variant: 'text' | 'rect' | 'circle' = 'text';
</script>

<div
	class="skeleton skeleton-{variant}"
	style="width: {width}; height: {height};"
	role="presentation"
	aria-hidden="true"
></div>

<style>
	.skeleton {
		background: linear-gradient(
			90deg,
			var(--bg-secondary) 0%,
			var(--bg-tertiary) 50%,
			var(--bg-secondary) 100%
		);
		background-size: 200% 100%;
		animation: loading 1.5s ease-in-out infinite;
	}

	.skeleton-text {
		border-radius: var(--radius-sm);
	}

	.skeleton-rect {
		border-radius: var(--radius-md);
	}

	.skeleton-circle {
		border-radius: 50%;
	}

	@keyframes loading {
		0% {
			background-position: 200% 0;
		}
		100% {
			background-position: -200% 0;
		}
	}
</style>
