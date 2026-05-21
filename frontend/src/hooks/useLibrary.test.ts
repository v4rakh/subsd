import { apiFetch } from '../api';
import type { Artist, Album, Playlist } from '../types';
import { useLibrary } from './useLibrary';
import { renderHook, act } from '@testing-library/react';
import { describe, it, expect, vi, beforeEach } from 'vitest';

// Mock the apiFetch module so no real HTTP calls are made.
vi.mock('../api', () => ({
	apiFetch: vi.fn()
}));

const mockFetch = vi.mocked(apiFetch);

function makeResponse(data: unknown): Response {
	return {
		json: () => Promise.resolve(data),
		status: 200,
		ok: true
	} as unknown as Response;
}

beforeEach(() => {
	mockFetch.mockReset();
});

describe('useLibrary — loadArtists', () => {
	it('fetches artists and updates state', async () => {
		const artists: Artist[] = [
			{ id: 'a1', name: 'Artist One', albumCount: 2, coverArt: '' },
			{ id: 'a2', name: 'Artist Two', albumCount: 1, coverArt: '' }
		];
		mockFetch.mockResolvedValueOnce(makeResponse(artists));

		const { result } = renderHook(() => useLibrary());
		expect(result.current.artists).toEqual([]);

		await act(async () => {
			await result.current.loadArtists();
		});

		expect(mockFetch).toHaveBeenCalledWith('/api/v1/artists');
		expect(result.current.artists).toEqual(artists);
	});

	it('sets loading flag during fetch', async () => {
		let resolveArtists!: (v: Response) => void;
		mockFetch.mockReturnValueOnce(
			new Promise<Response>((res) => {
				resolveArtists = res;
			})
		);

		const { result } = renderHook(() => useLibrary());

		act(() => {
			void result.current.loadArtists();
		});
		const loadingDuring = result.current.loading.artists;

		await act(async () => {
			resolveArtists(makeResponse([]));
		});

		expect(loadingDuring).toBe(true);
		expect(result.current.loading.artists).toBe(false);
	});

	it('clears loading flag even on error', async () => {
		mockFetch.mockRejectedValueOnce(new Error('network error'));

		const { result } = renderHook(() => useLibrary());
		await act(async () => {
			try {
				await result.current.loadArtists();
			} catch {
				// expected
			}
		});

		expect(result.current.loading.artists).toBe(false);
	});
});

describe('useLibrary — selectArtist', () => {
	it('fetches artist albums and sets state', async () => {
		const artist: Artist = { id: 'a1', name: 'Artist One', albumCount: 2, coverArt: '' };
		const artistWithAlbums: Artist = {
			...artist,
			album: [
				{ id: 'alb1', name: 'Album 1', artist: 'Artist One', artistId: 'a1', coverArt: '', songCount: 10 },
				{ id: 'alb2', name: 'Album 2', artist: 'Artist One', artistId: 'a1', coverArt: '', songCount: 5 }
			]
		};
		mockFetch.mockResolvedValueOnce(makeResponse(artistWithAlbums));

		const { result } = renderHook(() => useLibrary());
		await act(async () => {
			await result.current.selectArtist(artist);
		});

		expect(mockFetch).toHaveBeenCalledWith('/api/v1/artist/a1');
		expect(result.current.selectedArtist).toEqual(artist);
		expect(result.current.albums).toHaveLength(2);
		expect(result.current.albums[0].id).toBe('alb1');
	});

	it('clears previous album and song selection when selecting new artist', async () => {
		const artist1: Artist = { id: 'a1', name: 'A1', albumCount: 1, coverArt: '' };
		const artist2: Artist = { id: 'a2', name: 'A2', albumCount: 1, coverArt: '' };

		mockFetch
			.mockResolvedValueOnce(
				makeResponse({
					...artist1,
					album: [{ id: 'alb1', name: 'Alb', artist: 'A1', artistId: 'a1', coverArt: '', songCount: 1 }]
				})
			)
			.mockResolvedValueOnce(makeResponse({ ...artist2, album: [] }));

		const { result } = renderHook(() => useLibrary());

		await act(async () => {
			await result.current.selectArtist(artist1);
		});
		expect(result.current.albums).toHaveLength(1);

		await act(async () => {
			await result.current.selectArtist(artist2);
		});

		expect(result.current.selectedAlbum).toBeNull();
		expect(result.current.songs).toEqual([]);
	});
});

