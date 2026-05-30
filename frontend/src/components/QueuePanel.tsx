import { Panel, PanelHeader, PanelList, PanelSearch, EmptyState } from './Panel';
import { PlaylistBrowser } from './PlaylistBrowser';
import type { PlayerState, Track, Playlist, Song } from '../types';
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter } from './ui/dialog';
import { cn } from '@/lib/utils';
import { DndContext, closestCenter, PointerSensor, useSensor, useSensors, type DragEndEvent } from '@dnd-kit/core';
import { SortableContext, useSortable, verticalListSortingStrategy } from '@dnd-kit/sortable';
import { CSS } from '@dnd-kit/utilities';
import { XIcon, GripVertical, Play, ListPlus } from 'lucide-react';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';

interface RowProps {
	track: Track;
	i: number;
	currentIdx: number;
	onJump: (idx: number) => void;
	onRemove: (idx: number) => void;
	dragHandleProps?: Record<string, unknown>;
}

function QueueRow({ track, i, currentIdx, onJump, onRemove, dragHandleProps }: RowProps) {
	const { t } = useTranslation();
	return (
		<div
			className={cn(
				'group flex items-center gap-2 pl-1.5 pr-4 py-5 cursor-pointer transition-colors duration-75 select-none',
				i === currentIdx ? 'text-green' : 'hover:bg-bg2'
			)}
			onClick={() => onJump(i)}>
			<span
				className="w-5 shrink-0 flex items-center justify-center lg:opacity-0 lg:group-hover:opacity-100 text-dim transition-opacity cursor-grab touch-none"
				{...dragHandleProps}
				onClick={(e) => e.stopPropagation()}>
				<GripVertical size={14} />
			</span>
			<div className="flex-1 min-w-0">
				<div className="truncate">{track.title || track.id}</div>
				<div className="text-dim truncate text-xs">
					{track.artist}
					{track.album && <span> · [{track.album}]</span>}
				</div>
			</div>
			<button
				className="shrink-0 lg:opacity-0 lg:group-hover:opacity-100 text-dim hover:text-red transition-opacity p-2.5 rounded pr-0.5!"
				onClick={(e) => {
					e.stopPropagation();
					onRemove(i);
				}}
				title={t('queuePanel.removeTitle')}>
				<XIcon size={14} />
			</button>
		</div>
	);
}

