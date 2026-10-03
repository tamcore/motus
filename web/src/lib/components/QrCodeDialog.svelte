<script lang="ts">
	import QRCode from 'qrcode';
	import Button from './Button.svelte';
	import Modal from './Modal.svelte';

	export let open = false;
	export let onClose: () => void;
	export let apiToken: string | undefined = undefined;

	let canvas: HTMLCanvasElement;
	let error = '';

	// Derive server URL once (stable across renders)
	const serverUrl = typeof window !== 'undefined' ? window.location.origin : '';

	// Build the QR data: plain server URL or server URL with embedded token
	$: qrData = apiToken
		? `${serverUrl}?token=${apiToken}`
		: serverUrl;

	// Regenerate QR code reactively when dialog opens, canvas binds, or data changes
	$: if (open && canvas && qrData) {
		generateQrCode(canvas, qrData);
	}

	async function generateQrCode(target: HTMLCanvasElement, data: string) {
		try {
			await QRCode.toCanvas(target, data, {
				width: 256,
				margin: 2,
				color: {
					dark: '#000000',
					light: '#FFFFFF'
				}
			});
			error = '';
		} catch (err) {
			error = 'Failed to generate QR code';
			console.error('QR Code generation error:', err);
		}
	}
</script>

<Modal {open} title="Scan QR Code" on:close={onClose}>
	<p class="instruction">
		{#if apiToken}
			Scan this QR code with Traccar Manager to auto-configure the server connection and authenticate with the embedded API key.
		{:else}
			Use Traccar Manager app to scan this QR code and automatically configure your server connection.
		{/if}
	</p>

	<div class="qr-container">
		{#if error}
			<div class="error-message">{error}</div>
		{:else}
			<canvas bind:this={canvas}></canvas>
		{/if}
	</div>

	<div class="server-url">
		<label for="server-url">{apiToken ? 'Connection URL:' : 'Server URL:'}</label>
		<input
			type="text"
			id="server-url"
			value={qrData}
			readonly
			on:click={(e) => e.currentTarget.select()}
		/>
	</div>

	{#if apiToken}
		<p class="token-notice">
			This QR code contains an embedded API key. Anyone who scans it will have access to your account. Share it only with trusted parties.
		</p>
	{/if}

	<svelte:fragment slot="footer">
		<div class="dialog-footer">
			<Button variant="secondary" on:click={onClose}>Close</Button>
		</div>
	</svelte:fragment>
</Modal>

<style>
	.instruction {
		color: var(--text-secondary);
		margin-bottom: var(--space-6);
		text-align: center;
		font-size: var(--text-sm);
		line-height: 1.5;
	}

	.qr-container {
		display: flex;
		justify-content: center;
		margin-bottom: var(--space-6);
		padding: var(--space-4);
		background-color: var(--bg-primary);
		border-radius: var(--radius-lg);
	}

	.qr-container canvas {
		display: block;
		max-width: 100%;
		height: auto;
	}

	.error-message {
		color: var(--error);
		text-align: center;
		padding: var(--space-4);
	}

	.token-notice {
		margin-top: var(--space-4);
		padding: var(--space-3) var(--space-4);
		background-color: rgba(255, 170, 0, 0.1);
		border: 1px solid rgba(255, 170, 0, 0.3);
		border-radius: var(--radius-md);
		color: var(--warning);
		font-size: var(--text-sm);
		line-height: 1.5;
	}

	.server-url {
		display: flex;
		flex-direction: column;
		gap: var(--space-2);
	}

	.server-url label {
		font-size: var(--text-sm);
		font-weight: 500;
		color: var(--text-secondary);
	}

	.server-url input {
		width: 100%;
		padding: var(--space-3);
		background-color: var(--bg-primary);
		border: 1px solid var(--border-primary);
		border-radius: var(--radius-md);
		color: var(--text-primary);
		font-family: monospace;
		font-size: var(--text-sm);
		cursor: pointer;
	}

	.server-url input:focus {
		outline: none;
		border-color: var(--accent-primary);
	}

	.dialog-footer {
		display: flex;
		justify-content: flex-end;
		gap: var(--space-3);
	}
</style>
