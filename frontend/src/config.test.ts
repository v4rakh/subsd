import { afterEach, describe, expect, it, vi } from 'vitest';

// Each test re-imports the module via vi.resetModules() so the module-level
// `apiUrl` variable starts at its zero value ('') for every test case.
afterEach(() => {
	vi.resetModules();
	vi.unstubAllGlobals();
});

describe('getApiUrl', () => {
	it('returns empty string before loadConfig is called', async () => {
		const { getApiUrl } = await import('./config');
		expect(getApiUrl()).toBe('');
	});
});

describe('loadConfig', () => {
	it('sets apiUrl from a successful response', async () => {
		vi.stubGlobal(
			'fetch',
			vi.fn().mockResolvedValue({
				ok: true,
				json: () => Promise.resolve({ url: 'https://api.example.com' }),
			})
		);
		const { loadConfig, getApiUrl } = await import('./config');
		await loadConfig();
		expect(getApiUrl()).toBe('https://api.example.com');
	});

	it('defaults to empty string when apiUrl field is absent', async () => {
		vi.stubGlobal(
			'fetch',
			vi.fn().mockResolvedValue({
				ok: true,
				json: () => Promise.resolve({}),
			})
		);
		const { loadConfig, getApiUrl } = await import('./config');
		await loadConfig();
		expect(getApiUrl()).toBe('');
	});

	it('defaults to empty string on non-ok response', async () => {
		vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: false }));
		const { loadConfig, getApiUrl } = await import('./config');
		await loadConfig();
		expect(getApiUrl()).toBe('');
	});

	it('defaults to empty string on network failure', async () => {
		vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new Error('offline')));
		const { loadConfig, getApiUrl } = await import('./config');
		await loadConfig();
		expect(getApiUrl()).toBe('');
	});
});
