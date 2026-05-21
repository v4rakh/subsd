import { PanelList, ListItem, EmptyState, SkeletonList } from './Panel';
import { apiUrl } from '../api';
import type { Playlist, Song } from '../types';
import { fmtDuration, fmtAudioMeta } from '@/lib/format';
import { Play, PlusIcon } from 'lucide-react';
import { useTranslation } from 'react-i18next';

interface Props {
	currentTrackId?: string;
	playlists: Playlist[];
	selectedPlaylist: Playlist | null;
	playlistSongs: Song[];
	loadingPlaylists: boolean;
	loadingPlaylistTracks: boolean;
	onSelectPlaylist: (p: Playlist) => void;
	onPlayPlaylist: (id: string) => void;
	onEnqueuePlaylist: (id: string) => void;
	onPlaySong: (id: string) => void;
	onEnqueueSong: (id: string) => void;
}

function PlaylistRow({
	playlist,
	onSelect,
	onPlay,
	onEnqueue
}: {
	playlist: Playlist;
	onSelect: (p: Playlist) => void;
	onPlay: (id: string) => void;
	onEnqueue: (id: string) => void;
}) {
	const { t } = useTranslation();
	return (
		<div
			className="group flex items-center gap-3 px-6 py-3 cursor-pointer hover:bg-bg2 transition-colors select-none"
			onClick={() => onSelect(playlist)}>
			{playlist.coverArt ? (
				<img
					className="w-8 h-8 rounded object-cover shrink-0"
					src={apiUrl(`/api/v1/coverart/${playlist.coverArt}`)}
					alt=""
					loading="lazy"
					onError={(e) => {
						(e.target as HTMLImageElement).style.display = 'none';
					}}
				/>
			) : (
				<div className="w-8 h-8 rounded bg-bg3 shrink-0" />
			)}
			<div className="min-w-0 flex-1">
				<div className="truncate">{playlist.name}</div>
				{playlist.songCount > 0 && (
					<div className="text-dim text-xs mt-0.5">
						{playlist.songCount} {t('playlistPanel.tracks')}
					</div>
				)}
			</div>
			<button
				className="shrink-0 lg:opacity-0 lg:group-hover:opacity-100 text-dim hover:text-brand transition-opacity p-2.5 rounded hover:bg-bg3"
				title={t('playlistPanel.playPlaylistTitle')}
				onClick={(e) => {
					e.stopPropagation();
					onPlay(playlist.id);
				}}>
				<Play size={14} />
			</button>
			<button
				className="shrink-0 lg:opacity-0 lg:group-hover:opacity-100 text-dim hover:text-brand transition-opacity p-2.5 rounded hover:bg-bg3"
				title={t('playlistPanel.enqueuePlaylistTitle')}
				onClick={(e) => {
					e.stopPropagation();
					onEnqueue(playlist.id);
				}}>
				<PlusIcon size={16} />
			</button>
		</div>
	);
}

function SongRow({
	song,
	playing,
	onPlay,
	onEnqueue
}: {
	song: Song;
	playing: boolean;
	onPlay: (id: string) => void;
	onEnqueue: (id: string) => void;
}) {
	const { t } = useTranslation();
	const meta = fmtAudioMeta(song);
	return (
		<ListItem playing={playing} onClick={() => onEnqueue(song.id)} onDoubleClick={() => onPlay(song.id)}>
			<span className="w-6 shrink-0 text-right text-dim">{song.track || '–'}</span>
			<div className="flex-1 min-w-0">
				<div className="truncate">{song.title}</div>
				{meta && <div className="truncate text-xs text-dim mt-0.5">{meta}</div>}
			</div>
			<button
				className="shrink-0 lg:opacity-0 lg:group-hover:opacity-100 text-dim hover:text-brand transition-opacity p-2.5 rounded hover:bg-bg3 pr-0.5!"
				title={t('playlistPanel.playSongTitle')}
				onClick={(e) => {
					e.stopPropagation();
					onPlay(song.id);
				}}>
				<Play size={13} />
			</button>
			<button
				className="shrink-0 lg:opacity-0 lg:group-hover:opacity-100 text-dim hover:text-brand transition-opacity p-2.5 rounded hover:bg-bg3 pr-0.5!"
				title={t('playlistPanel.enqueueSongTitle')}
				onClick={(e) => {
					e.stopPropagation();
					onEnqueue(song.id);
				}}>
				<PlusIcon size={14} />
			</button>
			<span className="text-dim shrink-0">{fmtDuration(song.duration)}</span>
		</ListItem>
	);
}

export function PlaylistBrowser({
	currentTrackId,
	playlists,
	selectedPlaylist,
	playlistSongs,
	loadingPlaylists,
	loadingPlaylistTracks,
	onSelectPlaylist,
	onPlayPlaylist,
	onEnqueuePlaylist,
	onPlaySong,
	onEnqueueSong
}: Props) {
	const { t } = useTranslation();

	if (selectedPlaylist) {
		return (
			<PanelList>
				{loadingPlaylistTracks ? (
					<SkeletonList rows={6} />
				) : playlistSongs.length === 0 ? (
					<EmptyState text={t('playlistPanel.noTracks')} />
				) : (
					playlistSongs.map((s) => (
						<SongRow
							key={s.id}
							song={s}
							playing={s.id === currentTrackId}
							onPlay={onPlaySong}
							onEnqueue={onEnqueueSong}
						/>
					))
				)}
			</PanelList>
		);
	}

	return (
		<PanelList>
			{loadingPlaylists ? (
				<SkeletonList rows={6} />
			) : playlists.length === 0 ? (
				<EmptyState text={t('playlistPanel.empty')} />
			) : (
				playlists.map((p) => (
					<PlaylistRow
						key={p.id}
						playlist={p}
						onSelect={onSelectPlaylist}
						onPlay={onPlayPlaylist}
						onEnqueue={onEnqueuePlaylist}
					/>
				))
			)}
		</PanelList>
	);
}
