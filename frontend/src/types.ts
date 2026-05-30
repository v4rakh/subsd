// Domain types — mirror the JSON shapes from player/player.go and subsonic/client.go

// Scrobble outcome values (mirrors player.ScrobbleOK / ScrobbleError in Go).
export const SCROBBLE_OK = 'ok';
export const SCROBBLE_ERROR = 'error';
export const SCROBBLE_STATUSES = [SCROBBLE_OK, SCROBBLE_ERROR] as const;
export type ScrobbleStatus = (typeof SCROBBLE_STATUSES)[number];

// ReplayGain mode values (mirrors player.ReplayGainOff / Track / Album in Go).
export const REPLAY_GAIN_OFF = 'no';
export const REPLAY_GAIN_TRACK = 'track';
export const REPLAY_GAIN_ALBUM = 'album';
export const REPLAY_GAIN_MODES = [REPLAY_GAIN_OFF, REPLAY_GAIN_TRACK, REPLAY_GAIN_ALBUM] as const;
export type ReplayGainMode = (typeof REPLAY_GAIN_MODES)[number];

export interface Track {
	id: string;
	title: string;
	artist: string;
	album: string;
	duration: number;
	coverArt: string;
	streamUrl: string;
	suffix?: string;
	bitRate?: number;
	samplingRate?: number;
	channelCount?: number;
}

export interface PlayerState {
	playing: boolean;
	currentIdx: number;
	queue: Track[];
	position: number;
	duration: number;
	volume: number;
	shuffle: boolean;
	repeat: boolean;
	lastScrobble?: ScrobbleStatus;
	replayGain?: ReplayGainMode;
}

export interface Artist {
	id: string;
	name: string;
	albumCount: number;
	coverArt: string;
	album?: Album[];
}

export interface Album {
	id: string;
	name: string;
	artist: string;
	artistId: string;
	coverArt: string;
	year?: number;
	songCount: number;
	userRating?: number;
	song?: Song[];
}

export interface Song {
	id: string;
	title: string;
	artist: string;
	album: string;
	albumId: string;
	artistId: string;
	duration: number;
	track: number;
	coverArt: string;
	contentType: string;
	suffix?: string;
	bitRate?: number;
	samplingRate?: number;
	channelCount?: number;
	year?: number;
	genre?: string;
	size?: number;
	userRating?: number;
}

export interface SearchResult {
	artist?: Artist[];
	album?: Album[];
	song?: Song[];
}

export interface Playlist {
	id: string;
	name: string;
	songCount: number;
	coverArt: string;
	comment?: string;
	entry?: Song[];
}

export interface LyricLine {
	start: number;
	value: string;
}

export interface Lyrics {
	synced: boolean;
	lines: LyricLine[];
}

export interface Settings {
	lyricsEnabled: boolean;
}

export interface SatelliteDevice {
	id: string;
	name: string;
}

export interface SatelliteInfo {
	name: string;
	active: boolean;
	devices: SatelliteDevice[];
	activeDevice: string;
}

// WebSocket message union. All messages carry a `v` version field and an explicit `type`.
export type WsMessage =
	| (PlayerState & { v?: number; type: 'state' })
	| { v?: number; type: 'satellites'; satellites: SatelliteInfo[] }
	| { v?: number; type: 'satellite_disconnected'; name: string };
