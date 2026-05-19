import { useState, useCallback, useMemo, useRef } from 'react';
import { useWebSocket } from './useWebSocket';
import type { PlayerState } from '../types';
import { apiFetch } from '../api';

const DEFAULT_STATE: PlayerState = {
	playing: false,
	currentIdx: -1,
	queue: [],
	position: 0,
	duration: 0,
	volume: 100,
	shuffle: false,
	repeat: false
};

async function api(method: string, path: string, body?: unknown): Promise<void> {
	await apiFetch(path, {
		method,
		headers: body != null ? { 'Content-Type': 'application/json' } : {},
		body: body != null ? JSON.stringify(body) : undefined
	});
}

export interface PlayerControls {
	playPause: () => void;
	next: () => void;
	prev: () => void;
	shuffle: () => void;
	repeat: () => void;
	seek: (position: number) => void;
	setVolume: (volume: number) => void;
	clearQueue: () => void;
	removeTrack: (idx: number) => void;
	moveTrack: (from: number, to: number) => void;
	jumpTo: (idx: number) => void;
	playSong: (id: string) => void;
	playAlbum: (id: string) => void;
	enqueueSong: (id: string) => void;
	enqueueAlbum: (id: string) => void;
	playPlaylist: (id: string) => void;
	enqueuePlaylist: (id: string) => void;
}

export function usePlayer(): { playerState: PlayerState; controls: PlayerControls; connected: boolean } {
	const [playerState, setPlayerState] = useState<PlayerState>(DEFAULT_STATE);
	const seekTimer = useRef<ReturnType<typeof setTimeout> | null>(null);

	const connected = useWebSocket<PlayerState>(
		useCallback((data) => setPlayerState({ ...data, queue: data.queue ?? [] }), [])
	);

	const controls = useMemo<PlayerControls>(
		() => ({
			playPause: () => api('POST', '/api/playpause'),
			next: () => api('POST', '/api/next'),
			prev: () => api('POST', '/api/prev'),
			shuffle: () => api('POST', '/api/shuffle'),
			repeat: () => api('POST', '/api/repeat'),
			seek: (pos) => {
				if (seekTimer.current) clearTimeout(seekTimer.current);
				seekTimer.current = setTimeout(() => api('POST', '/api/seek', { position: pos }), 150);
			},
			setVolume: (vol) => api('POST', '/api/volume', { volume: vol }),
			clearQueue: () => api('DELETE', '/api/queue'),
			removeTrack: (idx) => api('DELETE', `/api/queue/${idx}`),
			moveTrack: (from, to) => api('POST', '/api/queue/move', { from, to }),
			jumpTo: (idx) => api('POST', `/api/queue/jump/${idx}`),
			playSong: (id) => api('POST', `/api/play/song/${id}`),
			playAlbum: (id) => api('POST', `/api/play/album/${id}`),
			enqueueSong: (id) => api('POST', `/api/queue/song/${id}`),
			enqueueAlbum: (id) => api('POST', `/api/queue/album/${id}`),
			playPlaylist: (id) => api('POST', `/api/play/playlist/${id}`),
			enqueuePlaylist: (id) => api('POST', `/api/queue/playlist/${id}`)
		}),
		[]
	);

	return { playerState, controls, connected };
}
