import { cn } from '@/lib/utils';
import { XIcon, ChevronLeft } from 'lucide-react';
import type { MouseEvent, ReactNode } from 'react';

interface PanelProps {
	children: ReactNode;
	className?: string;
}
export function Panel({ children, className }: PanelProps) {
	return (
		<div className={cn('flex flex-col lg:border-r border-border overflow-hidden bg-bg', className)}>{children}</div>
	);
}

interface PanelHeaderProps {
	title: string;
	children?: ReactNode;
	backLabel?: string;
	onBack?: () => void;
}
export function PanelHeader({ title, children, backLabel, onBack }: PanelHeaderProps) {
	return (
		<div className="flex items-center justify-between gap-2 px-4 h-10 border-b border-border shrink-0">
			<div className="flex items-center gap-2 min-w-0">
				{onBack && backLabel && (
					<button
						className="lg:hidden flex items-center gap-0.5 text-xs text-dim hover:text-brand transition-colors shrink-0 py-1 pr-2 border-r border-border mr-1"
						onClick={onBack}>
						<ChevronLeft size={14} />
						{backLabel}
					</button>
				)}
				<span className="label-ui truncate">{title}</span>
			</div>
			{children}
		</div>
	);
}

interface PanelListProps {
	children: ReactNode;
}
export function PanelList({ children }: PanelListProps) {
	return <div className="flex-1 overflow-y-auto">{children}</div>;
}

interface ListItemProps {
	active?: boolean;
	playing?: boolean;
	selected?: boolean;
	onClick?: (e: MouseEvent<HTMLDivElement>) => void;
	onDoubleClick?: () => void;
	children: ReactNode;
}
export function ListItem({ active, playing, selected, onClick, onDoubleClick, children }: ListItemProps) {
	return (
		<div
			className={cn(
				'group flex items-center gap-3 px-6 py-5 cursor-pointer transition-colors duration-75 select-none',
				active && 'bg-bg3 text-bright',
				playing && 'text-green',
				selected && !active && 'bg-brand/10',
				!active && !selected && 'hover:bg-bg2'
			)}
			onClick={onClick}
			onDoubleClick={onDoubleClick}>
			{children}
		</div>
	);
}

interface PanelSearchProps {
	value: string;
	onChange: (v: string) => void;
	placeholder?: string;
	autoFocus?: boolean;
}
export function PanelSearch({ value, onChange, placeholder, autoFocus }: PanelSearchProps) {
	return (
		<div className="relative flex items-center px-6 py-4 bg-bg1 border-b border-border shrink-0 p-1.5!">
			<input
				className="w-full bg-bg2 border border-border rounded-md px-4 py-3 pr-9 outline-none focus:border-brand focus:ring-2 focus:ring-brand/30 transition-colors placeholder:text-dim"
				type="text"
				value={value}
				onChange={(e) => onChange(e.target.value)}
				placeholder={placeholder}
				spellCheck={false}
				autoFocus={autoFocus}
			/>
			{value && (
				<button
					className="absolute right-7 p-1.5 text-dim hover:text-text transition-colors"
					onClick={() => onChange('')}
					tabIndex={-1}>
					<XIcon size={13} />
				</button>
			)}
		</div>
	);
}

export function EmptyState({ text }: { text: string }) {
	return <div className="px-6 py-10 text-dim text-center">{text}</div>;
}

export function SkeletonList({ rows = 8 }: { rows?: number }) {
	return (
		<>
			{Array.from({ length: rows }).map((_, i) => (
				<div key={i} className="flex items-center gap-3 px-6 py-5">
					<div className="w-10 h-10 rounded bg-bg2 animate-pulse shrink-0" />
					<div className="flex flex-col gap-2 flex-1">
						<div
							className="h-3 rounded bg-bg2 animate-pulse"
							style={{ width: `${50 + ((i * 13) % 35)}%` }}
						/>
						<div
							className="h-2.5 rounded bg-bg2 animate-pulse"
							style={{ width: `${25 + ((i * 17) % 25)}%` }}
						/>
					</div>
				</div>
			))}
		</>
	);
}
