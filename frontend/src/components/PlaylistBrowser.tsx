import { PanelList, ListItem, EmptyState, SkeletonList } from './Panel';
import { apiUrl } from '../api';
import type { Playlist, Song } from '../types';
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter } from './ui/dialog';
import { fmtDuration, fmtAudioMeta } from '@/lib/format';
import { DndContext, closestCenter, PointerSensor, useSensor, useSensors, type DragEndEvent } from '@dnd-kit/core';
import { SortableContext, useSortable, verticalListSortingStrategy } from '@dnd-kit/sortable';
import { CSS } from '@dnd-kit/utilities';
import { Play, PlusIcon, Pencil, Trash2, GripVertical, X } from 'lucide-react';
import { useState } from 'react';
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
	onRenamePlaylist?: (id: string, newName: string) => Promise<void>;
	onDeletePlaylist?: (id: string) => Promise<void>;
	onRemoveSongFromPlaylist?: (id: string, index: number) => Promise<void>;
	onReorderPlaylist?: (id: string, newSongIds: string[]) => Promise<void>;
	onAppendQueueToPlaylist?: (id: string) => Promise<void>;
}

// ── Name dialog (create / rename) ────────────────────────────────────────────

function NameDialog({
	open,
	title,
	initial,
	onConfirm,
	onClose
}: {
	open: boolean;
	title: string;
	initial: string;
	onConfirm: (name: string) => void;
	onClose: () => void;
}) {
	const { t } = useTranslation();
	const [value, setValue] = useState(initial);

	function handleOpen(isOpen: boolean) {
		if (isOpen) setValue(initial);
		else onClose();
	}

	return (
		<Dialog open={open} onOpenChange={handleOpen}>
			<DialogContent showCloseButton={false}>
				<DialogHeader>
					<DialogTitle>{title}</DialogTitle>
				</DialogHeader>
				<input
					className="w-full rounded border border-border bg-bg2 px-3 py-2 text-sm text-text placeholder:text-dim focus:outline-none focus:ring-1 focus:ring-brand"
					placeholder={t('playlistPanel.namePlaceholder')}
					value={value}
					autoFocus
					onChange={(e) => setValue(e.target.value)}
					onKeyDown={(e) => {
						if (e.key === 'Enter' && value.trim()) onConfirm(value.trim());
						if (e.key === 'Escape') onClose();
					}}
				/>
				<DialogFooter>
					<button
						className="text-sm px-3 py-1.5 rounded bg-bg3 hover:bg-bg2 text-dim transition-colors"
						onClick={onClose}>
						{t('playlistPanel.cancelButton')}
					</button>
					<button
						className="text-sm px-3 py-1.5 rounded bg-brand text-white hover:opacity-90 transition-opacity disabled:opacity-50"
						disabled={!value.trim()}
						onClick={() => onConfirm(value.trim())}>
						{t('playlistPanel.confirmButton')}
					</button>
				</DialogFooter>
			</DialogContent>
		</Dialog>
	);
}

// ── Delete confirm dialog ─────────────────────────────────────────────────────

function DeleteDialog({
	open,
	name,
	onConfirm,
	onClose
}: {
	open: boolean;
	name: string;
	onConfirm: () => void;
	onClose: () => void;
}) {
	const { t } = useTranslation();
	return (
		<Dialog open={open} onOpenChange={(o) => !o && onClose()}>
			<DialogContent showCloseButton={false}>
				<DialogHeader>
					<DialogTitle>{t('playlistPanel.deleteTitle')}</DialogTitle>
				</DialogHeader>
				<p className="text-sm text-dim">{t('playlistPanel.deleteConfirm', { name })}</p>
				<DialogFooter>
					<button
						className="text-sm px-3 py-1.5 rounded bg-bg3 hover:bg-bg2 text-dim transition-colors"
						onClick={onClose}>
						{t('playlistPanel.cancelButton')}
					</button>
					<button
						className="text-sm px-3 py-1.5 rounded bg-red text-white hover:opacity-90 transition-opacity"
						onClick={onConfirm}>
						{t('playlistPanel.deleteTitle')}
					</button>
				</DialogFooter>
			</DialogContent>
		</Dialog>
	);
}

