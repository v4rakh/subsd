import { getApiUrl } from './config';

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
