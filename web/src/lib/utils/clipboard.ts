/** Copies text to the clipboard, falling back to selecting `input` and execCommand. */
export async function copyText(text: string, input?: HTMLInputElement | null): Promise<boolean> {
	try {
		await navigator.clipboard.writeText(text);
		return true;
	} catch {
		if (!input) return false;
		input.select();
		document.execCommand('copy');
		return true;
	}
}
