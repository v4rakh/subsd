import { AUTH_FAILURE_EVENT } from './api';
import App from './App';
import { LoginScreen } from './components/LoginScreen';
import { loadConfig } from './config';
import React, { useEffect, useState } from 'react';
import ReactDOM from 'react-dom/client';
import './i18n';
import './index.css';

// Apply theme before first render to avoid flash
(function () {
	try {
		const stored = localStorage.getItem('theme');
		const preferLight = window.matchMedia?.('(prefers-color-scheme: light)').matches;
		const dark = stored ? stored === 'dark' : !preferLight;
		if (dark) document.documentElement.classList.add('dark');
	} catch {
		/* ignore */
	}
})();

function Root() {
	const [needsLogin, setNeedsLogin] = useState(false);

	useEffect(() => {
		const handler = () => setNeedsLogin(true);
		window.addEventListener(AUTH_FAILURE_EVENT, handler);
		return () => window.removeEventListener(AUTH_FAILURE_EVENT, handler);
	}, []);

	if (needsLogin) {
		return <LoginScreen onSuccess={() => setNeedsLogin(false)} />;
	}
	return <App />;
}

const rootEl = document.getElementById('root');
if (!rootEl) throw new Error('Root element not found');
loadConfig().then(() => {
	ReactDOM.createRoot(rootEl).render(
		<React.StrictMode>
			<Root />
		</React.StrictMode>
	);
});