describe('useLibrary — selectAlbum', () => {
	it('fetches album tracks and sets state', async () => {
		const album: Album = { id: 'alb1', name: 'Album', artist: 'A', artistId: 'a1', coverArt: '', songCount: 2 };
		const albumWithSongs = {
			...album,
			song: [
				{
					id: 's1',
					title: 'Track 1',
					artist: 'A',
					album: 'Album',
					albumId: 'alb1',
					artistId: 'a1',
					duration: 180,
					track: 1,
					coverArt: '',
					contentType: 'audio/flac'
				},
				{
					id: 's2',
					title: 'Track 2',
					artist: 'A',
					album: 'Album',
					albumId: 'alb1',
					artistId: 'a1',
					duration: 200,
					track: 2,
					coverArt: '',
					contentType: 'audio/flac'
				}
			]
		};
		mockFetch.mockResolvedValueOnce(makeResponse(albumWithSongs));

		const { result } = renderHook(() => useLibrary());
		await act(async () => {
			await result.current.selectAlbum(album);
		});

		expect(mockFetch).toHaveBeenCalledWith('/api/v1/album/alb1');
		expect(result.current.selectedAlbum).toEqual(album);
		expect(result.current.songs).toHaveLength(2);
		expect(result.current.songs[0].id).toBe('s1');
	});
});

describe('useLibrary — loadPlaylists', () => {
	it('fetches playlists and updates state', async () => {
		const playlists: Playlist[] = [
			{ id: 'p1', name: 'PL1', songCount: 5, coverArt: '' },
			{ id: 'p2', name: 'PL2', songCount: 3, coverArt: '' }
		];
		mockFetch.mockResolvedValueOnce(makeResponse(playlists));

		const { result } = renderHook(() => useLibrary());
		await act(async () => {
			await result.current.loadPlaylists();
		});

		expect(mockFetch).toHaveBeenCalledWith('/api/v1/playlists');
		expect(result.current.playlists).toEqual(playlists);
	});
});

describe('useLibrary — selectPlaylist', () => {
	it('fetches playlist songs and sets state', async () => {
		const playlist: Playlist = { id: 'p1', name: 'PL1', songCount: 2, coverArt: '' };
		const playlistWithEntries = {
			...playlist,
			entry: [
				{
					id: 's1',
					title: 'T1',
					artist: 'A',
					album: 'B',
					albumId: 'alb1',
					artistId: 'a1',
					duration: 100,
					track: 1,
					coverArt: '',
					contentType: 'audio/mp3'
				}
			]
		};
		mockFetch.mockResolvedValueOnce(makeResponse(playlistWithEntries));

		const { result } = renderHook(() => useLibrary());
		await act(async () => {
			await result.current.selectPlaylist(playlist);
		});

		expect(mockFetch).toHaveBeenCalledWith('/api/v1/playlist/p1');
		expect(result.current.selectedPlaylist).toEqual(playlist);
		expect(result.current.playlistSongs).toHaveLength(1);
	});
});

describe('useLibrary — clearSelectedPlaylist', () => {
	it('resets selected playlist and songs', async () => {
		const playlist: Playlist = { id: 'p1', name: 'PL1', songCount: 1, coverArt: '' };
		mockFetch.mockResolvedValueOnce(
			makeResponse({
				...playlist,
				entry: [
					{
						id: 's1',
						title: 'T',
						artist: 'A',
						album: 'B',
						albumId: 'a',
						artistId: 'a',
						duration: 100,
						track: 1,
						coverArt: '',
						contentType: ''
					}
				]
			})
		);

		const { result } = renderHook(() => useLibrary());
		await act(async () => {
			await result.current.selectPlaylist(playlist);
		});
		expect(result.current.selectedPlaylist).not.toBeNull();
		expect(result.current.playlistSongs).toHaveLength(1);

		act(() => {
			result.current.clearSelectedPlaylist();
		});

		expect(result.current.selectedPlaylist).toBeNull();
		expect(result.current.playlistSongs).toEqual([]);
	});
});
