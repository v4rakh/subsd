import { Toaster as Sonner, type ToasterProps } from 'sonner';

export function Toaster(props: ToasterProps) {
  return (
    <Sonner
      className="toaster group"
      toastOptions={{
        style: {
          background: 'var(--bg1)',
          color: 'var(--text)',
          borderColor: 'var(--border)',
          borderRadius: 'var(--radius)',
          fontSize: '11px',
        },
      }}
      {...props}
    />
  );
}
