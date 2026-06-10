<script lang="ts">
	import Modal from '$lib/components/Modal.svelte';

	export let open = false;
</script>

<button id="outside-trigger" on:click={() => (open = true)}>Open</button>

<Modal bind:open title="Test Modal">
	<button id="first-btn">First</button>
	<button id="second-btn">Second</button>

	<svelte:fragment slot="footer">
		<button id="footer-btn">Footer</button>
	</svelte:fragment>
</Modal>
