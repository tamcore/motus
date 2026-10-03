<script lang="ts">
	import ReportingIntervalPicker from '$lib/components/ReportingIntervalPicker.svelte';

	export let value = '';
</script>

<ReportingIntervalPicker bind:value />
<output id="bound-value">{value}</output>
