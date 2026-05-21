import { useState, useEffect, useCallback } from 'react';
import { apiFetch } from '../api';

export interface AudioDevice {
	name: string;
	description: string;
}

export function useAudioDevice() {
	const [devices, setDevices] = useState<AudioDevice[]>([]);
	const [current, setCurrent] = useState<string>('');

	useEffect(() => {
		apiFetch('/api/v1/devices')
			.then((r) => r.json())
			.then((body: { devices: AudioDevice[]; current: string }) => {
				setDevices(body.devices ?? []);
				setCurrent(body.current ?? '');
			})
			.catch(() => {});
	}, []);

	const setDevice = useCallback((name: string) => {
		setCurrent(name);
		apiFetch('/api/v1/device', {
			method: 'POST',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify({ name })
		}).catch(() => {});
	}, []);

	return { devices, current, setDevice };
}
