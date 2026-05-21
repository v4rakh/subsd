// Domain types — mirror the JSON shapes from player/player.go and subsonic/client.go

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
	lastScrobble?: string; // "", "ok", or "error"
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

// WebSocket message union. All messages carry a `v` version field.
// Player state messages have no `type` field; satellite list messages carry `type: "satellites"`.
export type WsMessage =
	| (PlayerState & { v?: number; type?: undefined })
	| { v?: number; type: 'satellites'; satellites: SatelliteInfo[] }
	| { v?: number; type: 'satellite_disconnected'; name: string };
