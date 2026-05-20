import { useAudioDevice, type AudioDevice } from '../hooks/useAudioDevice';
import type { PlayerControls } from '../hooks/usePlayer';
import type { Theme } from '../hooks/useTheme';
import type { Track, PlayerState } from '../types';
import { Button } from '@/components/ui/button';
import { Slider } from '@/components/ui/slider';
import { fmtDuration, fmtAudioMeta } from '@/lib/format';
import { cn } from '@/lib/utils';
import {
	Play,
	Pause,
	SkipBack,
	SkipForward,
	Shuffle,
	Repeat2,
	Sun,
	Moon,
	Volume2,
	SearchIcon,
	Headphones,
	Check,
	Radio
} from 'lucide-react';
import { useState, useEffect, useRef } from 'react';
import { useTranslation } from 'react-i18next';

function useIsDesktop() {
	const [isDesktop, setIsDesktop] = useState(() => window.innerWidth >= 1024);
	useEffect(() => {
		const mq = window.matchMedia('(min-width: 1024px)');
		const handler = (e: MediaQueryListEvent) => setIsDesktop(e.matches);
		mq.addEventListener('change', handler);
		return () => mq.removeEventListener('change', handler);
	}, []);
	return isDesktop;
}

interface Props {
	playerState: PlayerState;
	controls: PlayerControls;
	connected: boolean;
	theme: Theme;
	onThemeToggle: () => void;
	onOpenSearch: () => void;
}

function TrackInfo({ track }: { track: Track | null }) {
	return (
		<>
			{track?.coverArt ? (
				<img className="w-12 h-12 rounded object-cover shrink-0" src={track.coverArt} alt="" />
			) : (
				<div className="w-12 h-12 rounded bg-bg3 shrink-0" />
			)}
			<div className="min-w-0 flex-1 overflow-hidden">
				<div className="truncate text-bright leading-tight">{track?.title ?? '—'}</div>
				<div className="truncate text-sm text-dim mt-0.5">{track?.artist ?? ''}</div>
				{track && fmtAudioMeta(track) && (
					<div className="truncate text-xs text-dim mt-0.5">{fmtAudioMeta(track)}</div>
				)}
			</div>
		</>
	);
}

function ConnectionDot({ connected }: { connected: boolean }) {
	const { t } = useTranslation();
	return (
		<div className="relative group shrink-0 cursor-default p-2">
			<div className={cn('w-2.5 h-2.5 rounded-full', connected ? 'bg-green' : 'bg-red')} />
			<div className="pointer-events-none absolute bottom-full right-2 mb-1 px-2 py-1 rounded bg-bg3 border border-border text-xs text-text whitespace-nowrap shadow-sm opacity-0 group-hover:opacity-100 transition-opacity">
				{connected ? t('nowPlaying.connected') : t('nowPlaying.disconnected')}
			</div>
		</div>
	);
}

function ScrobbleDot({ lastScrobble }: { lastScrobble?: string }) {
	const { t } = useTranslation();
	if (!lastScrobble) return null;
	const ok = lastScrobble === 'ok';
	return (
		<div className="relative group shrink-0 cursor-default p-2">
			<Radio className={cn('size-3', ok ? 'text-green' : 'text-red')} />
			<div className="pointer-events-none absolute bottom-full right-2 mb-1 px-2 py-1 rounded bg-bg3 border border-border text-xs text-text whitespace-nowrap shadow-sm opacity-0 group-hover:opacity-100 transition-opacity">
				{ok ? t('nowPlaying.scrobbleOk') : t('nowPlaying.scrobbleError')}
			</div>
		</div>
	);
}

function AudioDevicePopover({
	devices,
	current,
	setDevice
}: {
	devices: AudioDevice[];
	current: string;
	setDevice: (name: string) => void;
}) {
	const { t } = useTranslation();
	const [open, setOpen] = useState(false);
	const ref = useRef<HTMLDivElement>(null);

	useEffect(() => {
		if (!open) return;
		const handler = (e: MouseEvent) => {
			if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false);
		};
		document.addEventListener('mousedown', handler);
		return () => document.removeEventListener('mousedown', handler);
	}, [open]);

	return (
		<div ref={ref} className="relative shrink-0">
			<Button
				variant="ghost"
				size="icon-sm"
				className={cn('text-dim', open && 'text-brand bg-muted')}
				onClick={() => setOpen((o) => !o)}
				title={t('nowPlaying.audioDevice')}>
				<Headphones className="size-4" />
			</Button>
			{open && (
				<div className="absolute bottom-full right-0 mb-2 min-w-[200px] max-w-[280px] bg-bg1 border border-border rounded-lg shadow-lg overflow-hidden z-50">
					{devices.map((d) => (
						<button
							key={d.name}
							className={cn(
								'w-full flex items-center gap-3 px-4 py-3 text-sm text-left hover:bg-bg2 transition-colors',
								current === d.name ? 'text-brand' : 'text-text'
							)}
							onClick={() => {
								setDevice(d.name);
								setOpen(false);
							}}>
							<span className="flex-1 truncate">{d.description || d.name}</span>
							{current === d.name && <Check className="size-3.5 shrink-0" />}
						</button>
					))}
				</div>
			)}
		</div>
	);
}

