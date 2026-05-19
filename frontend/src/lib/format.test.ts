import { fmtDuration, fmtAudioMeta } from './format';
import { describe, it, expect } from 'vitest';

describe('fmtDuration', () => {
	it('formats 0 seconds', () => {
		expect(fmtDuration(0)).toBe('0:00');
	});

	it('formats seconds under a minute', () => {
		expect(fmtDuration(5)).toBe('0:05');
		expect(fmtDuration(59)).toBe('0:59');
	});

	it('formats exactly one minute', () => {
		expect(fmtDuration(60)).toBe('1:00');
	});

	it('formats minutes and seconds', () => {
		expect(fmtDuration(90)).toBe('1:30');
		expect(fmtDuration(125)).toBe('2:05');
	});

	it('formats over one hour', () => {
		expect(fmtDuration(3600)).toBe('60:00');
		expect(fmtDuration(3661)).toBe('61:01');
	});

	it('truncates fractional seconds', () => {
		expect(fmtDuration(59.9)).toBe('0:59');
		expect(fmtDuration(60.1)).toBe('1:00');
	});

	it('pads seconds with leading zero', () => {
		expect(fmtDuration(61)).toBe('1:01');
		expect(fmtDuration(609)).toBe('10:09');
	});
});

describe('fmtAudioMeta', () => {
	it('returns empty string for no fields', () => {
		expect(fmtAudioMeta({})).toBe('');
	});

	it('includes uppercased suffix', () => {
		expect(fmtAudioMeta({ suffix: 'flac' })).toBe('FLAC');
		expect(fmtAudioMeta({ suffix: 'mp3' })).toBe('MP3');
	});

	it('includes sampling rate in kHz', () => {
		expect(fmtAudioMeta({ samplingRate: 44100 })).toBe('44.1 kHz');
		expect(fmtAudioMeta({ samplingRate: 48000 })).toBe('48.0 kHz');
	});

	it('omits sampling rate when 0', () => {
		expect(fmtAudioMeta({ samplingRate: 0 })).toBe('');
	});

	it('includes bitrate in kbps', () => {
		expect(fmtAudioMeta({ bitRate: 320 })).toBe('320 kbps');
	});

	it('omits bitrate when 0', () => {
		expect(fmtAudioMeta({ bitRate: 0 })).toBe('');
	});

	it('includes mono label for channelCount=1', () => {
		expect(fmtAudioMeta({ channelCount: 1 })).toBe('mono');
	});

	it('does not include channel label for channelCount=2', () => {
		expect(fmtAudioMeta({ channelCount: 2 })).toBe('');
	});

	it('joins all fields with separator', () => {
		const result = fmtAudioMeta({
			suffix: 'flac',
			samplingRate: 44100,
			bitRate: 1000,
			channelCount: 1
		});
		expect(result).toBe('FLAC · 44.1 kHz · 1000 kbps · mono');
	});

	it('joins partial fields correctly', () => {
		const result = fmtAudioMeta({ suffix: 'mp3', bitRate: 320 });
		expect(result).toBe('MP3 · 320 kbps');
	});

	it('skips undefined fields', () => {
		const result = fmtAudioMeta({ suffix: 'aac', channelCount: 2 });
		expect(result).toBe('AAC');
	});
});
