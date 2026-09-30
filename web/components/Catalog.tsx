/* eslint-disable @next/next/no-img-element */
import Link from "next/link";
import { api, formatDuration, languageNames, workHref, type Page, type WorkCard, type WorkKind } from "@/lib/api";
import { SearchIcon } from "@/components/icons";
import { WorkGrid } from "@/components/Works";

type Props = {
  kind: WorkKind;
  searchParams: { q?: string; lang?: string; page?: string };
};

const copy = {
  book: {
    path: "/books", endpoint: "/api/books", title: "Audiobooks", noun: "book", placeholder: "Search titles or authors (no accents needed)",
    eyebrow: "Featured audiobook", langs: ["vi", "en"],
  },
  album: {
    path: "/music", endpoint: "/api/music", title: "Music", noun: "album", placeholder: "Search albums or artists (no accents needed)",
    eyebrow: "Featured album", langs: [] as string[],
  },
};

/** The Books and Music tabs: featured hero, search, language filter, grid and pages. */
export async function Catalog({ kind, searchParams: sp }: Props) {
  const c = copy[kind];
  const q = (sp.q ?? "").trim().slice(0, 100);
  const lang = /^[a-z]{2}$/.test(sp.lang ?? "") ? sp.lang! : "";
  const page = /^\d+$/.test(sp.page ?? "") ? Math.max(1, Number(sp.page)) : 1;

  const qs = new URLSearchParams({ page: String(page) });
  if (q) qs.set("q", q);
  if (lang) qs.set("lang", lang);
  const res = await api<Page<WorkCard>>(`${c.endpoint}?${qs}`, 60);
  const items = res?.items ?? [];
  const total = res?.total ?? 0;
  const plain = !q && !lang && page === 1;
  const featured = plain ? items.find((w) => w.featured && w.playable) : undefined;
  const hosted = items.filter((w) => w.playable);
  const external = items.filter((w) => !w.playable);

  const href = (p: { page?: number; lang?: string }) => {
    const s = new URLSearchParams();
    if (q) s.set("q", q);
    const l = p.lang ?? lang;
    if (l) s.set("lang", l);
    if (p.page && p.page > 1) s.set("page", String(p.page));
    const str = s.toString();
    return str ? `${c.path}?${str}` : c.path;
  };

  return (
    <>
      {featured && (
        <section className="hero hero-audio" aria-label={c.eyebrow}>
          {featured.cover && <img className="hero-img" src={featured.cover} alt="" fetchPriority="high" />}
          <div className="hero-shade" />
          <div className="hero-body">
            <span className="eyebrow">{c.eyebrow} · Free to listen</span>
            <h1>{featured.title}</h1>
            <div className="chips">
              {featured.creator && <span className="chip">{featured.creator}</span>}
              {featured.trackCount > 1 && <span className="chip">{featured.trackCount} {kind === "book" ? "chapters" : "tracks"}</span>}
              {featured.durationSec > 0 && <span className="chip">{formatDuration(featured.durationSec)}</span>}
            </div>
            <div className="actions">
              <Link href={workHref(featured)} className="btn btn-primary" style={{ height: 52, padding: "0 28px", borderRadius: 26 }}>Listen now</Link>
            </div>
          </div>
        </section>
      )}

      <div className="wrap" style={{ padding: "44px var(--gutter) 64px", display: "flex", flexDirection: "column", gap: 24 }}>
        <h1 style={{ fontSize: "clamp(34px, 4vw, 48px)" }}>{q ? `Results for “${q}”` : c.title}</h1>

        <form action={c.path} role="search" className="search">
          <SearchIcon size={22} />
          <label htmlFor="q" className="sr-only">{c.placeholder}</label>
          <input id="q" name="q" type="search" defaultValue={q} placeholder={c.placeholder} autoComplete="off" />
          {lang && <input type="hidden" name="lang" value={lang} />}
          <button type="submit" className="btn btn-primary" style={{ height: 40 }}>Search</button>
        </form>

        {c.langs.length > 0 && (
          <nav aria-label="Languages" className="pills">
            <Link href={href({ lang: "" })} className="pill" aria-current={lang === "" ? "true" : undefined}>All languages</Link>
            {c.langs.map((l) => (
              <Link key={l} href={href({ lang: l })} className="pill" aria-current={lang === l ? "true" : undefined}>{languageNames[l] ?? l}</Link>
            ))}
          </nav>
        )}

        <p className="muted" style={{ margin: 0, fontSize: 14 }}>
          {res ? `${total} ${c.noun}${total === 1 ? "" : "s"}${lang ? ` in ${languageNames[lang] ?? lang}` : ""}` : "We can't reach the catalog right now."}
        </p>

        {items.length === 0 && res && <p className="muted">Nothing here yet.</p>}

        {hosted.length > 0 && (
          <section className="catalog-section" aria-label="Free to listen">
            {external.length > 0 && <h2>Free to listen on Lumen</h2>}
            <WorkGrid items={hosted} />
          </section>
        )}

        {external.length > 0 && (
          <section className="catalog-section" aria-label="Listen on official platforms">
            <div>
              <h2>Listen on official platforms</h2>
              <p className="muted" style={{ margin: "6px 0 0", fontSize: 14 }}>
                Current artists&apos; music is copyrighted, so Lumen doesn&apos;t host it. These link to the artist&apos;s own channels.
              </p>
            </div>
            <WorkGrid items={external} />
          </section>
        )}

        {res && res.totalPages > 1 && (
          <nav aria-label="Pages" className="pager">
            <Link href={href({ page: page - 1 })} className="btn" aria-disabled={page <= 1} tabIndex={page <= 1 ? -1 : undefined}>Previous</Link>
            <span className="muted">Page {res.page} of {res.totalPages}</span>
            <Link href={href({ page: page + 1 })} className="btn" aria-disabled={page >= res.totalPages} tabIndex={page >= res.totalPages ? -1 : undefined}>Next</Link>
          </nav>
        )}
      </div>
    </>
  );
}
