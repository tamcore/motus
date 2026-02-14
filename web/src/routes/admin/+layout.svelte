<script lang="ts">
	import { page } from '$app/stores';

	const adminLinks = [
		{ href: '/admin/users', label: 'Users' },
		{ href: '/admin/statistics', label: 'Statistics' },
		{ href: '/admin/audit', label: 'Audit Log' }
	];
</script>

<div class="admin-layout">
	<nav class="admin-nav" aria-label="Admin navigation">
		{#each adminLinks as link}
			<a
				href={link.href}
				class="admin-nav-link"
				class:active={$page.url.pathname === link.href}
			>
				{link.label}
			</a>
		{/each}
	</nav>

	<slot />
</div>

<style>
	.admin-layout {
		display: flex;
		flex-direction: column;
	}

	.admin-nav {
		display: flex;
		gap: var(--space-1);
		padding: var(--space-3) var(--space-4);
		background-color: var(--bg-secondary);
		border-bottom: 1px solid var(--border-color);
		justify-content: center;
	}

	.admin-nav-link {
		padding: var(--space-2) var(--space-4);
		text-decoration: none;
		color: var(--text-secondary);
		font-size: var(--text-sm);
		font-weight: var(--font-medium);
		border-radius: var(--radius-md);
		transition: all var(--transition-fast);
	}

	.admin-nav-link:hover {
		color: var(--text-primary);
		background-color: var(--bg-hover);
	}

	.admin-nav-link.active {
		color: var(--accent-primary);
		background-color: rgba(0, 212, 255, 0.08);
	}

	@media (max-width: 768px) {
		.admin-nav {
			overflow-x: auto;
			justify-content: flex-start;
			-webkit-overflow-scrolling: touch;
		}

		.admin-nav-link {
			white-space: nowrap;
		}
	}
</style>
