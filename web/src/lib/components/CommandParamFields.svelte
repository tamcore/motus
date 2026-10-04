<script lang="ts">
	import Input from './Input.svelte';
	import ReportingIntervalPicker from './ReportingIntervalPicker.svelte';

	export let type: string;
	export let frequency: string;
	export let sosNumber: string;
	export let speed: string;
	export let text: string;
</script>

{#if type === 'positionPeriodic'}
	<ReportingIntervalPicker bind:value={frequency} />
{:else if type === 'sosNumber'}
	<Input name="sosNumber" label="SOS Phone Number" placeholder="+1234567890" bind:value={sosNumber} />
{:else if type === 'setSpeedAlarm'}
	<Input name="speed" label="Speed Limit (km/h, 0 = disable)" placeholder="80" bind:value={speed} />
{:else if type === 'custom'}
	<Input name="text" label="Raw Command" placeholder="rconf" bind:value={text} />
{/if}
