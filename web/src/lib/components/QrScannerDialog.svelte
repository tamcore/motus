<script lang="ts">
	import { onDestroy, tick } from 'svelte';
	import jsQR from 'jsqr';
	import Button from './Button.svelte';
	import Modal from './Modal.svelte';

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
</script>

<Modal {open} title="Scan QR Code" on:close={handleClose}>
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

	<svelte:fragment slot="footer">
		<div class="dialog-footer">
			<Button variant="secondary" on:click={handleClose}>Cancel</Button>
		</div>
	</svelte:fragment>
</Modal>

<style>
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
		color: var(--text-secondary);
	}

	.status-message.error {
		color: var(--error);
	}

	.spinner {
		width: 2rem;
		height: 2rem;
		border: 3px solid var(--border-color);
		border-top-color: var(--accent-primary);
		border-radius: 50%;
		animation: spin 0.8s linear infinite;
	}

	.dialog-footer {
		display: flex;
		justify-content: flex-end;
	}
</style>
