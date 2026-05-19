// Shared formatting utilities for durations and audio metadata.

interface AudioMeta {
	suffix?: string;
	bitRate?: number;
	samplingRate?: number;
	channelCount?: number;
}

export function fmtDuration(seconds: number): string {
	const s = Math.floor(seconds);
	const m = Math.floor(s / 60);
	return `${m}:${String(s % 60).padStart(2, '0')}`;
}

export function fmtAudioMeta(t: AudioMeta): string {
	const parts: string[] = [];
	if (t.suffix) parts.push(t.suffix.toUpperCase());
	if (t.samplingRate && t.samplingRate > 0) parts.push(`${(t.samplingRate / 1000).toFixed(1)} kHz`);
	if (t.bitRate && t.bitRate > 0) parts.push(`${t.bitRate} kbps`);
	if (t.channelCount === 1) parts.push('mono');
	return parts.join(' · ');
}
