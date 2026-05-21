import { apiFetch } from '../api';
import type { PlayerState } from '../types';
import { usePlayer } from './usePlayer';
import { renderHook, act } from '@testing-library/react';
import { describe, it, expect, vi, beforeEach } from 'vitest';

// Mock useWebSocket so tests don't need a live WebSocket server.
// The mock exposes onMessage so tests can inject state updates.
let capturedOnMessage: ((data: PlayerState) => void) | null = null;
let mockConnected = true;

vi.mock('./useWebSocket', () => ({
	useWebSocket: (onMessage: (data: PlayerState) => void) => {
		capturedOnMessage = onMessage;
		return mockConnected;
	}
}));

// Mock apiFetch so controls don't make real HTTP calls.
vi.mock('../api', () => ({
	apiFetch: vi.fn().mockResolvedValue({ status: 200, ok: true })
}));

const mockFetch = vi.mocked(apiFetch);

beforeEach(() => {
	mockFetch.mockReset();
	mockFetch.mockResolvedValue({ status: 200, ok: true } as Response);
	capturedOnMessage = null;
	mockConnected = true;
});

describe('usePlayer — initial state', () => {
	it('starts with default state', () => {
		const { result } = renderHook(() => usePlayer());
		const { playerState } = result.current;
		expect(playerState.playing).toBe(false);
		expect(playerState.currentIdx).toBe(-1);
		expect(playerState.queue).toEqual([]);
		expect(playerState.volume).toBe(100);
		expect(playerState.shuffle).toBe(false);
		expect(playerState.repeat).toBe(false);
	});

	it('reports connected status from useWebSocket', () => {
		const { result } = renderHook(() => usePlayer());
		expect(result.current.connected).toBe(true);
	});
});

describe('usePlayer — WebSocket state sync', () => {
	it('updates playerState on WebSocket message', () => {
		const { result } = renderHook(() => usePlayer());

		const newState: PlayerState = {
			playing: true,
			currentIdx: 2,
			queue: [{ id: 't1', title: 'T', artist: 'A', album: 'B', duration: 100, coverArt: '', streamUrl: '' }],
			position: 30,
			duration: 180,
			volume: 80,
			shuffle: true,
			repeat: false
		};

		act(() => {
			capturedOnMessage?.(newState);
		});

		expect(result.current.playerState.playing).toBe(true);
		expect(result.current.playerState.currentIdx).toBe(2);
		expect(result.current.playerState.volume).toBe(80);
		expect(result.current.playerState.shuffle).toBe(true);
	});

	it('normalises null queue to empty array', () => {
		const { result } = renderHook(() => usePlayer());

		act(() => {
			// Simulate a message where queue is omitted/null (server may send this).
			capturedOnMessage?.({
				playing: false,
				currentIdx: -1,
				queue: null as unknown as [],
				position: 0,
				duration: 0,
				volume: 100,
				shuffle: false,
				repeat: false
			});
		});

		expect(result.current.playerState.queue).toEqual([]);
	});
});

