import type { Metadata } from "next";
import Link from "next/link";
import { api, Card, Genre, Page } from "@/lib/api";
import { Poster } from "@/components/Poster";
import { SearchIcon } from "@/components/icons";

export const metadata: Metadata = { title: "Browse films" };

type Props = { searchParams: Promise<{ genre?: string; q?: string; focus?: string; page?: string }> };

export default async function BrowsePage({ searchParams }: Props) {
  const sp = await searchParams;
  const q = (sp.q ?? "").trim().slice(0, 100);
  const genre = /^\d+$/.test(sp.genre ?? "") ? Number(sp.genre) : 0;
  const page = /^\d+$/.test(sp.page ?? "") ? Math.max(1, Number(sp.page)) : 1;

  const [genres, results] = await Promise.all([
    api<Genre[]>("/api/genres", 86400),
    q
      ? api<Page<Card>>(`/api/search?q=${encodeURIComponent(q)}&page=${page}`, 300)
      : api<Page<Card>>(`/api/browse?page=${page}${genre ? `&genre=${genre}` : ""}`, 60),
  ]);
  const items = results?.items ?? [];
  const total = results?.total ?? 0;
  const genreName = genres?.find((g) => g.id === genre)?.name;

  // Keeps the current search or genre when moving between pages.
  const pageHref = (p: number) => {
    const qs = new URLSearchParams();
    if (q) qs.set("q", q);
    else if (genre) qs.set("genre", String(genre));
    if (p > 1) qs.set("page", String(p));
    const s = qs.toString();
    return s ? `/browse?${s}` : "/browse";
  };

  return (
    <div className="wrap" style={{ padding: "44px var(--gutter) 64px", display: "flex", flexDirection: "column", gap: 24 }}>
      <h1 style={{ fontSize: "clamp(34px, 4vw, 48px)" }}>{q ? `Results for “${q}”` : "Browse films"}</h1>

      <form action="/browse" role="search" className="search">
        <SearchIcon size={22} />
        <label htmlFor="q" className="sr-only">Search films</label>
        <input id="q" name="q" type="search" defaultValue={q} placeholder="Search titles" autoFocus={sp.focus === "1"} autoComplete="off" />
        <button type="submit" className="btn btn-primary" style={{ height: 40 }}>Search</button>
      </form>

      {!q && genres && (
        <nav aria-label="Genres" className="pills">
          <Link href="/browse" className="pill" aria-current={genre === 0 ? "true" : undefined}>All</Link>
          {genres.map((g) => (
            <Link key={g.id} href={`/browse?genre=${g.id}`} className="pill" aria-current={genre === g.id ? "true" : undefined}>{g.name}</Link>
          ))}
        </nav>
      )}

      <p className="muted" style={{ margin: 0, fontSize: 14 }}>
        {q
          ? `${total} result${total === 1 ? "" : "s"}. Films marked Free play here; others show where to watch legally.`
          : `${total} film${total === 1 ? "" : "s"} free on Lumen${genreName ? ` in ${genreName}` : ""}`}
      </p>

      {items.length > 0 ? (
        <div className="grid">{items.map((c) => <Poster key={c.tmdbId} card={c} />)}</div>
      ) : (
        <p className="muted">{results ? "Nothing here yet." : "We can't reach the catalog right now."}</p>
      )}

      {results && results.totalPages > 1 && (
        <nav aria-label="Pages" className="pager">
          <Link href={pageHref(page - 1)} className="btn" aria-disabled={page <= 1} tabIndex={page <= 1 ? -1 : undefined}>Previous</Link>
          <span className="muted">Page {results.page} of {results.totalPages}</span>
          <Link href={pageHref(page + 1)} className="btn" aria-disabled={page >= results.totalPages} tabIndex={page >= results.totalPages ? -1 : undefined}>Next</Link>
        </nav>
      )}
    </div>
  );
}
