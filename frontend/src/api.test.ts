import { afterEach, describe, expect, it, vi } from 'vitest';

// Each test re-imports api.ts fresh (via vi.resetModules) so that the config
// module mock is isolated per test.
afterEach(() => {
	vi.resetModules();
	vi.unstubAllGlobals();
});

describe('apiFetch', () => {
	it('prepends apiUrl to the path when non-empty', async () => {
		vi.doMock('./config', () => ({ getApiUrl: () => 'https://api.example.com' }));
		const mockFetch = vi.fn().mockResolvedValue({ status: 200, ok: true });
		vi.stubGlobal('fetch', mockFetch);

		const { apiFetch } = await import('./api');
		await apiFetch('/api/state');

		expect(mockFetch).toHaveBeenCalledWith('https://api.example.com/api/state', { credentials: 'include' });
	});

	it('uses the path as-is when apiUrl is empty', async () => {
		vi.doMock('./config', () => ({ getApiUrl: () => '' }));
		const mockFetch = vi.fn().mockResolvedValue({ status: 200, ok: true });
		vi.stubGlobal('fetch', mockFetch);

		const { apiFetch } = await import('./api');
		await apiFetch('/api/state');

		expect(mockFetch).toHaveBeenCalledWith('/api/state', { credentials: 'include' });
	});

	it('dispatches AUTH_FAILURE_EVENT and throws on 401', async () => {
		vi.doMock('./config', () => ({ getApiUrl: () => '' }));
		vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ status: 401, ok: false }));
		const dispatchSpy = vi.spyOn(window, 'dispatchEvent');

		const { apiFetch, AUTH_FAILURE_EVENT } = await import('./api');
		await expect(apiFetch('/api/state')).rejects.toThrow('unauthorized');
		expect(dispatchSpy).toHaveBeenCalledWith(
			expect.objectContaining({ type: AUTH_FAILURE_EVENT })
		);
	});

	it('returns the response on success', async () => {
		vi.doMock('./config', () => ({ getApiUrl: () => '' }));
		const response = { status: 200, ok: true } as Response;
		vi.stubGlobal('fetch', vi.fn().mockResolvedValue(response));

		const { apiFetch } = await import('./api');
		const result = await apiFetch('/api/artists');

		expect(result).toBe(response);
	});
});
