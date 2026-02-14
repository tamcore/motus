<script lang="ts">
	import QRCode from 'qrcode';
	import Button from './Button.svelte';

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

	function handleBackdropClick(event: MouseEvent) {
		if (event.target === event.currentTarget) {
			onClose();
		}
	}
</script>

{#if open}
	<!-- svelte-ignore a11y_click_events_have_key_events -->
	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<div class="dialog-backdrop" on:click={handleBackdropClick}>
		<div class="dialog" role="dialog" aria-modal="true" aria-labelledby="dialog-title">
			<div class="dialog-header">
				<h2 id="dialog-title">Scan QR Code</h2>
				<button class="close-button" on:click={onClose} aria-label="Close dialog">
					<svg viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2">
						<path d="M18 6L6 18M6 6l12 12"/>
					</svg>
				</button>
			</div>

			<div class="dialog-content">
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
			</div>

			<div class="dialog-footer">
				<Button variant="secondary" on:click={onClose}>Close</Button>
			</div>
		</div>
	</div>
{/if}

<style>
	.dialog-backdrop {
		position: fixed;
		top: 0;
		left: 0;
		right: 0;
		bottom: 0;
		background-color: rgba(0, 0, 0, 0.5);
		display: flex;
		align-items: center;
		justify-content: center;
		z-index: 1000;
		padding: var(--space-4);
	}

	.dialog {
		background-color: var(--bg-secondary);
		border-radius: var(--radius-xl);
		box-shadow: var(--shadow-xl);
		max-width: 500px;
		width: 100%;
		max-height: 90vh;
		overflow: hidden;
		display: flex;
		flex-direction: column;
	}

	.dialog-header {
		display: flex;
		align-items: center;
		justify-content: space-between;
		padding: var(--space-6);
		border-bottom: 1px solid var(--border-primary);
	}

	.dialog-header h2 {
		margin: 0;
		font-size: var(--text-xl);
		color: var(--text-primary);
	}

	.close-button {
		background: none;
		border: none;
		padding: var(--space-2);
		cursor: pointer;
		color: var(--text-secondary);
		display: flex;
		align-items: center;
		justify-content: center;
		border-radius: var(--radius-md);
		transition: all 0.2s;
	}

	.close-button:hover {
		background-color: var(--bg-hover);
		color: var(--text-primary);
	}

	.dialog-content {
		padding: var(--space-6);
		overflow-y: auto;
		flex: 1;
	}

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
		padding: var(--space-6);
		border-top: 1px solid var(--border-primary);
		display: flex;
		justify-content: flex-end;
		gap: var(--space-3);
	}
</style>
