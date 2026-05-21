import { useEffect, useRef, useState, useCallback } from 'react';
import { getApiUrl } from '../config';

const MIN_RETRY_MS = 1_000;
const MAX_RETRY_MS = 30_000;

export function useWebSocket<T>(onMessage: (data: T) => void): boolean {
	const [connected, setConnected] = useState(false);
	const wsRef = useRef<WebSocket | null>(null);
	const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
	const retryDelayRef = useRef(MIN_RETRY_MS);
	const onMessageRef = useRef(onMessage);
	onMessageRef.current = onMessage;

	const connect = useCallback(() => {
		const base = getApiUrl();
		let wsUrl: string;
		if (base) {
			const u = new URL(base);
			u.protocol = u.protocol === 'https:' ? 'wss:' : 'ws:';
			u.pathname = '/ws';
			wsUrl = u.toString();
		} else {
			const proto = location.protocol === 'https:' ? 'wss' : 'ws';
			wsUrl = `${proto}://${location.host}/ws`;
		}
		const ws = new WebSocket(wsUrl);
		wsRef.current = ws;

		ws.onopen = () => {
			setConnected(true);
			retryDelayRef.current = MIN_RETRY_MS;
		};

		ws.onmessage = (e: MessageEvent<string>) => {
			try {
				onMessageRef.current(JSON.parse(e.data) as T);
			} catch {
				// malformed message — ignore
			}
		};

		ws.onclose = () => {
			// Only act if this socket is still the active one. Guards against
			// StrictMode's double-invoke where the cleanup closes ws1 but
			// onclose fires after ws2 is already open — without this check,
			// the late ws1 onclose would flip connected=false while ws2 is
			// live, leaving the overlay stuck.
			if (wsRef.current === ws) {
				setConnected(false);
				const delay = retryDelayRef.current;
				retryDelayRef.current = Math.min(delay * 2, MAX_RETRY_MS);
				timerRef.current = setTimeout(connect, delay);
			}
		};
	}, []);

	useEffect(() => {
		connect();
		return () => {
			if (timerRef.current) clearTimeout(timerRef.current);
			wsRef.current?.close();
			// Null out after closing so any late-firing onclose (which fires
			// asynchronously after close()) sees wsRef.current !== ws and
			// does not schedule a reconnect timer on an unmounted component.
			wsRef.current = null;
		};
	}, [connect]);

	return connected;
}
