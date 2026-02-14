<script lang="ts">
	import { createEventDispatcher } from 'svelte';
	import Modal from './Modal.svelte';
	import Button from './Button.svelte';
	import Input from './Input.svelte';
	import type { Calendar } from '$lib/types/api';
	import {
		CALENDAR_TEMPLATES,
		getScheduleSummary,
		getActiveStatus,
		validateIcalData,
		validateDateRangeConfig,
		buildDateRangeIcal,
		parseIcalToDateRangeConfig,
		type TemplateId,
		type RecurrenceType,
		type DateRangeBuilderConfig
	} from '$lib/utils/ical';

	export let open = false;
	export let calendar: Calendar | null = null;
	export let saving = false;

	const dispatch = createEventDispatcher<{
		save: { name: string; data: string };
		close: void;
	}>();

	// --- Mode state ---
	type InputMode = 'templates' | 'visual' | 'advanced';
	let activeMode: InputMode = 'templates';

	// --- Shared state ---
	let name = '';
	let nameError = '';
	let dataError = '';

	// --- Templates mode state ---
	let selectedTemplate: TemplateId = 'business_hours';

	// --- Visual Builder mode state ---
	let startDate = '';
	let endDate = '';
	let startHour = 8;
	let startMinute = 0;
	let endHour = 17;
	let endMinute = 0;
	let recurrence: RecurrenceType = 'weekly';
	let weeklyDays: boolean[] = [false, true, true, true, true, true, false];

	// --- Advanced mode state ---
	let icalData = '';

	// --- Existing visual builder state (day+time only, no date range) ---
	let selectedDays: boolean[] = [false, true, true, true, true, true, false];
	let builderStartHour = 8;
	let builderStartMinute = 0;
	let builderEndHour = 17;
	let builderEndMinute = 0;

	const DAY_LABELS = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat'];
	const ICAL_DAY_CODES = ['SU', 'MO', 'TU', 'WE', 'TH', 'FR', 'SA'];

	$: isEditing = calendar !== null;
	$: modalTitle = isEditing ? 'Edit Calendar' : 'Create Calendar';

	// Compute today's date for min validation
	$: todayStr = new Date().toISOString().split('T')[0];

	// Compute preview data based on active mode
	$: previewData = computePreviewData(activeMode, selectedTemplate, icalData, startDate, endDate, startHour, startMinute, endHour, endMinute, recurrence, weeklyDays);
	$: scheduleSummary = getScheduleSummary(previewData);
	$: activeStatus = getActiveStatus(previewData);

	// Compute visual builder validation error
	$: visualBuilderError = activeMode === 'visual' ? computeVisualBuilderError() : '';

	/**
	 * Reset form when modal opens.
	 */
	$: if (open) {
		resetForm();
	}

	function computePreviewData(
		mode: InputMode,
		_template: TemplateId,
		_ical: string,
		_sd: string, _ed: string,
		_sh: number, _sm: number,
		_eh: number, _em: number,
		_rec: RecurrenceType,
		_wd: boolean[]
	): string {
		if (mode === 'templates') {
			const template = CALENDAR_TEMPLATES.find((t) => t.id === _template);
			return template?.data || '';
		}
		if (mode === 'visual') {
			return buildDateRangeIcal({
				startDate: _sd,
				endDate: _ed,
				startHour: _sh,
				startMinute: _sm,
				endHour: _eh,
				endMinute: _em,
				recurrence: _rec,
				weeklyDays: _wd
			});
		}
		return _ical;
	}

	function computeVisualBuilderError(): string {
		if (!startDate && !endDate) return '';
		return validateDateRangeConfig({
			startDate,
			endDate,
			startHour,
			startMinute,
			endHour,
			endMinute,
			recurrence,
			weeklyDays
		}) || '';
	}

	function resetForm() {
		nameError = '';
		dataError = '';

		if (calendar) {
			name = calendar.name;
			icalData = calendar.data;

			// Try to parse into visual builder config
			const parsed = parseIcalToDateRangeConfig(calendar.data);
			if (parsed) {
				startDate = parsed.startDate;
				endDate = parsed.endDate;
				startHour = parsed.startHour;
				startMinute = parsed.startMinute;
				endHour = parsed.endHour;
				endMinute = parsed.endMinute;
				recurrence = parsed.recurrence;
				weeklyDays = [...parsed.weeklyDays];
				activeMode = 'visual';
			} else {
				activeMode = 'advanced';
			}

			selectedTemplate = 'custom';

			// Also parse for the old visual builder fields
			parseIcalToOldVisual(calendar.data);
		} else {
			name = '';
			icalData = '';
			selectedTemplate = 'business_hours';
			activeMode = 'templates';

			// Reset visual builder to defaults
			const today = new Date();
			const thirtyDaysOut = new Date(today);
			thirtyDaysOut.setDate(thirtyDaysOut.getDate() + 30);

			startDate = today.toISOString().split('T')[0];
			endDate = thirtyDaysOut.toISOString().split('T')[0];
			startHour = 8;
			startMinute = 0;
			endHour = 17;
			endMinute = 0;
			recurrence = 'weekly';
			weeklyDays = [false, true, true, true, true, true, false];

			// Old visual builder defaults
			selectedDays = [false, true, true, true, true, true, false];
			builderStartHour = 8;
			builderStartMinute = 0;
			builderEndHour = 17;
			builderEndMinute = 0;
		}
	}

	function parseIcalToOldVisual(data: string) {
		if (!data) return;
		try {
			const bydayMatch = data.match(/BYDAY=([A-Z,]+)/);
			if (bydayMatch) {
				const days = bydayMatch[1].split(',');
				selectedDays = ICAL_DAY_CODES.map((code) => days.includes(code));
			} else if (data.includes('FREQ=DAILY')) {
				selectedDays = [true, true, true, true, true, true, true];
			}
			const startMatch = data.match(/DTSTART[^:]*:(\d{4})(\d{2})(\d{2})T(\d{2})(\d{2})/);
			if (startMatch) {
				builderStartHour = parseInt(startMatch[4]);
				builderStartMinute = parseInt(startMatch[5]);
			}
			const endMatch = data.match(/DTEND[^:]*:(\d{4})(\d{2})(\d{2})T(\d{2})(\d{2})/);
			if (endMatch) {
				builderEndHour = parseInt(endMatch[4]);
				builderEndMinute = parseInt(endMatch[5]);
			}
		} catch {
			// If parsing fails, keep defaults
		}
	}

	function setMode(mode: InputMode) {
		activeMode = mode;
		dataError = '';

		if (mode === 'templates') {
			if (selectedTemplate === 'custom') {
				selectedTemplate = 'business_hours';
			}
		}
	}

	function applyTemplate(templateId: TemplateId) {
		selectedTemplate = templateId;
		const template = CALENDAR_TEMPLATES.find((t) => t.id === templateId);
		if (template && template.data) {
			icalData = template.data;
		} else if (templateId === 'custom') {
			// Switch to advanced mode for custom
			activeMode = 'advanced';
			if (!icalData) {
				icalData = '';
			}
		}
	}

	function handleSubmit() {
		nameError = '';
		dataError = '';

		if (!name.trim()) {
			nameError = 'Name is required';
			return;
		}

		let finalData = '';

		if (activeMode === 'templates') {
			const template = CALENDAR_TEMPLATES.find((t) => t.id === selectedTemplate);
			finalData = template?.data || '';
		} else if (activeMode === 'visual') {
			const config: DateRangeBuilderConfig = {
				startDate,
				endDate,
				startHour,
				startMinute,
				endHour,
				endMinute,
				recurrence,
				weeklyDays
			};
			const configError = validateDateRangeConfig(config);
			if (configError) {
				dataError = configError;
				return;
			}
			finalData = buildDateRangeIcal(config);
		} else {
			finalData = icalData;
		}

		const validationError = validateIcalData(finalData);
		if (validationError) {
			dataError = validationError;
			return;
		}

		dispatch('save', { name: name.trim(), data: finalData });
	}

	function handleClose() {
		open = false;
		dispatch('close');
	}

	function formatHourOption(h: number): string {
		const ampm = h >= 12 ? 'PM' : 'AM';
		const display = h % 12 || 12;
		return `${display} ${ampm}`;
	}

	function formatMinuteOption(m: number): string {
		return String(m).padStart(2, '0');
	}

	function toggleWeeklyDay(index: number) {
		weeklyDays[index] = !weeklyDays[index];
		weeklyDays = weeklyDays;
	}
