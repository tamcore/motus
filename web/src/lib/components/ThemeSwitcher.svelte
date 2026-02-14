<script lang="ts">
	import { theme } from '$lib/stores/theme';

	const icons: Record<string, string> = {
		dark: '\u263D',
		auto: '\u21BB',
		light: '\u2600'
	};

	const labels: Record<string, string> = {
		dark: 'Dark mode',
		auto: 'Auto mode',
		light: 'Light mode'
	};

	function cycleTheme() {
		const order: Array<'dark' | 'auto' | 'light'> = ['dark', 'auto', 'light'];
		const idx = order.indexOf($theme);
		const next = order[(idx + 1) % order.length];
		theme.setTheme(next);
	}
</script>

<button
	on:click={cycleTheme}
	class="theme-switcher"
	aria-label="Toggle theme"
	title={labels[$theme]}
>
	<span class="icon">{icons[$theme]}</span>
	<span class="label">{$theme}</span>
</button>

<style>
	.theme-switcher {
		display: flex;
		align-items: center;
		gap: var(--space-2);
		padding: var(--space-2) var(--space-3);
		background-color: var(--bg-secondary);
		border: 1px solid var(--border-color);
		border-radius: var(--radius-md);
		color: var(--text-primary);
		cursor: pointer;
		font-size: var(--text-sm);
		transition: all var(--transition-fast);
	}

	.theme-switcher:hover {
		background-color: var(--bg-hover);
		border-color: var(--border-hover);
	}

	.icon {
		font-size: var(--text-lg);
	}

	.label {
		text-transform: capitalize;
	}
</style>
