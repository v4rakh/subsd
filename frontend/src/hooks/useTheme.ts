import { useEffect, useState } from 'react';

export type Theme = 'dark' | 'light';

function getInitial(): Theme {
	try {
		const stored = localStorage.getItem('theme');
		if (stored === 'dark' || stored === 'light') return stored;
	} catch {
		/* ignore */
	}
	return window.matchMedia?.('(prefers-color-scheme: light)').matches ? 'light' : 'dark';
}

export function useTheme() {
	const [theme, setTheme] = useState<Theme>(getInitial);

	useEffect(() => {
		document.documentElement.classList.toggle('dark', theme === 'dark');
		try {
			localStorage.setItem('theme', theme);
		} catch {
			/* ignore */
		}
	}, [theme]);

	const toggle = () => setTheme((t) => (t === 'dark' ? 'light' : 'dark'));
	return { theme, toggle };
}
