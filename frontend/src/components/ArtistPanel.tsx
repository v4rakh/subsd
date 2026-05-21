import { Panel, PanelHeader, PanelList, PanelSearch, ListItem, EmptyState, SkeletonList } from './Panel';
import { apiUrl } from '../api';
import type { Artist } from '../types';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';

interface Props {
	artists: Artist[];
	selectedArtist: Artist | null;
	loading: boolean;
	onSelect: (artist: Artist) => void;
	className?: string;
}

export function ArtistPanel({ artists, selectedArtist, loading, onSelect, className }: Props) {
	const { t } = useTranslation();
	const [query, setQuery] = useState('');

	const filtered = query.trim() ? artists.filter((a) => a.name.toLowerCase().includes(query.toLowerCase())) : artists;

	return (
		<Panel className={className}>
			<PanelHeader title={t('artistPanel.title')} />
			<PanelSearch value={query} onChange={setQuery} placeholder={t('artistPanel.searchPlaceholder')} />
			<PanelList>
				{loading ? (
					<SkeletonList />
				) : filtered.length === 0 ? (
					<EmptyState text={t('artistPanel.empty')} />
				) : (
					filtered.map((a) => (
						<ListItem key={a.id} active={selectedArtist?.id === a.id} onClick={() => onSelect(a)}>
							{a.coverArt ? (
								<img
									className="w-10 h-10 rounded object-cover shrink-0"
									src={apiUrl(`/api/coverart/${a.coverArt}`)}
									alt=""
									loading="lazy"
									onError={(e) => {
										(e.target as HTMLImageElement).style.display = 'none';
									}}
								/>
							) : (
								<div className="w-10 h-10 rounded bg-bg3 shrink-0" />
							)}
							<div className="min-w-0 flex-1">
								<div className="truncate">{a.name}</div>
								{a.albumCount > 0 && (
									<div className="text-dim text-xs mt-0.5">
										{a.albumCount} {t('artistPanel.albums')}
									</div>
								)}
							</div>
						</ListItem>
					))
				)}
			</PanelList>
		</Panel>
	);
}
