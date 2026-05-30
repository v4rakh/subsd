import { apiFetch } from './api';
import { AlbumPanel } from './components/AlbumPanel';
import { ArtistPanel } from './components/ArtistPanel';
import { NowPlaying } from './components/NowPlaying';
import { Panel, PanelHeader } from './components/Panel';
import { PlaylistBrowser } from './components/PlaylistBrowser';
import { QueuePanel } from './components/QueuePanel';
import { SearchPanel } from './components/SearchPanel';
import { ShortcutsModal } from './components/ShortcutsModal';
import { TrackPanel } from './components/TrackPanel';
import { Dialog, DialogContent } from './components/ui/dialog';
import { Toaster } from './components/ui/sonner';
import { useLibrary } from './hooks/useLibrary';
import { usePlayer } from './hooks/usePlayer';
import { useTheme } from './hooks/useTheme';
import type { Artist, Album } from './types';
import { cn } from '@/lib/utils';
import { Play } from 'lucide-react';
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';

type MobileTab = 'artists' | 'albums' | 'tracks' | 'queue' | 'search' | 'playlists';

export default function App() {
	const { t } = useTranslation();
	const { playerState, controls, connected, satellites } = usePlayer();
	const lib = useLibrary();
	const { theme, toggle: toggleTheme } = useTheme();
	const [showShortcuts, setShowShortcuts] = useState(false);
	const [showSearch, setShowSearch] = useState(false);
	const [mobileTab, setMobileTab] = useState<MobileTab>('artists');
	const [showOverlay, setShowOverlay] = useState(false);

	const TABS: { id: MobileTab; label: string }[] = useMemo(
		() => [
			{ id: 'artists', label: t('tabs.artists') },
			{ id: 'albums', label: t('tabs.albums') },
			{ id: 'tracks', label: t('tabs.tracks') },
			{ id: 'queue', label: t('tabs.queue') },
			{ id: 'search', label: t('tabs.search') },
			{ id: 'playlists', label: t('tabs.playlists') }
		],
		[t]
	);

	useEffect(() => {
		lib.loadArtists();
		lib.loadPlaylists();
	}, [lib.loadArtists, lib.loadPlaylists]);

	// Only show the connecting overlay if the WS hasn't connected within 300 ms.
	// This avoids a flash on fast LAN connections and prevents the overlay from
	// appearing at all during rapid refreshes in dev mode.
	useEffect(() => {
		if (connected) {
			setShowOverlay(false);
			return;
		}
		const id = setTimeout(() => setShowOverlay(true), 300);
		return () => clearTimeout(id);
	}, [connected]);

	// Reconnect notification — only show after a successful connection was established
	const wasConnected = useRef(false);
	useEffect(() => {
		if (connected) {
			wasConnected.current = true;
			toast.dismiss('ws');
		} else if (wasConnected.current) {
			toast.error(t('toast.disconnected'), { id: 'ws', duration: Infinity });
		}
	}, [connected, t]);

	useEffect(() => {
		function onKey(e: KeyboardEvent) {
			const target = e.target as HTMLElement;
			const isInput = target.tagName === 'INPUT' || target.tagName === 'TEXTAREA';

			// ESC always blurs focused inputs so the user can get back to global shortcuts
			if (e.key === 'Escape') {
				if (isInput) {
					target.blur();
					e.preventDefault();
				}
				return;
			}

			if (isInput) return;

			if (e.code === 'Space') {
				e.preventDefault();
				controls.playPause();
			}
			if (e.code === 'ArrowRight') controls.next();
			if (e.code === 'ArrowLeft') controls.prev();
			if (e.key === '[') controls.setVolume(Math.max(0, playerState.volume - 5));
			if (e.key === ']') controls.setVolume(Math.min(100, playerState.volume + 5));
			if (e.key === '?') setShowShortcuts((s) => !s);
			if (e.key === '/') {
				e.preventDefault();
				if (window.innerWidth >= 1024) {
					setShowSearch((s) => !s);
				} else {
					setMobileTab('search');
				}
			}
		}
		window.addEventListener('keydown', onKey);
		return () => window.removeEventListener('keydown', onKey);
	}, [controls, playerState.volume]);

	const handleSelectArtist = useCallback(
		(artist: Artist) => {
			lib.selectArtist(artist);
			setMobileTab('albums');
			setShowSearch(false);
		},
		[lib.selectArtist]
	);

	const handleSelectAlbum = useCallback(
		(album: Album) => {
			lib.selectAlbum(album);
			setMobileTab('tracks');
			setShowSearch(false);
		},
		[lib.selectAlbum]
	);

	const searchProps = {
		onSelectArtist: handleSelectArtist,
		onSelectAlbum: handleSelectAlbum,
		onPlayAlbum: controls.playAlbum,
		onEnqueueAlbum: controls.enqueueAlbum,
		onPlaySong: controls.playSong,
		onEnqueueSong: controls.enqueueSong
	};

	const playlistBrowserProps = {
		currentTrackId: playerState.queue[playerState.currentIdx]?.id,
		playlists: lib.playlists,
		selectedPlaylist: lib.selectedPlaylist,
		playlistSongs: lib.playlistSongs,
		loadingPlaylists: lib.loading.playlists,
		loadingPlaylistTracks: lib.loading.playlistTracks,
		onSelectPlaylist: lib.selectPlaylist,
		onPlayPlaylist: controls.playPlaylist,
		onEnqueuePlaylist: controls.enqueuePlaylist,
		onPlaySong: controls.playSong,
		onEnqueueSong: controls.enqueueSong
	};

	return (
		<div className="relative flex flex-col h-full">
			{showOverlay && (
				<div className="absolute inset-0 z-50 backdrop-blur-sm bg-bg/40 flex items-center justify-center">
					<span className="label-ui text-dim">{t('connecting')}</span>
				</div>
			)}

			{/* Panels */}
			<div className="flex flex-1 overflow-hidden">
				<ArtistPanel
					className={cn('flex-1 lg:[flex-grow:0.7] lg:min-w-[180px]', mobileTab !== 'artists' && 'hidden lg:flex')}
					artists={lib.artists}
					selectedArtist={lib.selectedArtist}
					loading={lib.loading.artists}
					onSelect={handleSelectArtist}
				/>
				<AlbumPanel
					className={cn('flex-1 min-w-[220px]', mobileTab !== 'albums' && 'hidden lg:flex')}
					albums={lib.albums}
					selectedArtist={lib.selectedArtist}
					selectedAlbum={lib.selectedAlbum}
					loading={lib.loading.albums}
					onSelect={handleSelectAlbum}
					onPlay={controls.playAlbum}
					onEnqueue={controls.enqueueAlbum}
					onRateAlbum={lib.setAlbumRating}
					onBack={() => setMobileTab('artists')}
				/>
				<TrackPanel
					className={cn('flex-1 min-w-[260px]', mobileTab !== 'tracks' && 'hidden lg:flex')}
					songs={lib.songs}
					selectedAlbum={lib.selectedAlbum}
					playerState={playerState}
					loading={lib.loading.tracks}
					onPlayAlbum={controls.playAlbum}
					onEnqueueAlbum={controls.enqueueAlbum}
					onPlaySong={controls.playSong}
					onEnqueueSong={controls.enqueueSong}
					onRateSong={lib.setSongRating}
					onBack={() => setMobileTab('albums')}
				/>
				<QueuePanel
					className={cn('flex-1 [flex-grow:1.3] min-w-[220px] border-r-0', mobileTab !== 'queue' && 'hidden lg:flex')}
					playerState={playerState}
					onJump={controls.jumpTo}
					onRemove={controls.removeTrack}
					onClear={controls.clearQueue}
					onMove={controls.moveTrack}
					{...playlistBrowserProps}
					onClearPlaylist={lib.clearSelectedPlaylist}
					onPlayPlaylist={controls.playPlaylist}
					onEnqueuePlaylist={controls.enqueuePlaylist}
				/>
				{/* Mobile-only search panel */}
				<SearchPanel className={cn('flex-1 lg:hidden', mobileTab !== 'search' && 'hidden')} {...searchProps} />
				{/* Mobile-only playlists panel */}
				<Panel className={cn('flex-1 lg:hidden', mobileTab !== 'playlists' && 'hidden')}>
					<PanelHeader
						title={lib.selectedPlaylist ? lib.selectedPlaylist.name : t('playlistPanel.title')}
						backLabel={lib.selectedPlaylist ? t('playlistPanel.title') : undefined}
						onBack={lib.selectedPlaylist ? lib.clearSelectedPlaylist : undefined}>
						{lib.selectedPlaylist && (
							<>
								<button
									className="flex items-center gap-1 text-sm text-dim hover:text-brand transition-colors px-2 py-1.5 rounded hover:bg-bg2 shrink-0 pr-2!"
									onClick={() => controls.playPlaylist(lib.selectedPlaylist?.id ?? '')}>
									<Play size={13} />
									{t('playlistPanel.playAll')}
								</button>
								<button
									className="text-sm text-dim hover:text-brand transition-colors px-2 py-1.5 rounded hover:bg-bg2 shrink-0 pr-2!"
									onClick={() => controls.enqueuePlaylist(lib.selectedPlaylist?.id ?? '')}>
									{t('playlistPanel.queueAll')}
								</button>
							</>
						)}
					</PanelHeader>
					<PlaylistBrowser {...playlistBrowserProps} />
				</Panel>
			</div>

			<NowPlaying
				playerState={playerState}
				controls={controls}
				connected={connected}
				theme={theme}
				satellites={satellites}
				onThemeToggle={toggleTheme}
				onOpenSearch={() => {
					if (window.innerWidth >= 1024) setShowSearch(true);
					else setMobileTab('search');
				}}
				onRefreshCache={() => apiFetch('/api/v1/cache', { method: 'POST' }).catch(() => { /* empty */ })}
				setReplayGain={controls.setReplayGain}
			/>

			{/* Mobile tab bar */}
			<nav className="flex lg:hidden border-t border-border bg-bg1 shrink-0 pb-[env(safe-area-inset-bottom)]">
				{TABS.map((tab) => (
					<button
						key={tab.id}
						className={cn(
							'flex-1 flex items-center justify-center py-4 text-[0.65rem] font-semibold tracking-widest uppercase transition-colors',
							mobileTab === tab.id ? 'text-brand' : 'text-dim'
						)}
						onClick={() => setMobileTab(tab.id)}>
						<span className={cn('px-3 py-1.5 rounded-full transition-colors', mobileTab === tab.id && 'bg-brand/10')}>
							{tab.label}
						</span>
					</button>
				))}
			</nav>

			<ShortcutsModal open={showShortcuts} onClose={() => setShowShortcuts(false)} />

			{/* Desktop search dialog */}
			<Dialog
				open={showSearch}
				onOpenChange={(o) => {
					if (!o) setShowSearch(false);
				}}>
				<DialogContent
					className="bg-bg1 border-border text-text max-w-2xl sm:max-w-3xl p-0! gap-0 overflow-hidden flex flex-col max-h-[80vh]"
					showCloseButton={false}>
					<SearchPanel embedded {...searchProps} />
				</DialogContent>
			</Dialog>

			<Toaster theme={theme} position="bottom-right" />
		</div>
	);
}