function SortableRow(props: RowProps) {
	const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({
		id: String(props.i)
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
			<QueueRow {...props} dragHandleProps={{ ...attributes, ...listeners }} />
		</div>
	);
}

interface Props {
	playerState: PlayerState;
	onJump: (idx: number) => void;
	onRemove: (idx: number) => void;
	onClear: () => void;
	onMove: (from: number, to: number) => void;
	// Playlist browser props
	playlists: Playlist[];
	selectedPlaylist: Playlist | null;
	playlistSongs: Song[];
	loadingPlaylists: boolean;
	loadingPlaylistTracks: boolean;
	onSelectPlaylist: (p: Playlist) => void;
	onClearPlaylist: () => void;
	onPlayPlaylist: (id: string) => void;
	onEnqueuePlaylist: (id: string) => void;
	onPlaySong: (id: string) => void;
	onEnqueueSong: (id: string) => void;
	onSaveQueueAsPlaylist?: (name: string) => Promise<void>;
	onCreatePlaylist?: (name: string) => Promise<void>;
	onRenamePlaylist?: (id: string, newName: string) => Promise<void>;
	onDeletePlaylist?: (id: string) => Promise<void>;
	onRemoveSongFromPlaylist?: (id: string, index: number) => Promise<void>;
	onReorderPlaylist?: (id: string, newSongIds: string[]) => Promise<void>;
	onAppendQueueToPlaylist?: (id: string) => Promise<void>;
	className?: string;
}

export function QueuePanel({
	playerState,
	onJump,
	onRemove,
	onClear,
	onMove,
	playlists,
	selectedPlaylist,
	playlistSongs,
	loadingPlaylists,
	loadingPlaylistTracks,
	onSelectPlaylist,
	onClearPlaylist,
	onPlayPlaylist,
	onEnqueuePlaylist,
	onPlaySong,
	onEnqueueSong,
	onSaveQueueAsPlaylist,
	onCreatePlaylist,
	onRenamePlaylist,
	onDeletePlaylist,
	onRemoveSongFromPlaylist,
	onReorderPlaylist,
	onAppendQueueToPlaylist,
	className
}: Props) {
	const { t } = useTranslation();
	const [query, setQuery] = useState('');
	const [mode, setMode] = useState<'queue' | 'playlists'>('queue');
	const [saveQueueDialog, setSaveQueueDialog] = useState(false);
	const [saveQueueName, setSaveQueueName] = useState('');
	const [createDialog, setCreateDialog] = useState(false);
	const [createName, setCreateName] = useState('');
	const { queue, currentIdx } = playerState;

	const filtered = query.trim()
		? queue
				.map((track, i) => ({ track, i }))
				.filter(
					({ track }) =>
						track.title.toLowerCase().includes(query.toLowerCase()) ||
						(track.artist ?? '').toLowerCase().includes(query.toLowerCase())
				)
		: queue.map((track, i) => ({ track, i }));

	const canDrag = !query.trim();

	const sensors = useSensors(useSensor(PointerSensor, { activationConstraint: { distance: 5 } }));

	function handleDragEnd(event: DragEndEvent) {
		const { active, over } = event;
		if (over && active.id !== over.id) {
			onMove(Number(active.id), Number(over.id));
		}
	}

	const queueLabel = `${t('queuePanel.title')}${queue.length ? ' · ' + queue.length : ''}`;

	const playlistBrowserMutationProps = {
		onRenamePlaylist,
		onDeletePlaylist,
		onRemoveSongFromPlaylist,
		onReorderPlaylist,
		onAppendQueueToPlaylist
	};

	if (mode === 'playlists') {
		if (selectedPlaylist) {
			return (
				<Panel className={className}>
					<PanelHeader
						title={selectedPlaylist.name}
						backLabel={t('playlistPanel.title')}
						onBack={onClearPlaylist}>
						<button
							className="hidden lg:block text-sm text-dim hover:text-text transition-colors px-2 py-1.5 rounded hover:bg-bg2 shrink-0 pr-0.5!"
							onClick={() => onClearPlaylist()}>
							← {t('playlistPanel.title')}
						</button>
						<button
							className="flex items-center gap-1 text-sm text-dim hover:text-brand transition-colors px-2 py-1.5 rounded hover:bg-bg2 shrink-0 pr-0.5!"
							onClick={() => onPlayPlaylist(selectedPlaylist.id)}>
							<Play size={13} />
							{t('playlistPanel.playAll')}
						</button>
						<button
							className="text-sm text-dim hover:text-brand transition-colors px-2 py-1.5 rounded hover:bg-bg2 shrink-0 pr-0.5!"
							onClick={() => onEnqueuePlaylist(selectedPlaylist.id)}>
							{t('playlistPanel.queueAll')}
						</button>
					</PanelHeader>
					<PlaylistBrowser
						currentTrackId={queue[currentIdx]?.id}
						playlists={playlists}
						selectedPlaylist={selectedPlaylist}
						playlistSongs={playlistSongs}
						loadingPlaylists={loadingPlaylists}
						loadingPlaylistTracks={loadingPlaylistTracks}
						onSelectPlaylist={onSelectPlaylist}
						onPlayPlaylist={onPlayPlaylist}
						onEnqueuePlaylist={onEnqueuePlaylist}
						onPlaySong={onPlaySong}
						onEnqueueSong={onEnqueueSong}
						{...playlistBrowserMutationProps}
					/>
				</Panel>
			);
		}

		return (
			<Panel className={className}>
				<PanelHeader title={t('playlistPanel.title')}>
					<button
						className="hidden lg:block text-sm text-dim hover:text-text transition-colors px-2 py-1.5 rounded hover:bg-bg2 shrink-0 pr-2!"
						onClick={() => setMode('queue')}>
						← {t('queuePanel.title')}
					</button>
					{onCreatePlaylist && (
						<button
							className="text-sm text-dim hover:text-brand transition-colors px-2 py-1.5 rounded hover:bg-bg2 shrink-0"
							title={t('playlistPanel.newPlaylist')}
							onClick={() => {
								setCreateName('');
								setCreateDialog(true);
							}}>
							+ {t('playlistPanel.newPlaylist')}
						</button>
					)}
				</PanelHeader>
				<PlaylistBrowser
					playlists={playlists}
					selectedPlaylist={selectedPlaylist}
					playlistSongs={playlistSongs}
					loadingPlaylists={loadingPlaylists}
					loadingPlaylistTracks={loadingPlaylistTracks}
					onSelectPlaylist={onSelectPlaylist}
					onPlayPlaylist={onPlayPlaylist}
					onEnqueuePlaylist={onEnqueuePlaylist}
					onPlaySong={onPlaySong}
					onEnqueueSong={onEnqueueSong}
					{...playlistBrowserMutationProps}
				/>
			</Panel>
		);
	}

	return (
		<>
			<Panel className={className}>
				<PanelHeader title={queueLabel}>
					<button
						className="hidden lg:block text-sm text-dim hover:text-brand transition-colors px-2 py-1.5 rounded hover:bg-bg2"
						onClick={() => setMode('playlists')}>
						{t('tabs.playlists')}
					</button>
					{onSaveQueueAsPlaylist && queue.length > 0 && (
						<button
							className="text-dim hover:text-brand transition-colors p-2 rounded hover:bg-bg2"
							title={t('playlistPanel.saveQueueTitle')}
							onClick={() => {
								setSaveQueueName('');
								setSaveQueueDialog(true);
							}}>
							<ListPlus size={15} />
						</button>
					)}
					{queue.length > 0 && (
						<button
							className="text-sm text-dim hover:text-red hover:bg-bg2 transition-colors px-3 py-1.5 rounded pr-2!"
							onClick={onClear}
							title={t('queuePanel.clearTitle')}>
							✕ {t('queuePanel.clearButton')}
						</button>
					)}
				</PanelHeader>
				<PanelSearch value={query} onChange={setQuery} placeholder={t('queuePanel.searchPlaceholder')} />
				<PanelList>
					{queue.length === 0 ? (
						<EmptyState text={t('queuePanel.empty')} />
					) : canDrag ? (
						<DndContext sensors={sensors} collisionDetection={closestCenter} onDragEnd={handleDragEnd}>
							<SortableContext
								items={filtered.map(({ i }) => String(i))}
								strategy={verticalListSortingStrategy}>
								{filtered.map(({ track, i }) => (
									<SortableRow
										key={`${track.id}-${i}`}
										track={track}
										i={i}
										currentIdx={currentIdx}
										onJump={onJump}
										onRemove={onRemove}
									/>
								))}
							</SortableContext>
						</DndContext>
					) : (
						filtered.map(({ track, i }) => (
							<QueueRow
								key={`${track.id}-${i}`}
								track={track}
								i={i}
								currentIdx={currentIdx}
								onJump={onJump}
								onRemove={onRemove}
							/>
						))
					)}
				</PanelList>
			</Panel>
			{onCreatePlaylist && (
				<Dialog open={createDialog} onOpenChange={(o) => !o && setCreateDialog(false)}>
					<DialogContent showCloseButton={false}>
						<DialogHeader>
							<DialogTitle>{t('playlistPanel.createTitle')}</DialogTitle>
						</DialogHeader>
						<input
							className="w-full rounded border border-border bg-bg2 px-3 py-2 text-sm text-text placeholder:text-dim focus:outline-none focus:ring-1 focus:ring-brand"
							placeholder={t('playlistPanel.namePlaceholder')}
							value={createName}
							autoFocus
							onChange={(e) => setCreateName(e.target.value)}
							onKeyDown={(e) => {
								if (e.key === 'Enter' && createName.trim()) {
									onCreatePlaylist(createName.trim());
									setCreateDialog(false);
								}
								if (e.key === 'Escape') setCreateDialog(false);
							}}
						/>
						<DialogFooter>
							<button
								className="text-sm px-3 py-1.5 rounded bg-bg3 hover:bg-bg2 text-dim transition-colors"
								onClick={() => setCreateDialog(false)}>
								{t('playlistPanel.cancelButton')}
							</button>
							<button
								className="text-sm px-3 py-1.5 rounded bg-brand text-white hover:opacity-90 transition-opacity disabled:opacity-50"
								disabled={!createName.trim()}
								onClick={() => {
									onCreatePlaylist(createName.trim());
									setCreateDialog(false);
								}}>
								{t('playlistPanel.createButton')}
							</button>
						</DialogFooter>
					</DialogContent>
				</Dialog>
			)}
			{onSaveQueueAsPlaylist && (
				<Dialog open={saveQueueDialog} onOpenChange={(o) => !o && setSaveQueueDialog(false)}>
					<DialogContent showCloseButton={false}>
						<DialogHeader>
							<DialogTitle>{t('playlistPanel.saveQueueTitle')}</DialogTitle>
						</DialogHeader>
						<input
							className="w-full rounded border border-border bg-bg2 px-3 py-2 text-sm text-text placeholder:text-dim focus:outline-none focus:ring-1 focus:ring-brand"
							placeholder={t('playlistPanel.namePlaceholder')}
							value={saveQueueName}
							autoFocus
							onChange={(e) => setSaveQueueName(e.target.value)}
							onKeyDown={(e) => {
								if (e.key === 'Enter' && saveQueueName.trim()) {
									onSaveQueueAsPlaylist(saveQueueName.trim());
									setSaveQueueDialog(false);
								}
								if (e.key === 'Escape') setSaveQueueDialog(false);
							}}
						/>
						<DialogFooter>
							<button
								className="text-sm px-3 py-1.5 rounded bg-bg3 hover:bg-bg2 text-dim transition-colors"
								onClick={() => setSaveQueueDialog(false)}>
								{t('playlistPanel.cancelButton')}
							</button>
							<button
								className="text-sm px-3 py-1.5 rounded bg-brand text-white hover:opacity-90 transition-opacity disabled:opacity-50"
								disabled={!saveQueueName.trim()}
								onClick={() => {
									onSaveQueueAsPlaylist(saveQueueName.trim());
									setSaveQueueDialog(false);
								}}>
								{t('playlistPanel.saveQueue')}
							</button>
						</DialogFooter>
					</DialogContent>
				</Dialog>
			)}
		</>
	);
}
