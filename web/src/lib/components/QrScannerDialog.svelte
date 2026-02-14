<script lang="ts">
	import { onDestroy, tick } from 'svelte';
	import jsQR from 'jsqr';
	import Button from './Button.svelte';

	export let open: boolean = false;
	export let onClose: () => void;
	export let onScan: (url: string) => void;

	type ScanState = 'idle' | 'requesting' | 'scanning' | 'denied' | 'error' | 'success';

	let state: ScanState = 'idle';
	let video: HTMLVideoElement;
	let canvas: HTMLCanvasElement;
	let stream: MediaStream | null = null;
	let animationFrameId: number | null = null;

	$: if (open) {
		if (state === 'idle') {
			startScanning();
		}
	} else {
		stopCamera();
		state = 'idle';
	}

	async function startScanning() {
		state = 'requesting';
		try {
			const newStream = await navigator.mediaDevices.getUserMedia({
				video: { facingMode: 'environment' }
			});

			// If the dialog was closed while waiting for permission, release the stream
			if (!open) {
				newStream.getTracks().forEach((t) => t.stop());
				return;
			}

			stream = newStream;
			state = 'scanning';

			// Wait for the DOM to update so the <video> element is mounted
			await tick();

			if (video && stream) {
				video.srcObject = stream;
				await video.play();
				scanFrame();
			}
		} catch (err: unknown) {
			if (!open) return;
			if (
				err instanceof Error &&
				(err.name === 'NotAllowedError' || err.name === 'PermissionDeniedError')
			) {
				state = 'denied';
			} else {
				state = 'error';
			}
		}
	}

	function scanFrame() {
		if (state !== 'scanning' || !video || !canvas) return;

		const context = canvas.getContext('2d');
		if (!context) return;

		if (video.readyState >= video.HAVE_ENOUGH_DATA) {
			canvas.width = video.videoWidth;
			canvas.height = video.videoHeight;
			context.drawImage(video, 0, 0, canvas.width, canvas.height);
			const imageData = context.getImageData(0, 0, canvas.width, canvas.height);
			const result = jsQR(imageData.data, imageData.width, imageData.height);
			if (result) {
				state = 'success';
				stopCamera();
				onScan(result.data);
				return;
			}
		}

		animationFrameId = requestAnimationFrame(scanFrame);
	}

	function stopCamera() {
		if (animationFrameId !== null) {
			cancelAnimationFrame(animationFrameId);
			animationFrameId = null;
		}
		if (stream) {
			stream.getTracks().forEach((track) => track.stop());
			stream = null;
		}
	}

	function handleClose() {
		stopCamera();
		state = 'idle';
		onClose();
	}

	onDestroy(() => {
		stopCamera();
	});

	function handleBackdropClick(event: MouseEvent) {
		if (event.target === event.currentTarget) {
			handleClose();
		}
	}
</script>

{#if open}
	<!-- svelte-ignore a11y_click_events_have_key_events -->
	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<div class="dialog-backdrop" on:click={handleBackdropClick}>
		<div class="dialog" role="dialog" aria-modal="true" aria-labelledby="scanner-title">
			<div class="dialog-header">
				<h2 id="scanner-title">Scan QR Code</h2>
				<button class="close-button" on:click={handleClose} aria-label="Close dialog">
					<svg viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2">
						<path d="M18 6L6 18M6 6l12 12"/>
					</svg>
				</button>
			</div>

			<div class="dialog-content">
				{#if state === 'requesting'}
					<div class="status-message">
						<div class="spinner" aria-hidden="true"></div>
						<p>Requesting camera access…</p>
					</div>

				{:else if state === 'scanning'}
					<p class="instruction">
						Point the camera at the QR code shown in Motus settings.
					</p>
					<div class="camera-container">
						<!-- svelte-ignore a11y_media_has_caption -->
						<video bind:this={video} class="camera-feed" playsinline autoplay muted></video>
					</div>
					<!-- Off-screen canvas for frame capture; not visible to user -->
					<canvas bind:this={canvas} class="offscreen-canvas" aria-hidden="true"></canvas>

				{:else if state === 'denied'}
					<div class="status-message error">
						<svg viewBox="0 0 24 24" width="48" height="48" fill="none" stroke="currentColor" stroke-width="1.5">
							<circle cx="12" cy="12" r="10"/>
							<path d="M12 8v4m0 4h.01"/>
						</svg>
						<p>Camera access was denied.</p>
						<p class="status-hint">Please allow camera access in your device settings and try again.</p>
					</div>

				{:else if state === 'error'}
					<div class="status-message error">
						<svg viewBox="0 0 24 24" width="48" height="48" fill="none" stroke="currentColor" stroke-width="1.5">
							<circle cx="12" cy="12" r="10"/>
							<path d="M12 8v4m0 4h.01"/>
						</svg>
						<p>Could not start the camera.</p>
						<p class="status-hint">Make sure your device has a camera and try again.</p>
					</div>
				{/if}
			</div>

			<div class="dialog-footer">
				<Button variant="secondary" on:click={handleClose}>Cancel</Button>
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
		margin-bottom: var(--space-4);
		text-align: center;
		font-size: var(--text-sm);
		line-height: 1.5;
	}

	.camera-container {
		width: 100%;
		aspect-ratio: 4 / 3;
		background-color: #000;
		border-radius: var(--radius-lg);
		overflow: hidden;
		display: flex;
		align-items: center;
		justify-content: center;
	}

	.camera-feed {
		width: 100%;
		height: 100%;
		object-fit: cover;
		display: block;
	}

	.offscreen-canvas {
		position: absolute;
		left: -9999px;
		top: -9999px;
		width: 1px;
		height: 1px;
	}

	.status-message {
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		gap: var(--space-3);
		padding: var(--space-8) 0;
		color: var(--text-secondary);
		text-align: center;
	}

	.status-message p {
		margin: 0;
		font-size: var(--text-base);
	}

	.status-hint {
		font-size: var(--text-sm) !important;
		color: var(--text-muted, var(--text-secondary));
	}

	.status-message.error {
		color: var(--error);
	}

	.spinner {
		width: 2rem;
		height: 2rem;
		border: 3px solid var(--border-primary);
		border-top-color: var(--accent-primary);
		border-radius: 50%;
		animation: spin 0.8s linear infinite;
	}

	@keyframes spin {
		to {
			transform: rotate(360deg);
		}
	}

	.dialog-footer {
		padding: var(--space-6);
		border-top: 1px solid var(--border-primary);
		display: flex;
		justify-content: flex-end;
	}
</style>
