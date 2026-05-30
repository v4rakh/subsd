import { useState, useCallback } from 'react';
import type { Artist, Album, Song, Playlist } from '../types';
import { apiFetch } from '../api';

interface LoadingState {
	artists: boolean;
	albums: boolean;
	tracks: boolean;
	playlists: boolean;
	playlistTracks: boolean;
}

export interface LibraryState {
	artists: Artist[];
	albums: Album[];
	songs: Song[];
	selectedArtist: Artist | null;
	selectedAlbum: Album | null;
	playlists: Playlist[];
	selectedPlaylist: Playlist | null;
	playlistSongs: Song[];
	loading: LoadingState;
}

export interface LibraryActions {
	loadArtists: () => Promise<void>;
	selectArtist: (artist: Artist) => Promise<void>;
	selectAlbum: (album: Album) => Promise<void>;
	loadPlaylists: () => Promise<void>;
	selectPlaylist: (playlist: Playlist) => Promise<void>;
	clearSelectedPlaylist: () => void;
	setAlbumRating: (albumId: string, rating: number) => Promise<void>;
	setSongRating: (songId: string, rating: number) => Promise<void>;
	createPlaylist: (name: string) => Promise<void>;
	saveQueueAsPlaylist: (name: string, queue: { id: string }[]) => Promise<void>;
	renamePlaylist: (id: string, newName: string) => Promise<void>;
	deletePlaylist: (id: string) => Promise<void>;
	addSongsToPlaylist: (id: string, songIds: string[]) => Promise<void>;
	addAlbumToPlaylist: (id: string, albumId: string) => Promise<void>;
	removeSongFromPlaylist: (id: string, index: number) => Promise<void>;
	reorderPlaylist: (id: string, newSongIds: string[]) => Promise<void>;
	appendQueueToPlaylist: (id: string) => Promise<void>;
}