// ── Playlist list row ─────────────────────────────────────────────────────────

function PlaylistRow({
	playlist,
	onSelect,
	onPlay,
	onEnqueue,
	onRename,
	onDelete,
	onAppendQueue
}: {
	playlist: Playlist;
	onSelect: (p: Playlist) => void;
	onPlay: (id: string) => void;
	onEnqueue: (id: string) => void;
	onRename?: (p: Playlist) => void;
	onDelete?: (p: Playlist) => void;
	onAppendQueue?: (id: string) => void;
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
			{onAppendQueue && (
				<button
					className="shrink-0 lg:opacity-0 lg:group-hover:opacity-100 text-dim hover:text-brand transition-opacity p-2.5 rounded hover:bg-bg3"
					title={t('playlistPanel.appendQueue')}
					onClick={(e) => {
						e.stopPropagation();
						onAppendQueue(playlist.id);
					}}>
					<PlusIcon size={14} />
				</button>
			)}
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
			{onRename && (
				<button
					className="shrink-0 lg:opacity-0 lg:group-hover:opacity-100 text-dim hover:text-brand transition-opacity p-2.5 rounded hover:bg-bg3"
					title={t('playlistPanel.renameTitle')}
					onClick={(e) => {
						e.stopPropagation();
						onRename(playlist);
					}}>
					<Pencil size={13} />
				</button>
			)}
			{onDelete && (
				<button
					className="shrink-0 lg:opacity-0 lg:group-hover:opacity-100 text-dim hover:text-red transition-opacity p-2.5 rounded hover:bg-bg3"
					title={t('playlistPanel.deleteTitle')}
					onClick={(e) => {
						e.stopPropagation();
						onDelete(playlist);
					}}>
					<Trash2 size={13} />
				</button>
			)}
		</div>
	);
}

// ── Song row (detail view) ────────────────────────────────────────────────────

function SongRow({
	song,
	index,
	playing,
	onPlay,
	onEnqueue,
	onRemove,
	dragHandleProps
}: {
	song: Song;
	index: number;
	playing: boolean;
	onPlay: (id: string) => void;
	onEnqueue: (id: string) => void;
	onRemove?: (index: number) => void;
	dragHandleProps?: Record<string, unknown>;
}) {
	const { t } = useTranslation();
	const meta = fmtAudioMeta(song);
	return (
		<ListItem playing={playing} onClick={() => onEnqueue(song.id)} onDoubleClick={() => onPlay(song.id)}>
			{onRemove && (
				<span
					className="w-5 shrink-0 flex items-center justify-center lg:opacity-0 lg:group-hover:opacity-100 text-dim transition-opacity cursor-grab touch-none"
					{...dragHandleProps}
					onClick={(e) => e.stopPropagation()}>
					<GripVertical size={14} />
				</span>
			)}
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
			{onRemove && (
				<button
					className="shrink-0 lg:opacity-0 lg:group-hover:opacity-100 text-dim hover:text-red transition-opacity p-2.5 rounded hover:bg-bg3 pr-0.5!"
					title={t('playlistPanel.removeTitle')}
					onClick={(e) => {
						e.stopPropagation();
						onRemove(index);
					}}>
					<X size={13} />
				</button>
			)}
			<span className="text-dim shrink-0">{fmtDuration(song.duration)}</span>
		</ListItem>
	);
}

function SortableSongRow(props: Parameters<typeof SongRow>[0]) {
	const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({
		id: String(props.index)
	});
	return (
		<div
			ref={setNodeRef}
			style={{
				transform: CSS.Transform.toString(transform),
				transition,
				opacity: isDragging ? 0.4 : 1,
				zIndex: isDragging ? 1 : undefined,
				position: 'relative'
			}}>
			<SongRow {...props} dragHandleProps={{ ...attributes, ...listeners }} />
		</div>
	);
}