export function NowPlaying({ playerState, controls, connected, theme, onThemeToggle, onOpenSearch }: Props) {
	const { t } = useTranslation();
	const { playing, currentIdx, queue, position, duration, volume, shuffle, repeat, lastScrobble } = playerState;
	const track = queue[currentIdx] ?? null;
	const { devices, current, setDevice } = useAudioDevice();
	const isDesktop = useIsDesktop();

	return (
		<div className="bg-bg1 border-t border-border shrink-0 lg:pb-[env(safe-area-inset-bottom)]">
			{/* ── Desktop main bar ── */}
			<div className="hidden lg:grid grid-cols-[1fr_max-content_1fr] items-center px-6 py-5 gap-x-6 pr-0.5! pl-0.5!">
				{/* Track info — left column */}
				<div className="flex items-center gap-4 min-w-0 overflow-hidden pl-2!">
					<TrackInfo track={track} />
				</div>

				{/* Transport + seek + volume — center column */}
				<div className="flex flex-col items-center gap-2">
					<div className="flex items-center gap-1.5">
						<Button
							variant="ghost"
							size="icon-sm"
							className={cn('text-dim', shuffle && 'text-brand')}
							onClick={controls.shuffle}
							title={t('nowPlaying.shuffle')}>
							<Shuffle className="size-4" />
						</Button>
						<Button
							variant="ghost"
							size="icon-sm"
							className="text-dim"
							onClick={controls.prev}
							title={t('nowPlaying.previous')}>
							<SkipBack className="size-4" />
						</Button>
						<Button
							variant="ghost"
							size="icon"
							className="text-bright hover:text-brand"
							onClick={controls.playPause}
							title={playing ? t('nowPlaying.pause') : t('nowPlaying.play')}>
							{playing ? <Pause className="size-4" /> : <Play className="size-4" />}
						</Button>
						<Button
							variant="ghost"
							size="icon-sm"
							className="text-dim"
							onClick={controls.next}
							title={t('nowPlaying.next')}>
							<SkipForward className="size-4" />
						</Button>
						<Button
							variant="ghost"
							size="icon-sm"
							className={cn('text-dim', repeat && 'text-brand')}
							onClick={controls.repeat}
							title={t('nowPlaying.repeat')}>
							<Repeat2 className="size-4" />
						</Button>
					</div>
					<div className="flex items-center gap-2 w-80 xl:w-[34rem]">
						<span className="text-xs text-dim w-8 text-right tabular-nums shrink-0">
							{fmtDuration(position)}
						</span>
						<Slider
							className="flex-1 [&_[data-slot=slider-track]]:h-2"
							min={0}
							max={duration > 0 ? duration : 1}
							step={1}
							value={[position]}
							onValueChange={(v) => controls.seek(Array.isArray(v) ? v[0] : v)}
							aria-label={t('nowPlaying.seek')}
						/>
						<span className="text-xs text-dim w-8 tabular-nums shrink-0">{fmtDuration(duration)}</span>
					</div>
					<div className="flex items-center gap-2 w-44 xl:w-64">
						<Volume2 className="size-3.5 text-dim shrink-0" />
						<Slider
							key={`vol-desktop-${isDesktop}`}
							className="flex-1"
							min={0}
							max={100}
							step={1}
							value={[volume]}
							onValueChange={(v) => controls.setVolume(Array.isArray(v) ? v[0] : v)}
							aria-label={t('nowPlaying.volume')}
						/>
					</div>
				</div>

				{/* Audio device + search + theme + dot — right column */}
				<div className="flex items-center justify-end gap-2 pr-2!">
					{devices.length > 0 && (
						<AudioDevicePopover devices={devices} current={current} setDevice={setDevice} />
					)}
					<Button
						variant="ghost"
						size="icon-sm"
						className="text-dim shrink-0"
						onClick={onOpenSearch}
						title={t('searchPanel.title')}>
						<SearchIcon className="size-4" />
					</Button>
					<Button
						variant="ghost"
						size="icon-sm"
						className="text-dim shrink-0"
						onClick={onThemeToggle}
						title={theme === 'dark' ? t('nowPlaying.lightMode') : t('nowPlaying.darkMode')}>
						{theme === 'dark' ? <Sun className="size-4" /> : <Moon className="size-4" />}
					</Button>
					<ScrobbleDot lastScrobble={lastScrobble} />
					<ConnectionDot connected={connected} />
				</div>
			</div>

			{/* ── Mobile: track info row ── */}
			<div className="flex lg:hidden items-center gap-4 px-6 pt-6 pb-4 pl-2! pr-2!">
				<TrackInfo track={track} />
				<div className="flex items-center gap-1 shrink-0">
					{devices.length > 0 && (
						<AudioDevicePopover devices={devices} current={current} setDevice={setDevice} />
					)}
					<Button
						variant="ghost"
						size="icon-sm"
						className="text-dim"
						onClick={onOpenSearch}
						title={t('searchPanel.title')}>
						<SearchIcon className="size-4" />
					</Button>
					<Button
						variant="ghost"
						size="icon-sm"
						className="text-dim"
						onClick={onThemeToggle}
						title={theme === 'dark' ? t('nowPlaying.lightMode') : t('nowPlaying.darkMode')}>
						{theme === 'dark' ? <Sun className="size-4" /> : <Moon className="size-4" />}
					</Button>
					<ScrobbleDot lastScrobble={lastScrobble} />
					<ConnectionDot connected={connected} />
				</div>
			</div>

			{/* ── Mobile: controls row ── */}
			<div className="flex lg:hidden items-center justify-center gap-5 px-6 pb-4">
				<Button
					variant="ghost"
					size="icon-sm"
					className={cn('text-dim', shuffle && 'text-brand')}
					onClick={controls.shuffle}
					title={t('nowPlaying.shuffle')}>
					<Shuffle className="size-4" />
				</Button>
				<Button
					variant="ghost"
					size="icon-sm"
					className="text-dim"
					onClick={controls.prev}
					title={t('nowPlaying.previous')}>
					<SkipBack className="size-5" />
				</Button>
				<Button
					variant="ghost"
					size="icon"
					className="text-bright hover:text-brand size-12"
					onClick={controls.playPause}
					title={playing ? t('nowPlaying.pause') : t('nowPlaying.play')}>
					{playing ? <Pause className="size-6" /> : <Play className="size-6" />}
				</Button>
				<Button
					variant="ghost"
					size="icon-sm"
					className="text-dim"
					onClick={controls.next}
					title={t('nowPlaying.next')}>
					<SkipForward className="size-5" />
				</Button>
				<Button
					variant="ghost"
					size="icon-sm"
					className={cn('text-dim', repeat && 'text-brand')}
					onClick={controls.repeat}
					title={t('nowPlaying.repeat')}>
					<Repeat2 className="size-4" />
				</Button>
			</div>

			{/* ── Mobile: seek bar ── */}
			<div className="flex lg:hidden items-center gap-2 px-6 pb-5 pl-2! pr-2!">
				<span className="text-xs text-dim w-8 text-right tabular-nums shrink-0">{fmtDuration(position)}</span>
				<Slider
					className="flex-1 [&_[data-slot=slider-track]]:h-2"
					min={0}
					max={duration > 0 ? duration : 1}
					step={1}
					value={[position]}
					onValueChange={(v) => controls.seek(Array.isArray(v) ? v[0] : v)}
					aria-label={t('nowPlaying.seek')}
				/>
				<span className="text-xs text-dim w-8 tabular-nums shrink-0">{fmtDuration(duration)}</span>
			</div>

			{/* ── Mobile: volume ── */}
			<div className="flex lg:hidden items-center gap-2 px-6 pb-6 p-2!">
				<Volume2 className="size-4 text-dim shrink-0" />
				<Slider
					key={`vol-mobile-${isDesktop}`}
					className="flex-1"
					min={0}
					max={100}
					step={1}
					value={[volume]}
					onValueChange={(v) => controls.setVolume(Array.isArray(v) ? v[0] : v)}
					aria-label={t('nowPlaying.volume')}
				/>
			</div>
		</div>
	);
}
