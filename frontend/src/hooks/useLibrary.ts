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
		clearSelectedPlaylist
	};
}
