<script lang="ts">
	import { marked } from 'marked';
	import DOMPurify from 'isomorphic-dompurify';

	export let content: string;

	marked.use({ gfm: true, breaks: true });

	$: html = DOMPurify.sanitize(marked.parse(content) as string);
</script>

<div class="markdown-body">{@html html}</div>

<style>
	.markdown-body {
		line-height: 1.6;
		word-break: break-word;
	}
	.markdown-body :global(p) {
		margin: 0.4em 0;
	}
	.markdown-body :global(p:first-child) {
		margin-top: 0;
	}
	.markdown-body :global(p:last-child) {
		margin-bottom: 0;
	}
	.markdown-body :global(h1),
	.markdown-body :global(h2),
	.markdown-body :global(h3) {
		margin: 0.6em 0 0.3em;
		font-weight: 600;
	}
	.markdown-body :global(ul),
	.markdown-body :global(ol) {
		padding-left: 1.4em;
		margin: 0.4em 0;
	}
	.markdown-body :global(li) {
		margin: 0.15em 0;
	}
	.markdown-body :global(code) {
		background: rgba(0, 0, 0, 0.07);
		padding: 0.1em 0.35em;
		border-radius: 3px;
		font-size: 0.88em;
		font-family: ui-monospace, monospace;
	}
	.markdown-body :global(pre) {
		background: rgba(0, 0, 0, 0.07);
		padding: 0.6em 0.8em;
		border-radius: 6px;
		overflow-x: auto;
		margin: 0.4em 0;
	}
	.markdown-body :global(pre code) {
		background: none;
		padding: 0;
		font-size: 0.85em;
	}
	.markdown-body :global(table) {
		border-collapse: collapse;
		width: 100%;
		margin: 0.5em 0;
		font-size: 0.9em;
	}
	.markdown-body :global(th),
	.markdown-body :global(td) {
		border: 1px solid rgba(0, 0, 0, 0.15);
		padding: 0.3em 0.6em;
		text-align: left;
	}
	.markdown-body :global(th) {
		background: rgba(0, 0, 0, 0.05);
		font-weight: 600;
	}
	.markdown-body :global(blockquote) {
		border-left: 3px solid rgba(0, 0, 0, 0.2);
		margin: 0.4em 0;
		padding: 0.1em 0.7em;
		color: var(--color-muted, #6b7280);
	}
	.markdown-body :global(a) {
		color: var(--color-primary, #2563eb);
		text-decoration: underline;
	}
	.markdown-body :global(hr) {
		border: none;
		border-top: 1px solid rgba(0, 0, 0, 0.15);
		margin: 0.5em 0;
	}
</style>
