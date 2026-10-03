<script lang="ts">
	import Modal from '$lib/components/Modal.svelte';

	export let open = false;
	export let closes = 0;
</script>

<button id="outside-trigger" on:click={() => (open = true)}>Open</button>

<Modal bind:open title="Test Modal" on:close={() => closes++}>
	<button id="first-btn">First</button>
	<button id="parent-close" on:click={() => (open = false)}>Done</button>

	<svelte:fragment slot="footer">
		<button id="footer-btn">Footer</button>
	</svelte:fragment>
</Modal>
<output id="closes">{closes}</output>
