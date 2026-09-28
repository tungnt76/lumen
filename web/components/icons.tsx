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
