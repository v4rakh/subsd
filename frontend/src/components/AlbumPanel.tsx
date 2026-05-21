import { Panel, PanelHeader, PanelList, PanelSearch, ListItem, EmptyState, SkeletonList } from './Panel';
import { apiUrl } from '../api';
import type { Artist, Album } from '../types';
import { PlusIcon, Play } from 'lucide-react';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';

interface Props {
	albums: Album[];
	selectedArtist: Artist | null;
	selectedAlbum: Album | null;
	loading: boolean;
	onSelect: (album: Album) => void;
	onPlay: (id: string) => void;
	onEnqueue: (id: string) => void;
	onBack?: () => void;
	className?: string;
}

export function AlbumPanel({
	albums,
	selectedArtist,
	selectedAlbum,
	loading,
	onSelect,
	onPlay,
	onEnqueue,
	onBack,
	className
}: Props) {
	const { t } = useTranslation();
	const [query, setQuery] = useState('');

	const filtered = query.trim() ? albums.filter((a) => a.name.toLowerCase().includes(query.toLowerCase())) : albums;

	return (
		<Panel className={className}>
			<PanelHeader
				title={selectedArtist?.name ?? t('albumPanel.defaultTitle')}
				backLabel={t('tabs.artists')}
				onBack={onBack}
			/>
			<PanelSearch value={query} onChange={setQuery} placeholder={t('albumPanel.searchPlaceholder')} />
			<PanelList>
				{loading ? (
					<SkeletonList />
				) : filtered.length === 0 ? (
					<EmptyState text={selectedArtist ? t('albumPanel.noAlbums') : t('albumPanel.selectArtist')} />
				) : (
					filtered.map((a) => (
						<ListItem
							key={a.id}
							active={selectedAlbum?.id === a.id}
							onClick={() => onSelect(a)}
							onDoubleClick={() => onPlay(a.id)}>
							{a.coverArt && (
								<img
									className="w-10 h-10 rounded object-cover shrink-0"
									src={apiUrl(`/api/v1/coverart/${a.coverArt}`)}
									alt=""
									loading="lazy"
									onError={(e) => {
										(e.target as HTMLImageElement).style.display = 'none';
									}}
								/>
							)}
							<div className="min-w-0 flex-1">
								<div className="truncate">{a.name}</div>
								{a.year != null && <div className="text-dim">{a.year}</div>}
							</div>
							<button
								className="shrink-0 lg:opacity-0 lg:group-hover:opacity-100 text-dim hover:text-brand transition-opacity p-2.5 rounded hover:bg-bg3 pr-0.5!"
								title={t('albumPanel.playTitle')}
								onClick={(e) => {
									e.stopPropagation();
									onPlay(a.id);
								}}>
								<Play size={14} />
							</button>
							<button
								className="shrink-0 lg:opacity-0 lg:group-hover:opacity-100 text-dim hover:text-brand transition-opacity p-2.5 rounded hover:bg-bg3 pr-0.5!"
								title={t('albumPanel.enqueueTitle')}
								onClick={(e) => {
									e.stopPropagation();
									onEnqueue(a.id);
								}}>
								<PlusIcon size={16} />
							</button>
						</ListItem>
					))
				)}
			</PanelList>
		</Panel>
	);
}
