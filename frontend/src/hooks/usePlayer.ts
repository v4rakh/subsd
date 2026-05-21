import { useState, useCallback, useMemo, useRef } from 'react';
import { useWebSocket } from './useWebSocket';
import type { PlayerState, SatelliteInfo, WsMessage } from '../types';
import { apiFetch } from '../api';
import { toast } from 'sonner';
import { useTranslation } from 'react-i18next';

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

export function usePlayer(): {
	playerState: PlayerState;
	controls: PlayerControls;
	connected: boolean;
	satellites: SatelliteInfo[];
} {
	const { t } = useTranslation();
	const [playerState, setPlayerState] = useState<PlayerState>(DEFAULT_STATE);
	const [satellites, setSatellites] = useState<SatelliteInfo[]>([]);
	const seekTimer = useRef<ReturnType<typeof setTimeout> | null>(null);

	const connected = useWebSocket<WsMessage>(
		useCallback(
			(msg) => {
				if (msg.type === 'satellites') {
					setSatellites(msg.satellites ?? []);
				} else if (msg.type === 'satellite_disconnected') {
					toast.warning(t('toast.satelliteDisconnected', { name: msg.name }));
				} else {
					setPlayerState({ ...msg, queue: msg.queue ?? [] });
				}
			},
			[t]
		)
	);

	const controls = useMemo<PlayerControls>(
		() => ({
			playPause: () => api('POST', '/api/v1/playpause'),
			next: () => api('POST', '/api/v1/next'),
			prev: () => api('POST', '/api/v1/prev'),
			shuffle: () => api('POST', '/api/v1/shuffle'),
			repeat: () => api('POST', '/api/v1/repeat'),
			seek: (pos) => {
				if (seekTimer.current) clearTimeout(seekTimer.current);
				seekTimer.current = setTimeout(() => api('POST', '/api/v1/seek', { position: pos }), 150);
			},
			setVolume: (vol) => api('POST', '/api/v1/volume', { volume: vol }),
			clearQueue: () => api('DELETE', '/api/v1/queue'),
			removeTrack: (idx) => api('DELETE', `/api/v1/queue/${idx}`),
			moveTrack: (from, to) => api('POST', '/api/v1/queue/move', { from, to }),
			jumpTo: (idx) => api('POST', `/api/v1/queue/jump/${idx}`),
			playSong: (id) => api('POST', `/api/v1/play/song/${id}`),
			playAlbum: (id) => api('POST', `/api/v1/play/album/${id}`),
			enqueueSong: (id) => api('POST', `/api/v1/queue/song/${id}`),
			enqueueAlbum: (id) => api('POST', `/api/v1/queue/album/${id}`),
			playPlaylist: (id) => api('POST', `/api/v1/play/playlist/${id}`),
			enqueuePlaylist: (id) => api('POST', `/api/v1/queue/playlist/${id}`)
		}),
		[]
	);

	return { playerState, controls, connected, satellites };
}
