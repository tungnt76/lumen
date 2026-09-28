/* eslint-disable @next/next/no-img-element */
import type { Metadata } from "next";
import { notFound } from "next/navigation";
import { api, formatRuntime, MovieDetail, Provider } from "@/lib/api";
import { Player } from "@/components/Player";
import { ListButton } from "@/components/Local";
import { PosterRow } from "@/components/Poster";
import { Scroller } from "@/components/Scroller";

export const revalidate = 300;

type Params = { params: Promise<{ id: string }>; searchParams: Promise<{ play?: string }> };

async function load(id: string) {
  if (!/^\d{1,9}$/.test(id)) return null;
  return api<MovieDetail>(`/api/movies/${id}`, 300);
}

export async function generateMetadata({ params }: Params): Promise<Metadata> {
  const m = await load((await params).id);
  if (!m) return { title: "Not found" };
  return {
    title: m.year ? `${m.title} (${m.year})` : m.title,
    description: m.overview.slice(0, 160),
    openGraph: { images: m.backdrop ? [m.backdrop] : [] },
  };
}

function ProviderList({ title, list }: { title: string; list: Provider[] }) {
  if (!list.length) return null;
  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 8 }}>
      <span className="muted" style={{ fontSize: 13 }}>{title}</span>
      <div className="providers">
        {list.map((p) => <img key={p.name} src={p.logo} alt={p.name} title={p.name} loading="lazy" />)}
      </div>
    </div>
  );
}

export default async function MoviePage({ params, searchParams }: Params) {
  const { id } = await params;
  const { play } = await searchParams;
  const m = await load(id);
  if (!m) notFound();

  const saved = { tmdbId: m.tmdbId, title: m.title, year: m.year, poster: m.poster, backdrop: m.backdrop };
  const hasProviders = m.providers && (m.providers.stream.length || m.providers.rent.length || m.providers.buy.length);

  return (
    <div className="wrap" style={{ paddingTop: 28, display: "flex", flexDirection: "column", gap: 40 }}>
      {m.playback ? (
        <Player film={saved} hls={m.playback.hls} subtitles={m.playback.subtitles} autoPlay={play === "1"} />
      ) : m.trailerKey ? (
        <iframe
          className="trailer"
          src={`https://www.youtube-nocookie.com/embed/${m.trailerKey}`}
          title={`${m.title} trailer`}
          allow="encrypted-media; picture-in-picture; fullscreen"
          loading="lazy"
        />
      ) : null}

      <div className="detail">
        <div style={{ display: "flex", flexDirection: "column", gap: 22, minWidth: 0 }}>
          <h1>{m.title}</h1>
          <div className="chips">
            {m.year > 0 && <span className="chip">{m.year}</span>}
            {m.genres.slice(0, 3).map((g) => <span key={g} className="chip">{g}</span>)}
            {m.runtime ? <span className="chip">{formatRuntime(m.runtime)}</span> : null}
            {m.playback ? <span className="chip chip-accent">Free on Lumen</span> : <span className="chip">Not hosted on Lumen</span>}
          </div>
          <div className="actions">
            <ListButton film={saved} />
            {m.playback && m.trailerKey && (
              <a className="btn" href={`https://www.youtube.com/watch?v=${m.trailerKey}`} target="_blank" rel="noopener noreferrer">Watch trailer</a>
            )}
          </div>
          {m.overview && <p className="overview">{m.overview}</p>}
          {m.cast.length > 0 && (
            <section style={{ display: "flex", flexDirection: "column", gap: 14 }}>
              <h2 style={{ fontSize: 20 }}>Cast</h2>
              <Scroller className="cast" inset>
                {m.cast.map((c) => (
                  <div key={c.name + c.character} className="cast-item">
                    {c.photo ? <img className="avatar" src={c.photo} alt="" loading="lazy" /> : <span className="avatar">{c.name.split(" ").map((w) => w[0]).slice(0, 2).join("")}</span>}
                    <span style={{ fontWeight: 600 }}>{c.name}</span>
                    {c.character && <span className="muted">{c.character}</span>}
                  </div>
                ))}
              </Scroller>
            </section>
          )}
        </div>

        <aside className="panel">
          <h2>Details</h2>
          {m.director && <div className="facts"><span className="muted">Director</span><span>{m.director}</span></div>}
          {m.year > 0 && <div className="facts"><span className="muted">Released</span><span>{m.year}</span></div>}
          {m.runtime ? <div className="facts"><span className="muted">Runtime</span><span>{formatRuntime(m.runtime)}</span></div> : null}
          {m.playback && m.playback.subtitles.length > 0 && (
            <div className="facts"><span className="muted">Subtitles</span><span>{m.playback.subtitles.map((s) => s.label).join(", ")}</span></div>
          )}
          {m.source && <div className="facts"><span className="muted">Rights</span><span>{m.source}</span></div>}
          {!m.playback && (
            hasProviders ? (
              <>
                <h2 style={{ marginTop: 8 }}>Where to watch</h2>
                <ProviderList title="Stream" list={m.providers!.stream} />
                <ProviderList title="Rent" list={m.providers!.rent} />
                <ProviderList title="Buy" list={m.providers!.buy} />
                <a href={m.providers!.link} target="_blank" rel="noopener noreferrer" style={{ fontSize: 14 }}>See all options on TMDB</a>
              </>
            ) : (
              <p className="muted" style={{ margin: 0, fontSize: 14 }}>No legal streaming option listed for your region yet.</p>
            )
          )}
          <span className="muted" style={{ fontSize: 12 }}>Metadata and images from TMDB. Streaming availability from JustWatch.</span>
        </aside>
      </div>

      <div style={{ margin: "0 calc(var(--gutter) * -1)", paddingBottom: 48 }}>
        <PosterRow title="More like this" items={m.similar} />
      </div>
    </div>
  );
}
