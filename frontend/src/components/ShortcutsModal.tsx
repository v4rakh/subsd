import { Dialog, DialogContent, DialogHeader, DialogTitle } from '@/components/ui/dialog';
import { useTranslation } from 'react-i18next';

interface Props {
	open: boolean;
	onClose: () => void;
}

export function ShortcutsModal({ open, onClose }: Props) {
	const { t } = useTranslation();

	const shortcuts: [string, string][] = [
		['Space', t('shortcuts.playPause')],
		['→', t('shortcuts.nextTrack')],
		['←', t('shortcuts.prevTrack')],
		['[', t('shortcuts.volumeDown')],
		[']', t('shortcuts.volumeUp')],
		['?', t('shortcuts.showShortcuts')],
		['/', t('shortcuts.search')]
	];

	return (
		<Dialog
			open={open}
			onOpenChange={(o) => {
				if (!o) onClose();
			}}>
			<DialogContent className="bg-bg1 border-border text-text sm:max-w-xs">
				<DialogHeader>
					<DialogTitle className="label-ui">{t('shortcuts.title')}</DialogTitle>
				</DialogHeader>
				<table className="w-full">
					<tbody>
						{shortcuts.map(([key, desc]) => (
							<tr key={key} className="border-b border-border last:border-0">
								<td className="py-1.5 pr-6 whitespace-nowrap">
									<kbd className="px-1.5 py-0.5 bg-bg3 border border-border rounded text-bright font-mono">
										{key}
									</kbd>
								</td>
								<td className="py-1.5 text-dim">{desc}</td>
							</tr>
						))}
					</tbody>
				</table>
			</DialogContent>
		</Dialog>
	);
}
