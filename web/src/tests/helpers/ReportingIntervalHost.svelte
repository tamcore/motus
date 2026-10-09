<script lang="ts">
	import ReportingIntervalPicker from '$lib/components/ReportingIntervalPicker.svelte';

	export let value = '';
	export let label: string | undefined = undefined;
	export let name: string | undefined = undefined;
</script>

<ReportingIntervalPicker bind:value {label} {name} />
<output id="bound-value">{value}</output>
