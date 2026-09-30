"use client";

import { useId } from "react";
import { toneFor } from "@/lib/api";

/** Preset avatars: a white line icon on a two-colour gradient. Keep ids in step with the API. */
export const AVATARS: { id: string; label: string; from: string; to: string; glyph: React.ReactNode }[] = [
  { id: "film", label: "Film", from: "#f6b04d", to: "#d9591e", glyph: <><rect x="3" y="5" width="18" height="14" rx="2" /><path d="M7 5v14M17 5v14M3 12h18" /></> },
  { id: "headphones", label: "Headphones", from: "#6d8bff", to: "#4b2fd6", glyph: <><path d="M4 15v-3a8 8 0 0 1 16 0v3" /><rect x="3" y="14" width="4" height="6" rx="1.5" /><rect x="17" y="14" width="4" height="6" rx="1.5" /></> },
  { id: "book", label: "Book", from: "#34c7a5", to: "#137a6d", glyph: <><path d="M2 5h6a4 4 0 0 1 4 4v11a3 3 0 0 0-3-3H2z" /><path d="M22 5h-6a4 4 0 0 0-4 4v11a3 3 0 0 1 3-3h7z" /></> },
  { id: "music", label: "Music", from: "#ff7ab8", to: "#c1307a", glyph: <><path d="M9 18V5l11-2v13" /><circle cx="6" cy="18" r="3" /><circle cx="17" cy="16" r="3" /></> },
  { id: "mic", label: "Microphone", from: "#9b7bff", to: "#5a2ea6", glyph: <><rect x="9" y="3" width="6" height="11" rx="3" /><path d="M5 11a7 7 0 0 0 14 0M12 18v3" /></> },
  { id: "camera", label: "Camera", from: "#5ec8f2", to: "#1a6fb0", glyph: <><rect x="3" y="7" width="18" height="13" rx="2" /><path d="M8 7l2-3h4l2 3" /><circle cx="12" cy="13.5" r="3.5" /></> },
  { id: "popcorn", label: "Popcorn", from: "#ff8a65", to: "#d8323c", glyph: <><path d="M5 10l2 11h10l2-11z" /><path d="M6 10a2.5 2.5 0 0 1 2.6-3.6 3.2 3.2 0 0 1 6.8 0A2.5 2.5 0 0 1 18 10M10 10l.5 11M14 10l-.5 11" /></> },
  { id: "star", label: "Star", from: "#ffd45c", to: "#e8871e", glyph: <path d="M12 3l2.8 5.8 6.2.8-4.5 4.4 1.1 6.2L12 17.3l-5.6 2.9 1.1-6.2L3 9.6l6.2-.8z" /> },
  { id: "moon", label: "Moon", from: "#4a5b8c", to: "#1c2340", glyph: <><path d="M20 14.5A8 8 0 1 1 9.5 4a6.5 6.5 0 0 0 10.5 10.5z" /><path d="M17 4v2M16 5h2" /></> },
  { id: "sun", label: "Sun", from: "#ffc24b", to: "#ff7a1a", glyph: <><circle cx="12" cy="12" r="4" /><path d="M12 2v2M12 20v2M4.9 4.9l1.4 1.4M17.7 17.7l1.4 1.4M2 12h2M20 12h2M4.9 19.1l1.4-1.4M17.7 6.3l1.4-1.4" /></> },
  { id: "leaf", label: "Leaf", from: "#8fd35a", to: "#2f8a3b", glyph: <><path d="M5 19C5 10 11 5 20 5c0 9-5 15-14 15" /><path d="M5 19c3-4 6-7 10-9" /></> },
  { id: "wave", label: "Wave", from: "#4fd1e8", to: "#1769aa", glyph: <path d="M2 8c2.5-3 5-3 7.5 0s5 3 7.5 0 3.5-2 5-1M2 13c2.5-3 5-3 7.5 0s5 3 7.5 0 3.5-2 5-1M2 18c2.5-3 5-3 7.5 0s5 3 7.5 0 3.5-2 5-1" /> },
  { id: "mountain", label: "Mountain", from: "#7fa7c9", to: "#34506e", glyph: <><path d="M3 20l6-10 4 6 3-4 5 8z" /><circle cx="17" cy="6" r="2" /></> },
  { id: "rocket", label: "Rocket", from: "#ff6f61", to: "#8e2de2", glyph: <><path d="M12 2c3 2 5 6 5 10l-2 5H9l-2-5c0-4 2-8 5-10z" /><circle cx="12" cy="10" r="2" /><path d="M9 17l-2.5 4 3.5-1M15 17l2.5 4-3.5-1" /></> },
  { id: "planet", label: "Planet", from: "#c38bff", to: "#3a3fb8", glyph: <><circle cx="12" cy="12" r="5.5" /><ellipse cx="12" cy="12" rx="10.5" ry="3.5" transform="rotate(-20 12 12)" /></> },
  { id: "heart", label: "Heart", from: "#ff8fa3", to: "#e0245e", glyph: <path d="M12 20s-7.5-4.6-7.5-10.2A4.3 4.3 0 0 1 12 7.2a4.3 4.3 0 0 1 7.5 2.6C19.5 15.4 12 20 12 20z" /> },
];

const byId = Object.fromEntries(AVATARS.map((a) => [a.id, a]));

/** A user's avatar; falls back to initials when the id isn't a preset. */
export function Avatar({ id, name = "", size = 36, className }: { id?: string; name?: string; size?: number; className?: string }) {
  const gid = useId();
  const a = id ? byId[id] : undefined;
  if (!a) {
    const initials = name.split(/\s+/).filter(Boolean).slice(0, 2).map((w) => w[0]?.toUpperCase()).join("") || "?";
    return (
      <span className={`avatar-initials ${className ?? ""}`} style={{ width: size, height: size, fontSize: size * 0.4, background: toneFor(name || "?") }} aria-hidden="true">
        {initials}
      </span>
    );
  }
  return (
    <svg className={className} width={size} height={size} viewBox="0 0 48 48" aria-hidden="true" style={{ flexShrink: 0, display: "block" }}>
      <defs>
        <linearGradient id={gid} x1="0" y1="0" x2="1" y2="1">
          <stop offset="0" stopColor={a.from} />
          <stop offset="1" stopColor={a.to} />
        </linearGradient>
      </defs>
      <circle cx="24" cy="24" r="24" fill={`url(#${gid})`} />
      <g transform="translate(11 11) scale(1.083)" fill="none" stroke="#fff" strokeWidth="1.9" strokeLinecap="round" strokeLinejoin="round">
        {a.glyph}
      </g>
    </svg>
  );
}

/** Grid of presets as radio buttons. */
export function AvatarPicker({ value, onChange }: { value: string; onChange: (id: string) => void }) {
  return (
    <div className="avatar-picker" role="radiogroup" aria-label="Avatar">
      {AVATARS.map((a) => (
        <button key={a.id} type="button" role="radio" aria-checked={value === a.id} aria-label={a.label} title={a.label} onClick={() => onChange(a.id)}>
          <Avatar id={a.id} size={52} />
        </button>
      ))}
    </div>
  );
}
