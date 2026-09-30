// Stroke icons (no icon font, no emoji).
type P = { size?: number };
const base = (size = 20) => ({
  width: size, height: size, viewBox: "0 0 24 24", fill: "none", stroke: "currentColor",
  strokeWidth: 2, strokeLinecap: "round" as const, strokeLinejoin: "round" as const, "aria-hidden": true,
});

export const PlayIcon = ({ size = 18 }: P) => (
  <svg width={size} height={size} viewBox="0 0 24 24" aria-hidden="true"><polygon points="6 4 20 12 6 20 6 4" fill="currentColor" /></svg>
);
export const ChevronLeftIcon = ({ size = 22 }: P) => (<svg {...base(size)}><polyline points="15 18 9 12 15 6" /></svg>);
export const ChevronRightIcon = ({ size = 22 }: P) => (<svg {...base(size)}><polyline points="9 18 15 12 9 6" /></svg>);
export const SearchIcon =({ size }: P) => (<svg {...base(size)}><circle cx="11" cy="11" r="7" /><line x1="21" y1="21" x2="16.65" y2="16.65" /></svg>);
export const PlusIcon = ({ size }: P) => (<svg {...base(size)}><line x1="12" y1="5" x2="12" y2="19" /><line x1="5" y1="12" x2="19" y2="12" /></svg>);
export const CheckIcon = ({ size }: P) => (<svg {...base(size)}><polyline points="20 6 9 17 4 12" /></svg>);
export const InfoIcon = ({ size }: P) => (<svg {...base(size)}><circle cx="12" cy="12" r="9" /><line x1="12" y1="11" x2="12" y2="16" /><line x1="12" y1="8" x2="12" y2="8" /></svg>);
export const HomeIcon = ({ size }: P) => (<svg {...base(size)}><path d="M3 11l9-7 9 7v9a1 1 0 0 1-1 1h-5v-6h-6v6H4a1 1 0 0 1-1-1z" /></svg>);
export const BookmarkIcon = ({ size }: P) => (<svg {...base(size)}><path d="M6 3h12v18l-6-4-6 4z" /></svg>);
export const UploadIcon = ({ size }: P) => (<svg {...base(size)}><path d="M12 16V4" /><polyline points="7 9 12 4 17 9" /><path d="M4 16v3a1 1 0 0 0 1 1h14a1 1 0 0 0 1-1v-3" /></svg>);
export const BackIcon = ({ size }: P) => (<svg {...base(size)}><polyline points="15 18 9 12 15 6" /></svg>);
export const PauseIcon = ({ size = 18 }: P) => (
  <svg width={size} height={size} viewBox="0 0 24 24" aria-hidden="true"><rect x="6" y="4" width="4" height="16" rx="1" fill="currentColor" /><rect x="14" y="4" width="4" height="16" rx="1" fill="currentColor" /></svg>
);
export const BookIcon = ({ size }: P) => (<svg {...base(size)}><path d="M4 19V5a2 2 0 0 1 2-2h13v16H6a2 2 0 0 0-2 2z" /><path d="M4 19a2 2 0 0 1 2-2h13" /></svg>);
export const MusicIcon = ({ size }: P) => (<svg {...base(size)}><path d="M9 18V5l11-2v13" /><circle cx="6" cy="18" r="3" /><circle cx="17" cy="16" r="3" /></svg>);
export const FilmIcon = ({ size }: P) => (<svg {...base(size)}><rect x="3" y="4" width="18" height="16" rx="2" /><line x1="7" y1="4" x2="7" y2="20" /><line x1="17" y1="4" x2="17" y2="20" /><line x1="3" y1="12" x2="21" y2="12" /></svg>);
export const PrevIcon = ({ size }: P) => (<svg {...base(size)}><polygon points="19 20 9 12 19 4 19 20" fill="currentColor" /><line x1="5" y1="19" x2="5" y2="5" /></svg>);
export const NextIcon = ({ size }: P) => (<svg {...base(size)}><polygon points="5 4 15 12 5 20 5 4" fill="currentColor" /><line x1="19" y1="5" x2="19" y2="19" /></svg>);
export const Back15Icon = ({ size }: P) => (<svg {...base(size)}><path d="M3 12a9 9 0 1 0 3-6.7" /><polyline points="3 3 3 8 8 8" /><text x="12" y="15.5" fontSize="7.5" textAnchor="middle" fill="currentColor" stroke="none" fontWeight="700">15</text></svg>);
export const Fwd30Icon = ({ size }: P) => (<svg {...base(size)}><path d="M21 12a9 9 0 1 1-3-6.7" /><polyline points="21 3 21 8 16 8" /><text x="12" y="15.5" fontSize="7.5" textAnchor="middle" fill="currentColor" stroke="none" fontWeight="700">30</text></svg>);
export const CloseIcon = ({ size }: P) => (<svg {...base(size)}><line x1="6" y1="6" x2="18" y2="18" /><line x1="18" y1="6" x2="6" y2="18" /></svg>);
export const MoonIcon = ({ size }: P) => (<svg {...base(size)}><path d="M21 12.8A9 9 0 1 1 11.2 3a7 7 0 0 0 9.8 9.8z" /></svg>);
export const ExternalIcon = ({ size }: P) => (<svg {...base(size)}><path d="M14 4h6v6" /><line x1="20" y1="4" x2="11" y2="13" /><path d="M19 14v5a1 1 0 0 1-1 1H5a1 1 0 0 1-1-1V6a1 1 0 0 1 1-1h5" /></svg>);
export const SunIcon = ({ size }: P) => (<svg {...base(size)}><circle cx="12" cy="12" r="4" /><path d="M12 2v2M12 20v2M4.93 4.93l1.41 1.41M17.66 17.66l1.41 1.41M2 12h2M20 12h2M4.93 19.07l1.41-1.41M17.66 6.34l1.41-1.41" /></svg>);
export const PenIcon = ({ size }: P) => (<svg {...base(size)}><path d="M12 20h9" /><path d="M16.5 3.5a2.12 2.12 0 0 1 3 3L7 19l-4 1 1-4z" /></svg>);
export const CodeIcon = ({ size }: P) => (<svg {...base(size)}><polyline points="16 18 22 12 16 6" /><polyline points="8 6 2 12 8 18" /></svg>);
