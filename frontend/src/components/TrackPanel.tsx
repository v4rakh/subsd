import { Panel, PanelHeader, PanelList, PanelSearch, ListItem, EmptyState, SkeletonList, StarRating } from './Panel';
import type { Album, Song, PlayerState } from '../types';
import { fmtDuration, fmtAudioMeta } from '@/lib/format';
import { PlusIcon, Play } from 'lucide-react';
import { useEffect, useState, type MouseEvent } from 'react';
import { useTranslation } from 'react-i18next';

interface Props {
	songs: Song[];
	selectedAlbum: Album | null;
	playerState: PlayerState;
	loading: boolean;
	onPlayAlbum: (id: string) => void;
	onEnqueueAlbum: (id: string) => void;
	onPlaySong: (id: string) => void;
	onEnqueueSong: (id: string) => void;
	onRateSong: (id: string, rating: number) => void;
	onBack?: () => void;
	className?: string;
}

export function TrackPanel({
	songs,
	selectedAlbum,
	playerState,
	loading,
	onPlayAlbum,
	onEnqueueAlbum,
	onPlaySong,
	onEnqueueSong,
	onRateSong,
	onBack,
	className
}: Props) {
	const { t } = useTranslation();
	const [query, setQuery] = useState('');
	const [selected, setSelected] = useState<Set<string>>(new Set());
	const [lastClickedId, setLastClickedId] = useState<string | null>(null);
	const currentId = playerState.queue[playerState.currentIdx]?.id;

	useEffect(() => {
		setSelected(new Set());
		setLastClickedId(null);
	}, [selectedAlbum?.id]);

	useEffect(() => {
		setLastClickedId(null);
	}, [query]);

	useEffect(() => {
		if (selected.size === 0) return;
		function onKey(e: KeyboardEvent) {
			if (e.key === 'Escape') {
				setSelected(new Set());
				setLastClickedId(null);
			}
		}
		window.addEventListener('keydown', onKey);
		return () => window.removeEventListener('keydown', onKey);
	}, [selected.size]);

	const filtered = query.trim()
		? songs.filter(
				(s) =>
					s.title.toLowerCase().includes(query.toLowerCase()) ||
					(s.artist ?? '').toLowerCase().includes(query.toLowerCase())
			)
		: songs;

	function handleRowClick(e: MouseEvent<HTMLDivElement>, song: Song) {
		if (e.shiftKey && lastClickedId) {
			const ids = filtered.map((s) => s.id);
			const lastIdx = ids.indexOf(lastClickedId);
			const thisIdx = ids.indexOf(song.id);
			const [from, to] = [Math.min(lastIdx, thisIdx), Math.max(lastIdx, thisIdx)];
			setSelected((prev) => new Set([...prev, ...ids.slice(from, to + 1)]));
		} else if (e.ctrlKey || e.metaKey) {
			setSelected((prev) => {
				const next = new Set(prev);
				if (next.has(song.id)) next.delete(song.id);
				else next.add(song.id);
				return next;
			});
			setLastClickedId(song.id);
		} else {
			onEnqueueSong(song.id);
		}
	}

	function enqueueSelected() {
		filtered.filter((s) => selected.has(s.id)).forEach((s) => onEnqueueSong(s.id));
		setSelected(new Set());
		setLastClickedId(null);
	}

	return (
		<Panel className={className}>
			<PanelHeader
				title={selectedAlbum?.name ?? t('trackPanel.defaultTitle')}
				backLabel={t('tabs.albums')}
				onBack={onBack}>
				{selectedAlbum && (
					<div className="flex items-center gap-1">
						<button
							className="text-sm text-dim hover:text-brand hover:bg-bg2 transition-colors px-3 py-1.5 rounded pr-2!"
							onClick={() => onPlayAlbum(selectedAlbum.id)}
							title={t('trackPanel.playTitle')}>
							▶ {t('trackPanel.playButton')}
						</button>
						<button
							className="text-sm text-dim hover:text-brand hover:bg-bg2 transition-colors px-3 py-1.5 rounded pr-2!"
							onClick={() => (selected.size > 0 ? enqueueSelected() : onEnqueueAlbum(selectedAlbum.id))}
							title={
								selected.size > 0 ? t('trackPanel.queueTrackTitle') : t('trackPanel.queueAlbumTitle')
							}>
							{selected.size > 0
								? t('trackPanel.enqueueSelected', { count: selected.size })
								: t('trackPanel.queueButton')}
						</button>
					</div>
				)}
			</PanelHeader>
			<PanelSearch value={query} onChange={setQuery} placeholder={t('trackPanel.searchPlaceholder')} />
			<PanelList>
				{loading ? (
					<SkeletonList rows={10} />
				) : filtered.length === 0 ? (
					<EmptyState text={selectedAlbum ? t('trackPanel.noTracks') : t('trackPanel.selectAlbum')} />
				) : (
					filtered.map((s) => (
						<ListItem
							key={s.id}
							playing={s.id === currentId}
							selected={selected.has(s.id)}
							onClick={(e) => handleRowClick(e, s)}
							onDoubleClick={() => onPlaySong(s.id)}>
							<span className="w-6 shrink-0 text-right text-dim">{s.track || '–'}</span>
							<div className="flex-1 min-w-0">
								<div className="truncate">{s.title}</div>
								{fmtAudioMeta(s) && (
									<div className="truncate text-xs text-dim mt-0.5">{fmtAudioMeta(s)}</div>
								)}
							</div>
							<StarRating rating={s.userRating ?? 0} onRate={(r) => onRateSong(s.id, r)} />
							<button
								className="shrink-0 lg:opacity-0 lg:group-hover:opacity-100 text-dim hover:text-brand transition-opacity p-2.5 rounded hover:bg-bg3"
								title={t('trackPanel.playTitle')}
								onClick={(e) => {
									e.stopPropagation();
									onPlaySong(s.id);
								}}>
								<Play size={13} />
							</button>
							<button
								className="shrink-0 lg:opacity-0 lg:group-hover:opacity-100 text-dim hover:text-brand transition-opacity p-2.5 rounded hover:bg-bg3"
								title={t('trackPanel.queueTrackTitle')}
								onClick={(e) => {
									e.stopPropagation();
									onEnqueueSong(s.id);
								}}>
								<PlusIcon size={14} />
							</button>
							<span className="text-dim shrink-0">{fmtDuration(s.duration)}</span>
						</ListItem>
					))
				)}
			</PanelList>
			{selected.size > 0 && (
				<div className="shrink-0 border-t border-border bg-bg1 px-6 py-3 flex items-center justify-between gap-4">
					<span className="text-sm text-dim">{t('trackPanel.selectedCount', { count: selected.size })}</span>
					<button
						className="text-sm text-dim hover:text-text transition-colors px-3 py-1.5 rounded hover:bg-bg2"
						onClick={() => {
							setSelected(new Set());
							setLastClickedId(null);
						}}>
						{t('trackPanel.clearSelection')}
					</button>
				</div>
			)}
		</Panel>
	);
}
