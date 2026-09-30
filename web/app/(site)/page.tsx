/* eslint-disable @next/next/no-img-element */
import Link from "next/link";
import { api, formatRuntime, Home, type Page, type WorkCard } from "@/lib/api";
import { PosterRow, TopRow } from "@/components/Poster";
import { ContinueWatching, ListButton } from "@/components/Local";
import { ContinueListening } from "@/components/WorkControls";
import { AudioSpotlight, WorkRow } from "@/components/Works";
import { InfoIcon, PlayIcon } from "@/components/icons";

export const revalidate = 60;

export default async function HomePage() {
  const [home, books, music] = await Promise.all([
    api<Home>("/api/home", 60),
    api<Page<WorkCard>>("/api/books?page=1", 60),
    api<Page<WorkCard>>("/api/music?page=1", 60),
  ]);
  const freeMusic = music?.items.filter((w) => w.playable) ?? [];
  const artistMusic = music?.items.filter((w) => !w.playable) ?? [];
  const bookItems = books?.items ?? [];
  const spotBook = bookItems.find((w) => w.featured && w.playable) ?? bookItems.find((w) => w.playable);
  const spotAlbum = freeMusic.find((w) => w.featured) ?? freeMusic[0];
  const f = home?.featured;
  const rows = home?.rows ?? [];
  const [first, ...rest] = rows;

  return (
    <>
      {f ? (
        <section className="hero" aria-label="Featured film">
          {f.backdrop && <img className="hero-img" src={f.backdrop} alt="" fetchPriority="high" />}
          <div className="hero-shade" />
          <div className="hero-body">
            <span className="eyebrow">Featured · Free to watch</span>
            <h1>{f.title}</h1>
            <div className="chips">
              {f.year > 0 && <span className="chip">{f.year}</span>}
              {f.genres.slice(0, 2).map((g) => <span key={g} className="chip">{g}</span>)}
              {f.runtime ? <span className="chip">{formatRuntime(f.runtime)}</span> : null}
            </div>
            {f.overview && <p>{f.overview}</p>}
            <div className="actions">
              <Link href={`/movie/${f.tmdbId}?play=1`} className="btn btn-primary" style={{ height: 52, padding: "0 28px", borderRadius: 26 }}>
                <PlayIcon /> Play
              </Link>
              <Link href={`/movie/${f.tmdbId}`} className="btn btn-soft" style={{ height: 52, borderRadius: 26 }}>
                <InfoIcon size={18} /> More info
              </Link>
              <ListButton compact film={{ tmdbId: f.tmdbId, title: f.title, year: f.year, poster: f.poster, backdrop: f.backdrop }} />
            </div>
          </div>
        </section>
      ) : (
        <section className="hero" aria-label="Welcome">
          <div className="hero-body">
            <span className="eyebrow">Coming soon</span>
            <h1>Lumen</h1>
            <p>{home ? "The first films are being prepared. Meanwhile, browse what's trending." : "We can't reach the catalog right now. Please try again in a minute."}</p>
          </div>
        </section>
      )}

      <div className="rows">
        <ContinueWatching />
        <ContinueListening />
        {first && <PosterRow title={first.title} items={first.items} href="/browse" />}
        <AudioSpotlight book={spotBook} album={spotAlbum} />
        <WorkRow title="Free audiobooks" items={bookItems.slice(0, 12)} href="/books" />
        <WorkRow title="Free classical music" items={freeMusic.slice(0, 12)} href="/music" />
        <WorkRow title="Vietnamese artists · on official platforms" items={artistMusic.slice(0, 12)} href="/music" />
        {rest.map((r) =>
          r.kind === "top" ? (
            <TopRow key={r.title} title={r.title} items={r.items} />
          ) : (
            <PosterRow key={r.title} title={r.title} items={r.items} href={r.genreId ? `/browse?genre=${r.genreId}` : undefined} />
          ),
        )}
      </div>
    </>
  );
}
