import { Panel, PanelHeader, PanelList, PanelSearch, EmptyState, SkeletonList } from './Panel';
import { apiFetch, apiUrl } from '../api';
import type { Artist, Album, Song, SearchResult } from '../types';
import { Play, PlusIcon } from 'lucide-react';
import { useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';

interface Props {
	onSelectArtist: (artist: Artist) => void;
	onSelectAlbum: (album: Album) => void;
	onPlayAlbum: (id: string) => void;
	onEnqueueAlbum: (id: string) => void;
	onPlaySong: (id: string) => void;
	onEnqueueSong: (id: string) => void;
	className?: string;
	embedded?: boolean;
}

function SectionHeader({ label }: { label: string }) {
	return <div className="label-ui px-6 py-2 bg-bg1 border-b border-border border-t first:border-t-0">{label}</div>;
}

function ArtistRow({ artist, onSelect }: { artist: Artist; onSelect: (a: Artist) => void }) {
	const { t } = useTranslation();
	return (
		<div
			className="group flex items-center gap-3 px-6 py-3 cursor-pointer hover:bg-bg2 focus:bg-bg2 focus:outline-none transition-colors select-none"
			tabIndex={0}
			onClick={() => onSelect(artist)}
			onKeyDown={(e) => (e.key === 'Enter' || e.key === ' ') && onSelect(artist)}>
			{artist.coverArt ? (
				<img
					className="w-8 h-8 rounded object-cover shrink-0"
					src={apiUrl(`/api/coverart/${artist.coverArt}`)}
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
				<div className="truncate">{artist.name}</div>
				{artist.albumCount > 0 && (
					<div className="text-dim text-xs mt-0.5">
						{artist.albumCount} {t('artistPanel.albums')}
					</div>
				)}
			</div>
		</div>
	);
}

function AlbumRow({
	album,
	onSelect,
	onPlay,
	onEnqueue
}: {
	album: Album;
	onSelect: (a: Album) => void;
	onPlay: (id: string) => void;
	onEnqueue: (id: string) => void;
}) {
	const { t } = useTranslation();
	return (
		<div
			className="group flex items-center gap-3 px-6 py-3 cursor-pointer hover:bg-bg2 focus:bg-bg2 focus:outline-none transition-colors select-none"
			tabIndex={0}
			onClick={() => onSelect(album)}
			onKeyDown={(e) => (e.key === 'Enter' || e.key === ' ') && onSelect(album)}>
			{album.coverArt ? (
				<img
					className="w-8 h-8 rounded object-cover shrink-0"
					src={apiUrl(`/api/coverart/${album.coverArt}`)}
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
				<div className="truncate">{album.name}</div>
				<div className="truncate text-dim text-xs mt-0.5">
					{album.artist}
					{album.year ? ` · ${album.year}` : ''}
				</div>
			</div>
			<button
				className="shrink-0 lg:opacity-0 lg:group-hover:opacity-100 text-dim hover:text-brand transition-opacity p-2.5 rounded hover:bg-bg3"
				title={t('albumPanel.playTitle')}
				onClick={(e) => {
					e.stopPropagation();
					onPlay(album.id);
				}}>
				<Play size={14} />
			</button>
			<button
				className="shrink-0 lg:opacity-0 lg:group-hover:opacity-100 text-dim hover:text-brand transition-opacity p-2.5 rounded hover:bg-bg3"
				title={t('albumPanel.enqueueTitle')}
				onClick={(e) => {
					e.stopPropagation();
					onEnqueue(album.id);
				}}>
				<PlusIcon size={16} />
			</button>
		</div>
	);
}

function SongRow({
	song,
	onPlay,
	onEnqueue
}: {
	song: Song;
	onPlay: (id: string) => void;
	onEnqueue: (id: string) => void;
}) {
	const { t } = useTranslation();
	return (
		<div
			className="group flex items-center gap-3 px-6 py-3 cursor-pointer hover:bg-bg2 focus:bg-bg2 focus:outline-none transition-colors select-none"
			tabIndex={0}
			onClick={() => onEnqueue(song.id)}
			onKeyDown={(e) => (e.key === 'Enter' || e.key === ' ') && onEnqueue(song.id)}>
			<div className="min-w-0 flex-1">
				<div className="truncate">{song.title}</div>
				<div className="truncate text-dim text-xs mt-0.5">
					{song.artist}
					{song.album ? ` · ${song.album}` : ''}
				</div>
			</div>
			<button
				className="shrink-0 lg:opacity-0 lg:group-hover:opacity-100 text-dim hover:text-brand transition-opacity p-2.5 rounded hover:bg-bg3"
				title={t('trackPanel.playTitle')}
				onClick={(e) => {
					e.stopPropagation();
					onPlay(song.id);
				}}>
				<Play size={13} />
			</button>
			<button
				className="shrink-0 lg:opacity-0 lg:group-hover:opacity-100 text-dim hover:text-brand transition-opacity p-2.5 rounded hover:bg-bg3"
				title={t('trackPanel.queueTrackTitle')}
				onClick={(e) => {
					e.stopPropagation();
					onEnqueue(song.id);
				}}>
				<PlusIcon size={14} />
			</button>
		</div>
	);
}

export function SearchPanel({
	onSelectArtist,
	onSelectAlbum,
	onPlayAlbum,
	onEnqueueAlbum,
	onPlaySong,
	onEnqueueSong,
	className,
	embedded
}: Props) {
	const { t } = useTranslation();
	const [query, setQuery] = useState('');
	const [results, setResults] = useState<SearchResult | null>(null);
	const [loading, setLoading] = useState(false);
	const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
	const abortRef = useRef<AbortController | null>(null);

	useEffect(() => {
		if (timerRef.current) clearTimeout(timerRef.current);
		if (!query.trim()) {
			setResults(null);
			return;
		}
		timerRef.current = setTimeout(async () => {
			const controller = new AbortController();
			abortRef.current = controller;
			setLoading(true);
			try {
				const r = await apiFetch(`/api/search?q=${encodeURIComponent(query)}`, {
					signal: controller.signal
				});
				setResults(await r.json());
			} catch (e) {
				if (e instanceof Error && e.name === 'AbortError') return;
			} finally {
				if (!controller.signal.aborted) setLoading(false);
			}
		}, 280);
		return () => {
			if (timerRef.current) clearTimeout(timerRef.current);
			abortRef.current?.abort();
		};
	}, [query]);

	const artists = results?.artist ?? [];
	const albums = results?.album ?? [];
	const songs = results?.song ?? [];
	const hasResults = artists.length > 0 || albums.length > 0 || songs.length > 0;

	const body = (
		<>
			<PanelSearch
				value={query}
				onChange={setQuery}
				placeholder={t('searchPanel.placeholder')}
				autoFocus={embedded}
			/>
			<PanelList>
				{loading ? (
					<SkeletonList rows={5} />
				) : !query.trim() ? (
					<EmptyState text={t('searchPanel.hint')} />
				) : !hasResults ? (
					<EmptyState text={t('searchPanel.noResults')} />
				) : (
					<>
						{artists.length > 0 && (
							<>
								<SectionHeader label={t('tabs.artists')} />
								{artists.map((a) => (
									<ArtistRow key={a.id} artist={a} onSelect={onSelectArtist} />
								))}
							</>
						)}
						{albums.length > 0 && (
							<>
								<SectionHeader label={t('tabs.albums')} />
								{albums.map((a) => (
									<AlbumRow
										key={a.id}
										album={a}
										onSelect={onSelectAlbum}
										onPlay={onPlayAlbum}
										onEnqueue={onEnqueueAlbum}
									/>
								))}
							</>
						)}
						{songs.length > 0 && (
							<>
								<SectionHeader label={t('tabs.tracks')} />
								{songs.map((s) => (
									<SongRow key={s.id} song={s} onPlay={onPlaySong} onEnqueue={onEnqueueSong} />
								))}
							</>
						)}
					</>
				)}
			</PanelList>
		</>
	);

	if (embedded) {
		return <div className="flex flex-col flex-1 min-h-0 overflow-hidden p-5!">{body}</div>;
	}

	return (
		<Panel className={className}>
			<PanelHeader title={t('searchPanel.title')} />
			{body}
		</Panel>
	);
}
