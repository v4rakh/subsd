export const AUTH_FAILURE_EVENT = 'subsd:authfailure';

export async function apiFetch(input: string, init?: RequestInit): Promise<Response> {
	const r = await fetch(input, init);
	if (r.status === 401) {
		window.dispatchEvent(new Event(AUTH_FAILURE_EVENT));
		throw new Error('unauthorized');
	}
	return r;
}