</script>

<Modal {open} title={modalTitle} on:close={handleClose}>
	<form on:submit|preventDefault={handleSubmit} class="calendar-form">
		<!-- Name -->
		<Input
			label="Calendar Name"
			name="calendarName"
			placeholder="e.g., Business Hours"
			required
			bind:value={name}
			error={nameError}
		/>

		<!-- Mode Selector Tabs -->
		<div class="mode-tabs" role="tablist" aria-label="Calendar input mode">
			<button
				type="button"
				class="mode-tab"
				class:active={activeMode === 'templates'}
				on:click={() => setMode('templates')}
				role="tab"
				aria-selected={activeMode === 'templates'}
				aria-controls="panel-templates"
			>
				<svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2">
					<rect x="3" y="3" width="7" height="7"/>
					<rect x="14" y="3" width="7" height="7"/>
					<rect x="14" y="14" width="7" height="7"/>
					<rect x="3" y="14" width="7" height="7"/>
				</svg>
				Templates
			</button>
			<button
				type="button"
				class="mode-tab"
				class:active={activeMode === 'visual'}
				on:click={() => setMode('visual')}
				role="tab"
				aria-selected={activeMode === 'visual'}
				aria-controls="panel-visual"
			>
				<svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2">
					<rect x="3" y="4" width="18" height="18" rx="2" ry="2"/>
					<line x1="16" y1="2" x2="16" y2="6"/>
					<line x1="8" y1="2" x2="8" y2="6"/>
					<line x1="3" y1="10" x2="21" y2="10"/>
				</svg>
				Visual Builder
			</button>
			<button
				type="button"
				class="mode-tab"
				class:active={activeMode === 'advanced'}
				on:click={() => setMode('advanced')}
				role="tab"
				aria-selected={activeMode === 'advanced'}
				aria-controls="panel-advanced"
			>
				<svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2">
					<polyline points="16 18 22 12 16 6"/>
					<polyline points="8 6 2 12 8 18"/>
				</svg>
				Advanced
			</button>
		</div>

		<!-- Templates Panel -->
		{#if activeMode === 'templates'}
			<div id="panel-templates" role="tabpanel" aria-labelledby="tab-templates" class="mode-panel">
				<div class="template-grid">
					{#each CALENDAR_TEMPLATES as template (template.id)}
						<button
							type="button"
							class="template-card"
							class:selected={selectedTemplate === template.id}
							on:click={() => applyTemplate(template.id)}
						>
							<span class="template-icon">
								{#if template.id === 'business_hours'}
									<svg viewBox="0 0 24 24" width="20" height="20" fill="none" stroke="currentColor" stroke-width="2">
										<rect x="2" y="3" width="20" height="18" rx="2"/>
										<path d="M8 7v4h4"/>
									</svg>
								{:else if template.id === 'weekends'}
									<svg viewBox="0 0 24 24" width="20" height="20" fill="none" stroke="currentColor" stroke-width="2">
										<circle cx="12" cy="12" r="10"/>
										<path d="M8 14s1.5 2 4 2 4-2 4-2"/>
										<line x1="9" y1="9" x2="9.01" y2="9"/>
										<line x1="15" y1="9" x2="15.01" y2="9"/>
									</svg>
								{:else if template.id === 'weeknights'}
									<svg viewBox="0 0 24 24" width="20" height="20" fill="none" stroke="currentColor" stroke-width="2">
										<path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z"/>
									</svg>
								{:else if template.id === 'always'}
									<svg viewBox="0 0 24 24" width="20" height="20" fill="none" stroke="currentColor" stroke-width="2">
										<circle cx="12" cy="12" r="10"/>
										<polyline points="12 6 12 12 16 14"/>
									</svg>
								{:else}
									<svg viewBox="0 0 24 24" width="20" height="20" fill="none" stroke="currentColor" stroke-width="2">
										<path d="M11 4H4a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-7"/>
										<path d="M18.5 2.5a2.121 2.121 0 0 1 3 3L12 15l-4 1 1-4 9.5-9.5z"/>
									</svg>
								{/if}
							</span>
							<span class="template-name">{template.label}</span>
							<span class="template-desc">{template.description}</span>
						</button>
					{/each}
				</div>
			</div>
		{/if}

		<!-- Visual Builder Panel -->
		{#if activeMode === 'visual'}
			<div id="panel-visual" role="tabpanel" aria-labelledby="tab-visual" class="mode-panel">
				<div class="visual-builder">
					<!-- Date Range -->
					<div class="builder-section">
						<span class="builder-label">Date Range</span>
						<div class="date-range-row">
							<div class="date-input-group">
								<label for="start-date" class="date-label">Start</label>
								<input
									type="date"
									id="start-date"
									class="date-input"
									bind:value={startDate}
									min={todayStr}
									max={endDate || undefined}
									aria-label="Start date"
								/>
							</div>
							<span class="range-separator">to</span>
							<div class="date-input-group">
								<label for="end-date" class="date-label">End</label>
								<input
									type="date"
									id="end-date"
									class="date-input"
									bind:value={endDate}
									min={startDate || todayStr}
									aria-label="End date"
								/>
							</div>
						</div>
					</div>

					<!-- Time Range -->
					<div class="builder-section">
						<span class="builder-label">Time Range</span>
						<div class="time-range">
							<div class="time-picker">
								<label for="visual-start-hour" class="sr-only">Start hour</label>
								<select id="visual-start-hour" bind:value={startHour} class="time-select">
									{#each Array(24) as _, h}
										<option value={h}>{formatHourOption(h)}</option>
									{/each}
								</select>
								<span class="time-sep">:</span>
								<label for="visual-start-min" class="sr-only">Start minute</label>
								<select id="visual-start-min" bind:value={startMinute} class="time-select time-select-min">
									{#each Array.from({length: 60}, (_, i) => i) as m}
										<option value={m}>{formatMinuteOption(m)}</option>
									{/each}
								</select>
							</div>
							<span class="time-to">to</span>
							<div class="time-picker">
								<label for="visual-end-hour" class="sr-only">End hour</label>
								<select id="visual-end-hour" bind:value={endHour} class="time-select">
									{#each Array(24) as _, h}
										<option value={h}>{formatHourOption(h)}</option>
									{/each}
								</select>
								<span class="time-sep">:</span>
								<label for="visual-end-min" class="sr-only">End minute</label>
								<select id="visual-end-min" bind:value={endMinute} class="time-select time-select-min">
									{#each Array.from({length: 60}, (_, i) => i) as m}
										<option value={m}>{formatMinuteOption(m)}</option>
									{/each}
								</select>
							</div>
						</div>
					</div>

					<!-- Recurrence -->
					<div class="builder-section">
						<span class="builder-label">Recurrence</span>
						<div class="recurrence-selector">
							<button
								type="button"
								class="recurrence-btn"
								class:active={recurrence === 'none'}
								on:click={() => (recurrence = 'none')}
								aria-pressed={recurrence === 'none'}
							>
								None
							</button>
							<button
								type="button"
								class="recurrence-btn"
								class:active={recurrence === 'daily'}
								on:click={() => (recurrence = 'daily')}
								aria-pressed={recurrence === 'daily'}
							>
								Daily
							</button>
							<button
								type="button"
								class="recurrence-btn"
								class:active={recurrence === 'weekly'}
								on:click={() => (recurrence = 'weekly')}
								aria-pressed={recurrence === 'weekly'}
							>
								Weekly
							</button>
						</div>
					</div>

					<!-- Weekly Day Selector -->
					{#if recurrence === 'weekly'}
						<div class="builder-section">
							<span class="builder-label">Active Days</span>
							<div class="day-selector">
								{#each DAY_LABELS as day, i}
									<button
										type="button"
										class="day-btn"
										class:active={weeklyDays[i]}
										on:click={() => toggleWeeklyDay(i)}
										aria-label="{day} {weeklyDays[i] ? 'active' : 'inactive'}"
										aria-pressed={weeklyDays[i]}
									>
										{day}
									</button>
								{/each}
							</div>
						</div>
					{/if}

					<!-- Visual builder validation error -->
					{#if visualBuilderError}
						<div class="builder-error" role="alert">
							<svg viewBox="0 0 24 24" width="14" height="14" fill="none" stroke="currentColor" stroke-width="2">
								<circle cx="12" cy="12" r="10"/>
								<line x1="12" y1="8" x2="12" y2="12"/>
								<line x1="12" y1="16" x2="12.01" y2="16"/>
							</svg>
							{visualBuilderError}
						</div>
					{/if}
				</div>
			</div>
		{/if}

		<!-- Advanced Panel -->
		{#if activeMode === 'advanced'}
			<div id="panel-advanced" role="tabpanel" aria-labelledby="tab-advanced" class="mode-panel">
				<div class="form-group">
					<label for="ical-data" class="form-label">
						iCalendar Data (RFC 5545)
					</label>
					<textarea
						id="ical-data"
						bind:value={icalData}
						rows="10"
						class="ical-textarea"
						class:has-error={!!dataError}
						placeholder="BEGIN:VCALENDAR&#10;VERSION:2.0&#10;BEGIN:VEVENT&#10;SUMMARY:My Schedule&#10;DTSTART:20240101T080000&#10;DTEND:20240101T170000&#10;RRULE:FREQ=WEEKLY;BYDAY=MO,TU,WE,TH,FR&#10;END:VEVENT&#10;END:VCALENDAR"
						aria-invalid={dataError ? 'true' : undefined}
						aria-describedby={dataError ? 'ical-error' : undefined}
					></textarea>
					{#if dataError}
						<span id="ical-error" class="error-text" role="alert">{dataError}</span>
					{/if}
				</div>
			</div>
		{/if}

		<!-- Data error for non-advanced modes -->
		{#if dataError && activeMode !== 'advanced'}
			<div class="error-text" role="alert">{dataError}</div>
		{/if}

		<!-- Preview -->
		{#if previewData}
			<div class="preview-section">
				<span class="form-label">Schedule Preview</span>
				<div class="preview-card">
					<div class="preview-summary">
						<svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2">
							<rect x="3" y="4" width="18" height="18" rx="2" ry="2"/>
							<line x1="16" y1="2" x2="16" y2="6"/>
							<line x1="8" y1="2" x2="8" y2="6"/>
							<line x1="3" y1="10" x2="21" y2="10"/>
						</svg>
						<span>{scheduleSummary}</span>
					</div>
					<div class="preview-status" class:active={activeStatus.active}>
						<span class="status-dot"></span>
						<span>{activeStatus.label}</span>
					</div>
				</div>
			</div>
		{/if}
	</form>

	<svelte:fragment slot="footer">
		<Button variant="secondary" on:click={handleClose}>Cancel</Button>
		<Button loading={saving} on:click={handleSubmit}>
			{isEditing ? 'Update' : 'Create'}
		</Button>
	</svelte:fragment>
</Modal>

<style>
	.calendar-form {
		display: flex;
		flex-direction: column;
		gap: var(--space-4);
	}

	.form-group {
		display: flex;
		flex-direction: column;
		gap: var(--space-2);
	}

	.form-label {
		font-size: var(--text-sm);
		font-weight: var(--font-medium);
		color: var(--text-primary);
	}

	/* Mode Tabs */
	.mode-tabs {
		display: flex;
		gap: var(--space-1);
		background-color: var(--bg-secondary);
		border-radius: var(--radius-md);
		padding: var(--space-1);
		border: 1px solid var(--border-color);
	}

	.mode-tab {
		flex: 1;
		display: flex;
		align-items: center;
		justify-content: center;
		gap: var(--space-1);
		padding: var(--space-2) var(--space-3);
		background: none;
		border: 1px solid transparent;
		border-radius: var(--radius-sm);
		color: var(--text-secondary);
		font-size: var(--text-sm);
		font-weight: var(--font-medium);
		cursor: pointer;
		transition: all var(--transition-fast);
		white-space: nowrap;
	}

	.mode-tab:hover {
		color: var(--text-primary);
		background-color: var(--bg-hover);
	}

	.mode-tab.active {
		background-color: var(--bg-primary);
		color: var(--accent-primary);
		border-color: var(--border-color);
		box-shadow: var(--shadow-sm);
	}

	.mode-tab svg {
		flex-shrink: 0;
	}

	.mode-panel {
		animation: fadeIn 150ms ease;
	}

	@keyframes fadeIn {
		from { opacity: 0; transform: translateY(4px); }
		to { opacity: 1; transform: translateY(0); }
	}

	/* Template grid */
	.template-grid {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(140px, 1fr));
		gap: var(--space-2);
	}

	.template-card {
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: var(--space-1);
		padding: var(--space-3);
		background-color: var(--bg-secondary);
		border: 2px solid var(--border-color);
		border-radius: var(--radius-md);
		cursor: pointer;
		transition: all var(--transition-fast);
		text-align: center;
	}

	.template-card:hover {
		border-color: var(--accent-primary);
		background-color: var(--bg-hover);
	}

	.template-card.selected {
		border-color: var(--accent-primary);
		background-color: rgba(0, 212, 255, 0.08);
	}

	.template-icon {
		display: flex;
		align-items: center;
		justify-content: center;
		width: 32px;
		height: 32px;
		color: var(--text-secondary);
	}

	.template-card.selected .template-icon {
		color: var(--accent-primary);
	}

	.template-name {
		font-size: var(--text-sm);
		font-weight: var(--font-medium);
		color: var(--text-primary);
	}

	.template-desc {
		font-size: var(--text-xs);
		color: var(--text-tertiary);
		line-height: 1.3;
	}

	/* Visual builder */
	.visual-builder {
		display: flex;
		flex-direction: column;
		gap: var(--space-4);
		padding: var(--space-4);
		background-color: var(--bg-secondary);
		border: 1px solid var(--border-color);
		border-radius: var(--radius-md);
	}

	.builder-section {
		display: flex;
		flex-direction: column;
		gap: var(--space-2);
	}

	.builder-label {
		font-size: var(--text-sm);
		font-weight: var(--font-medium);
		color: var(--text-secondary);
	}

	/* Date range */
	.date-range-row {
		display: flex;
		align-items: flex-end;
		gap: var(--space-3);
		flex-wrap: wrap;
	}

	.date-input-group {
		display: flex;
		flex-direction: column;
		gap: var(--space-1);
		flex: 1;
		min-width: 140px;
	}

	.date-label {
		font-size: var(--text-xs);
		color: var(--text-tertiary);
		text-transform: uppercase;
		letter-spacing: 0.05em;
	}

	.date-input {
		padding: var(--space-2) var(--space-3);
		background-color: var(--bg-tertiary);
		border: 1px solid var(--border-color);
		border-radius: var(--radius-md);
		color: var(--text-primary);
		font-size: var(--text-sm);
		font-family: inherit;
		width: 100%;
	}

	.date-input:focus {
		outline: none;
		border-color: var(--accent-primary);
		box-shadow: 0 0 0 3px rgba(0, 212, 255, 0.1);
	}

	/* Color scheme for date input to match theme */
	.date-input::-webkit-calendar-picker-indicator {
		filter: invert(0.7);
		cursor: pointer;
	}

	.range-separator {
		color: var(--text-secondary);
		font-size: var(--text-sm);
		padding-bottom: var(--space-2);
	}

	/* Recurrence selector */
	.recurrence-selector {
		display: flex;
		gap: var(--space-1);
		background-color: var(--bg-tertiary);
		border-radius: var(--radius-md);
		padding: var(--space-1);
	}

	.recurrence-btn {
		flex: 1;
		padding: var(--space-2) var(--space-3);
		background: none;
		border: 1px solid transparent;
		border-radius: var(--radius-sm);
		color: var(--text-secondary);
		font-size: var(--text-sm);
		font-weight: var(--font-medium);
		cursor: pointer;
		transition: all var(--transition-fast);
	}

	.recurrence-btn:hover {
		color: var(--text-primary);
	}

	.recurrence-btn.active {
		background-color: var(--bg-primary);
		color: var(--accent-primary);
		border-color: var(--border-color);
		box-shadow: var(--shadow-sm);
	}

	/* Day selector */
	.day-selector {
		display: flex;
		gap: var(--space-1);
		flex-wrap: wrap;
	}

	.day-btn {
		min-width: 44px;
		padding: var(--space-2) var(--space-2);
		background-color: var(--bg-tertiary);
		border: 2px solid transparent;
		border-radius: var(--radius-md);
		color: var(--text-secondary);
		font-size: var(--text-sm);
		font-weight: var(--font-medium);
		cursor: pointer;
		transition: all var(--transition-fast);
	}

	.day-btn:hover {
		background-color: var(--bg-hover);
	}

	.day-btn.active {
		background-color: rgba(0, 212, 255, 0.15);
		border-color: var(--accent-primary);
		color: var(--accent-primary);
	}

	/* Builder error */
	.builder-error {
		display: flex;
		align-items: center;
		gap: var(--space-2);
		padding: var(--space-2) var(--space-3);
		background-color: rgba(255, 68, 68, 0.1);
		border: 1px solid rgba(255, 68, 68, 0.3);
		border-radius: var(--radius-md);
		color: var(--error);
		font-size: var(--text-sm);
	}

	.builder-error svg {
		flex-shrink: 0;
	}

	/* Time range */
	.time-range {
		display: flex;
		align-items: center;
		gap: var(--space-3);
		flex-wrap: wrap;
	}

	.time-picker {
		display: flex;
		align-items: center;
		gap: var(--space-1);
	}

	.time-select {
		padding: var(--space-2) var(--space-3);
		background-color: var(--bg-tertiary);
		border: 1px solid var(--border-color);
		border-radius: var(--radius-md);
		color: var(--text-primary);
		font-size: var(--text-sm);
	}

	.time-select:focus {
		outline: none;
		border-color: var(--accent-primary);
		box-shadow: 0 0 0 3px rgba(0, 212, 255, 0.1);
	}

	.time-select-min {
		min-width: 60px;
	}

	.time-sep {
		color: var(--text-secondary);
		font-weight: var(--font-bold);
	}

	.time-to {
		color: var(--text-secondary);
		font-size: var(--text-sm);
	}

	/* iCal textarea */
	.ical-textarea {
		width: 100%;
		padding: var(--space-3);
		background-color: var(--bg-secondary);
		border: 1px solid var(--border-color);
		border-radius: var(--radius-md);
		color: var(--text-primary);
		font-family: 'Courier New', monospace;
		font-size: var(--text-xs);
		resize: vertical;
		line-height: 1.5;
		box-sizing: border-box;
	}

	.ical-textarea:focus {
		outline: none;
		border-color: var(--accent-primary);
		box-shadow: 0 0 0 3px rgba(0, 212, 255, 0.1);
	}

	.ical-textarea.has-error {
		border-color: var(--error);
	}

	.error-text {
		font-size: var(--text-sm);
		color: var(--error);
	}

	/* Preview */
	.preview-section {
		display: flex;
		flex-direction: column;
		gap: var(--space-2);
	}

	.preview-card {
		display: flex;
		flex-direction: column;
		gap: var(--space-2);
		padding: var(--space-3);
		background-color: var(--bg-secondary);
		border: 1px solid var(--border-color);
		border-radius: var(--radius-md);
	}

	.preview-summary {
		display: flex;
		align-items: center;
		gap: var(--space-2);
		font-size: var(--text-sm);
		color: var(--text-primary);
	}

	.preview-summary svg {
		color: var(--text-secondary);
		flex-shrink: 0;
	}

	.preview-status {
		display: flex;
		align-items: center;
		gap: var(--space-2);
		font-size: var(--text-sm);
		color: var(--text-secondary);
	}

	.status-dot {
		width: 8px;
		height: 8px;
		border-radius: 50%;
		background-color: var(--status-offline);
	}

	.preview-status.active .status-dot {
		background-color: var(--status-online);
	}

	.preview-status.active {
		color: var(--status-online);
	}

	/* Accessibility */
	.sr-only {
		position: absolute;
		width: 1px;
		height: 1px;
		padding: 0;
		margin: -1px;
		overflow: hidden;
		clip: rect(0, 0, 0, 0);
		border: 0;
	}

	@media (max-width: 480px) {
		.template-grid {
			grid-template-columns: repeat(2, 1fr);
		}

		.time-range {
			flex-direction: column;
			align-items: flex-start;
		}

		.date-range-row {
			flex-direction: column;
		}

		.range-separator {
			padding-bottom: 0;
			align-self: center;
		}

		.mode-tab {
			padding: var(--space-2) var(--space-1);
			font-size: var(--text-xs);
		}

		.mode-tab svg {
			display: none;
		}
	}
</style>