// ── Main component ────────────────────────────────────────────────────────────

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
	onEnqueueSong,
	onRenamePlaylist,
	onDeletePlaylist,
	onRemoveSongFromPlaylist,
	onReorderPlaylist,
	onAppendQueueToPlaylist
}: Props) {
	const { t } = useTranslation();
	const [renaming, setRenaming] = useState<Playlist | null>(null);
	const [deleting, setDeleting] = useState<Playlist | null>(null);
	const [localSongs, setLocalSongs] = useState<Song[]>(playlistSongs);

	// Keep localSongs in sync when the prop changes (new playlist opened).
	if (localSongs !== playlistSongs && !localSongs.every((s, i) => s.id === playlistSongs[i]?.id)) {
		setLocalSongs(playlistSongs);
	}

	const sensors = useSensors(useSensor(PointerSensor, { activationConstraint: { distance: 5 } }));

	function handleDragEnd(event: DragEndEvent) {
		const { active, over } = event;
		if (!over || active.id === over.id || !selectedPlaylist || !onReorderPlaylist) return;
		const fromIdx = Number(active.id);
		const toIdx = Number(over.id);
		const reordered = [...localSongs];
		const [moved] = reordered.splice(fromIdx, 1);
		reordered.splice(toIdx, 0, moved);
		setLocalSongs(reordered);
		onReorderPlaylist(
			selectedPlaylist.id,
			reordered.map((s) => s.id)
		);
	}

	if (selectedPlaylist) {
		const songs = localSongs.length > 0 || playlistSongs.length === 0 ? localSongs : playlistSongs;
		const canReorder = !!onReorderPlaylist && !!onRemoveSongFromPlaylist;
		return (
			<PanelList>
				{loadingPlaylistTracks ? (
					<SkeletonList rows={6} />
				) : songs.length === 0 ? (
					<EmptyState text={t('playlistPanel.noTracks')} />
				) : canReorder ? (
					<DndContext sensors={sensors} collisionDetection={closestCenter} onDragEnd={handleDragEnd}>
						<SortableContext items={songs.map((_, i) => String(i))} strategy={verticalListSortingStrategy}>
							{songs.map((s, i) => (
								<SortableSongRow
									key={`${s.id}-${i}`}
									song={s}
									index={i}
									playing={s.id === currentTrackId}
									onPlay={onPlaySong}
									onEnqueue={onEnqueueSong}
									onRemove={
										onRemoveSongFromPlaylist
											? (idx) => onRemoveSongFromPlaylist(selectedPlaylist.id, idx)
											: undefined
									}
								/>
							))}
						</SortableContext>
					</DndContext>
				) : (
					songs.map((s, i) => (
						<SongRow
							key={`${s.id}-${i}`}
							song={s}
							index={i}
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
		<>
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
							onRename={onRenamePlaylist ? setRenaming : undefined}
							onDelete={onDeletePlaylist ? setDeleting : undefined}
							onAppendQueue={onAppendQueueToPlaylist}
						/>
					))
				)}
			</PanelList>

			{onRenamePlaylist && (
				<NameDialog
					open={!!renaming}
					title={t('playlistPanel.renameTitle')}
					initial={renaming?.name ?? ''}
					onConfirm={(name) => {
						if (renaming) onRenamePlaylist(renaming.id, name);
						setRenaming(null);
					}}
					onClose={() => setRenaming(null)}
				/>
			)}

			{onDeletePlaylist && (
				<DeleteDialog
					open={!!deleting}
					name={deleting?.name ?? ''}
					onConfirm={() => {
						if (deleting) onDeletePlaylist(deleting.id);
						setDeleting(null);
					}}
					onClose={() => setDeleting(null)}
				/>
			)}
		</>
	);
}
