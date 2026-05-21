import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { getApiUrl } from '../config';

interface Props {
	onSuccess: () => void;
}

export function LoginScreen({ onSuccess }: Props) {
	const { t } = useTranslation();
	const [token, setToken] = useState('');
	const [error, setError] = useState('');
	const [loading, setLoading] = useState(false);

	async function handleSubmit(e: React.FormEvent) {
		e.preventDefault();
		setError('');
		setLoading(true);
		try {
			const body = new URLSearchParams({ token });
			const r = await fetch(getApiUrl() + '/login', { method: 'POST', body, credentials: 'include' });
			if (r.ok) {
				onSuccess();
			} else {
				setError(t('login.wrongToken'));
			}
		} finally {
			setLoading(false);
		}
	}

	return (
		<div className="fixed inset-0 flex items-center justify-center bg-bg z-50">
			<form
				onSubmit={handleSubmit}
				className="flex flex-col gap-4 w-[min(320px,90vw)] p-8 bg-bg1 rounded-xl border border-border p-2!">
				<h1 className="text-sm font-semibold tracking-widest uppercase text-text font-[family-name:var(--font-ui)]">
					subsd
				</h1>
				<input
					type="password"
					className="px-3 py-2 rounded-md border border-border bg-bg text-text text-sm outline-none focus:border-brand font-[family-name:var(--font-ui)] placeholder:text-dim"
					placeholder={t('login.placeholder')}
					value={token}
					onChange={(e) => setToken(e.target.value)}
					autoFocus
				/>
				{error && <p className="text-xs text-red-400">{error}</p>}
				<button
					type="submit"
					disabled={loading}
					className="px-3 py-2 rounded-md border border-border bg-bg text-dim text-sm tracking-widest uppercase font-[family-name:var(--font-ui)] hover:text-text hover:bg-bg2 transition-colors disabled:opacity-50">
					{t('login.submit')}
				</button>
			</form>
		</div>
	);
}