export function useLibrary(): LibraryState & LibraryActions {
	const [artists, setArtists] = useState<Artist[]>([]);
	const [albums, setAlbums] = useState<Album[]>([]);
	const [songs, setSongs] = useState<Song[]>([]);
	const [selectedArtist, setSelectedArtist] = useState<Artist | null>(null);
	const [selectedAlbum, setSelectedAlbum] = useState<Album | null>(null);
	const [playlists, setPlaylists] = useState<Playlist[]>([]);
	const [selectedPlaylist, setSelectedPlaylist] = useState<Playlist | null>(null);
	const [playlistSongs, setPlaylistSongs] = useState<Song[]>([]);
	const [loading, setLoading] = useState<LoadingState>({
		artists: false,
		albums: false,
		tracks: false,
		playlists: false,
		playlistTracks: false
	});

	const loadArtists = useCallback(async () => {
		setLoading((l) => ({ ...l, artists: true }));
		try {
			const r = await apiFetch('/api/v1/artists');
			setArtists((await r.json()) ?? []);
		} finally {
			setLoading((l) => ({ ...l, artists: false }));
		}
	}, []);

	const selectArtist = useCallback(async (artist: Artist) => {
		setSelectedArtist(artist);
		setSelectedAlbum(null);
		setSongs([]);
		setLoading((l) => ({ ...l, albums: true }));
		try {
			const r = await apiFetch(`/api/v1/artist/${artist.id}`);
			const data: Artist = await r.json();
			setAlbums(data.album ?? []);
		} finally {
			setLoading((l) => ({ ...l, albums: false }));
		}
	}, []);

	const selectAlbum = useCallback(async (album: Album) => {
		setSelectedAlbum(album);
		setLoading((l) => ({ ...l, tracks: true }));
		try {
			const r = await apiFetch(`/api/v1/album/${album.id}`);
			const data: Album = await r.json();
			setSongs(data.song ?? []);
		} finally {
			setLoading((l) => ({ ...l, tracks: false }));
		}
	}, []);

	const loadPlaylists = useCallback(async () => {
		setLoading((l) => ({ ...l, playlists: true }));
		try {
			const r = await apiFetch('/api/v1/playlists');
			setPlaylists((await r.json()) ?? []);
		} finally {
			setLoading((l) => ({ ...l, playlists: false }));
		}
	}, []);

	const selectPlaylist = useCallback(async (playlist: Playlist) => {
		setSelectedPlaylist(playlist);
		setPlaylistSongs([]);
		setLoading((l) => ({ ...l, playlistTracks: true }));
		try {
			const r = await apiFetch(`/api/v1/playlist/${playlist.id}`);
			const data: Playlist = await r.json();
			setPlaylistSongs(data.entry ?? []);
		} finally {
			setLoading((l) => ({ ...l, playlistTracks: false }));
		}
	}, []);

	const clearSelectedPlaylist = useCallback(() => {
		setSelectedPlaylist(null);
		setPlaylistSongs([]);
	}, []);

	const createPlaylist = useCallback(async (name: string) => {
		await apiFetch('/api/v1/playlist', {
			method: 'POST',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify({ name, songIds: [] })
		});
		const r = await apiFetch('/api/v1/playlists');
		setPlaylists((await r.json()) ?? []);
	}, []);

	const saveQueueAsPlaylist = useCallback(async (name: string, queue: { id: string }[]) => {
		await apiFetch('/api/v1/playlist/from-queue', {
			method: 'POST',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify({ name })
		});
		// queue param unused server-side (server reads queue from player state), kept for callsite clarity
		void queue;
		const r = await apiFetch('/api/v1/playlists');
		setPlaylists((await r.json()) ?? []);
	}, []);

	const renamePlaylist = useCallback(async (id: string, newName: string) => {
		await apiFetch(`/api/v1/playlist/${id}`, {
			method: 'PUT',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify({ name: newName })
		});
		setPlaylists((prev) => prev.map((p) => (p.id === id ? { ...p, name: newName } : p)));
		setSelectedPlaylist((prev) => (prev?.id === id ? { ...prev, name: newName } : prev));
	}, []);

	const deletePlaylist = useCallback(
		async (id: string) => {
			await apiFetch(`/api/v1/playlist/${id}`, { method: 'DELETE' });
			setPlaylists((prev) => prev.filter((p) => p.id !== id));
			if (selectedPlaylist?.id === id) {
				setSelectedPlaylist(null);
				setPlaylistSongs([]);
			}
		},
		[selectedPlaylist]
	);

	const addSongsToPlaylist = useCallback(async (id: string, songIds: string[]) => {
		await apiFetch(`/api/v1/playlist/${id}/songs`, {
			method: 'POST',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify({ songIds })
		});
		setPlaylists((prev) => prev.map((p) => (p.id === id ? { ...p, songCount: p.songCount + songIds.length } : p)));
	}, []);

	const removeSongFromPlaylist = useCallback(async (id: string, index: number) => {
		await apiFetch(`/api/v1/playlist/${id}/songs/${index}`, { method: 'DELETE' });
		setPlaylistSongs((prev) => prev.filter((_, i) => i !== index));
		setPlaylists((prev) => prev.map((p) => (p.id === id ? { ...p, songCount: Math.max(0, p.songCount - 1) } : p)));
	}, []);

	const reorderPlaylist = useCallback(async (id: string, newSongIds: string[]) => {
		await apiFetch(`/api/v1/playlist/${id}/songs`, {
			method: 'PUT',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify({ songIds: newSongIds })
		});
	}, []);

	const addAlbumToPlaylist = useCallback(async (id: string, albumId: string) => {
		await apiFetch(`/api/v1/playlist/${id}/album/${albumId}`, { method: 'POST' });
	}, []);

	const appendQueueToPlaylist = useCallback(async (id: string) => {
		await apiFetch(`/api/v1/playlist/${id}/add-queue`, { method: 'POST' });
	}, []);

	const setAlbumRating = useCallback(async (albumId: string, rating: number) => {
		setAlbums((prev) => prev.map((a) => (a.id === albumId ? { ...a, userRating: rating } : a)));
		await apiFetch('/api/v1/rating', {
			method: 'POST',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify({ id: albumId, rating })
		});
	}, []);

	const setSongRating = useCallback(async (songId: string, rating: number) => {
		setSongs((prev) => prev.map((s) => (s.id === songId ? { ...s, userRating: rating } : s)));
		await apiFetch('/api/v1/rating', {
			method: 'POST',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify({ id: songId, rating })
		});
	}, []);

	return {
		artists,
		albums,
		songs,
		selectedArtist,
		selectedAlbum,
		playlists,
		selectedPlaylist,
		playlistSongs,
		loading,
		loadArtists,
		selectArtist,
		selectAlbum,
		loadPlaylists,
		selectPlaylist,
		clearSelectedPlaylist,
		setAlbumRating,
		setSongRating,
		createPlaylist,
		saveQueueAsPlaylist,
		renamePlaylist,
		deletePlaylist,
		addSongsToPlaylist,
		addAlbumToPlaylist,
		removeSongFromPlaylist,
		reorderPlaylist,
		appendQueueToPlaylist
	};
}
