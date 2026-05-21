let apiUrl = '';

export async function loadConfig(): Promise<void> {
	try {
		const r = await fetch('/config.json');
		if (r.ok) {
			const cfg = (await r.json()) as { url?: string };
			apiUrl = cfg.url ?? '';
		}
	} catch {
		// network error — fall back to same-origin
	}
}

export function getApiUrl(): string {
	return apiUrl;
}
