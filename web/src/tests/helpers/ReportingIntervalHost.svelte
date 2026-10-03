<script lang="ts">
	import ReportingIntervalPicker from '$lib/components/ReportingIntervalPicker.svelte';

	export let value = '';
	/** Value the "set externally" button assigns, simulating a parent update. */
	export let externalValue = '';
</script>

<ReportingIntervalPicker bind:value />
<output id="bound-value">{value}</output>
<button type="button" id="parent-reset" on:click={() => (value = '')}>reset</button>
<button type="button" id="parent-set" on:click={() => (value = externalValue)}>set</button>
