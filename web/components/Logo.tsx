/** Lumen's mark: a play button with a sound wave (films and audio) on the amber tile.
 * The same drawing is app/icon.svg (favicon) and app/apple-icon.png. */
export function LogoMark({ size = 28 }: { size?: number }) {
  return (
    <svg className="logo-mark" width={size} height={size} viewBox="0 0 64 64" aria-hidden="true">
      <rect width="64" height="64" rx="14" fill="#E8A33D" />
      <path d="M19 19.5v25L37.5 32z" fill="#1A1206" stroke="#1A1206" strokeWidth="4" strokeLinejoin="round" />
      <path d="M43 21.5a14 14 0 0 1 0 21" fill="none" stroke="#1A1206" strokeWidth="4.5" strokeLinecap="round" />
    </svg>
  );
}
