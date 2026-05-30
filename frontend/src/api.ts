import { getApiUrl } from './config';
import type { Lyrics, Settings } from './types';

export const AUTH_FAILURE_EVENT = 'subsd:authfailure';

export function apiUrl(path: string): string {
	return getApiUrl() + path;
}

export async function apiFetch(input: string, init?: RequestInit): Promise<Response> {
	const r = await fetch(getApiUrl() + input, { credentials: 'include', ...init });
	if (r.status === 401) {
		window.dispatchEvent(new Event(AUTH_FAILURE_EVENT));
		throw new Error('unauthorized');
	}
	return r;
}

export async function getLyrics(songId: string): Promise<Lyrics | null> {
	const r = await apiFetch(`/api/v1/lyrics/${encodeURIComponent(songId)}`);
	if (r.status === 404) return null;
	if (!r.ok) throw new Error(`lyrics fetch failed: ${r.status}`);
	return r.json();
}

export async function getSettings(): Promise<Settings> {
	const r = await apiFetch('/api/v1/settings');
	if (!r.ok) throw new Error(`settings fetch failed: ${r.status}`);
	return r.json();
}