describe('usePlayer — controls', () => {
	it('playPause calls POST /api/playpause', async () => {
		const { result } = renderHook(() => usePlayer());
		await act(async () => {
			result.current.controls.playPause();
		});
		expect(mockFetch).toHaveBeenCalledWith('/api/v1/playpause', expect.objectContaining({ method: 'POST' }));
	});

	it('next calls POST /api/next', async () => {
		const { result } = renderHook(() => usePlayer());
		await act(async () => {
			result.current.controls.next();
		});
		expect(mockFetch).toHaveBeenCalledWith('/api/v1/next', expect.objectContaining({ method: 'POST' }));
	});

	it('prev calls POST /api/prev', async () => {
		const { result } = renderHook(() => usePlayer());
		await act(async () => {
			result.current.controls.prev();
		});
		expect(mockFetch).toHaveBeenCalledWith('/api/v1/prev', expect.objectContaining({ method: 'POST' }));
	});

	it('shuffle calls POST /api/shuffle', async () => {
		const { result } = renderHook(() => usePlayer());
		await act(async () => {
			result.current.controls.shuffle();
		});
		expect(mockFetch).toHaveBeenCalledWith('/api/v1/shuffle', expect.objectContaining({ method: 'POST' }));
	});

	it('repeat calls POST /api/repeat', async () => {
		const { result } = renderHook(() => usePlayer());
		await act(async () => {
			result.current.controls.repeat();
		});
		expect(mockFetch).toHaveBeenCalledWith('/api/v1/repeat', expect.objectContaining({ method: 'POST' }));
	});

	it('setVolume calls POST /api/volume with body', async () => {
		const { result } = renderHook(() => usePlayer());
		await act(async () => {
			result.current.controls.setVolume(75);
		});
		expect(mockFetch).toHaveBeenCalledWith(
			'/api/v1/volume',
			expect.objectContaining({ method: 'POST', body: JSON.stringify({ volume: 75 }) })
		);
	});

	it('clearQueue calls DELETE /api/queue', async () => {
		const { result } = renderHook(() => usePlayer());
		await act(async () => {
			result.current.controls.clearQueue();
		});
		expect(mockFetch).toHaveBeenCalledWith('/api/v1/queue', expect.objectContaining({ method: 'DELETE' }));
	});

	it('removeTrack calls DELETE /api/queue/{idx}', async () => {
		const { result } = renderHook(() => usePlayer());
		await act(async () => {
			result.current.controls.removeTrack(3);
		});
		expect(mockFetch).toHaveBeenCalledWith('/api/v1/queue/3', expect.objectContaining({ method: 'DELETE' }));
	});

	it('moveTrack calls POST /api/queue/move with from/to', async () => {
		const { result } = renderHook(() => usePlayer());
		await act(async () => {
			result.current.controls.moveTrack(1, 4);
		});
		expect(mockFetch).toHaveBeenCalledWith(
			'/api/v1/queue/move',
			expect.objectContaining({ body: JSON.stringify({ from: 1, to: 4 }) })
		);
	});

	it('jumpTo calls POST /api/queue/jump/{idx}', async () => {
		const { result } = renderHook(() => usePlayer());
		await act(async () => {
			result.current.controls.jumpTo(2);
		});
		expect(mockFetch).toHaveBeenCalledWith('/api/v1/queue/jump/2', expect.objectContaining({ method: 'POST' }));
	});

	it('playSong calls POST /api/play/song/{id}', async () => {
		const { result } = renderHook(() => usePlayer());
		await act(async () => {
			result.current.controls.playSong('song42');
		});
		expect(mockFetch).toHaveBeenCalledWith('/api/v1/play/song/song42', expect.objectContaining({ method: 'POST' }));
	});

	it('playAlbum calls POST /api/play/album/{id}', async () => {
		const { result } = renderHook(() => usePlayer());
		await act(async () => {
			result.current.controls.playAlbum('alb7');
		});
		expect(mockFetch).toHaveBeenCalledWith('/api/v1/play/album/alb7', expect.objectContaining({ method: 'POST' }));
	});

	it('enqueueSong calls POST /api/queue/song/{id}', async () => {
		const { result } = renderHook(() => usePlayer());
		await act(async () => {
			result.current.controls.enqueueSong('s1');
		});
		expect(mockFetch).toHaveBeenCalledWith('/api/v1/queue/song/s1', expect.objectContaining({ method: 'POST' }));
	});

	it('enqueueAlbum calls POST /api/queue/album/{id}', async () => {
		const { result } = renderHook(() => usePlayer());
		await act(async () => {
			result.current.controls.enqueueAlbum('alb2');
		});
		expect(mockFetch).toHaveBeenCalledWith('/api/v1/queue/album/alb2', expect.objectContaining({ method: 'POST' }));
	});

	it('playPlaylist calls POST /api/play/playlist/{id}', async () => {
		const { result } = renderHook(() => usePlayer());
		await act(async () => {
			result.current.controls.playPlaylist('pl1');
		});
		expect(mockFetch).toHaveBeenCalledWith(
			'/api/v1/play/playlist/pl1',
			expect.objectContaining({ method: 'POST' })
		);
	});

	it('enqueuePlaylist calls POST /api/queue/playlist/{id}', async () => {
		const { result } = renderHook(() => usePlayer());
		await act(async () => {
			result.current.controls.enqueuePlaylist('pl2');
		});
		expect(mockFetch).toHaveBeenCalledWith(
			'/api/v1/queue/playlist/pl2',
			expect.objectContaining({ method: 'POST' })
		);
	});
});

describe('usePlayer — seek debounce', () => {
	it('debounces seek: multiple rapid calls result in one API call', async () => {
		vi.useFakeTimers();
		const { result } = renderHook(() => usePlayer());

		act(() => {
			result.current.controls.seek(10);
			result.current.controls.seek(20);
			result.current.controls.seek(30);
		});

		expect(mockFetch).not.toHaveBeenCalled();

		await act(async () => {
			vi.advanceTimersByTime(200);
		});

		expect(mockFetch).toHaveBeenCalledTimes(1);
		expect(mockFetch).toHaveBeenCalledWith(
			'/api/v1/seek',
			expect.objectContaining({ body: JSON.stringify({ position: 30 }) })
		);

		vi.useRealTimers();
	});

	it('fires seek API call after debounce delay', async () => {
		vi.useFakeTimers();
		const { result } = renderHook(() => usePlayer());

		act(() => {
			result.current.controls.seek(55);
		});

		expect(mockFetch).not.toHaveBeenCalled();

		await act(async () => {
			vi.advanceTimersByTime(150);
		});

		expect(mockFetch).toHaveBeenCalledWith(
			'/api/v1/seek',
			expect.objectContaining({ body: JSON.stringify({ position: 55 }) })
		);

		vi.useRealTimers();
	});
});
