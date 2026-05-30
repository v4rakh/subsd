import type { Playlist } from '../types';
import { Dialog, DialogContent, DialogHeader, DialogTitle } from './ui/dialog';
import { useTranslation } from 'react-i18next';

export function PlaylistPickerDialog({
	open,
	playlists,
	onPick,
	onClose
}: {
	open: boolean;
	playlists: Playlist[];
	onPick: (playlistId: string) => void;
	onClose: () => void;
}) {
	const { t } = useTranslation();
	return (
		<Dialog open={open} onOpenChange={(o) => !o && onClose()}>
			<DialogContent showCloseButton={false}>
				<DialogHeader>
					<DialogTitle>{t('playlistPanel.pickPlaylist')}</DialogTitle>
				</DialogHeader>
				<div className="flex flex-col max-h-64 overflow-y-auto divide-y divide-border rounded border border-border">
					{playlists.length === 0 ? (
						<p className="text-sm text-dim px-3 py-2">{t('playlistPanel.empty')}</p>
					) : (
						playlists.map((p) => (
							<button
								key={p.id}
								className="text-left px-3 py-2.5 hover:bg-bg2 text-sm transition-colors"
								onClick={() => {
									onPick(p.id);
									onClose();
								}}>
								{p.name}
							</button>
						))
					)}
				</div>
			</DialogContent>
		</Dialog>
	);
}
