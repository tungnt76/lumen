"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { LogoMark } from "./Logo";
import { ThemeToggle } from "./ThemeToggle";
import { AccountMenu } from "./AccountMenu";
import { BookIcon, BookmarkIcon, CodeIcon, FilmIcon, HomeIcon, MusicIcon, PenIcon, SearchIcon } from "./icons";

const links = [
  { href: "/", label: "Home", icon: HomeIcon },
  { href: "/browse", label: "Movies", icon: FilmIcon },
  { href: "/books", label: "Books", icon: BookIcon },
  { href: "/music", label: "Music", icon: MusicIcon },
  { href: "/my-list", label: "My list", icon: BookmarkIcon },
  { href: "/draw", label: "Excalidraw", icon: PenIcon },
  { href: "/code", label: "Code", icon: CodeIcon },
];

// Book and album pages belong to their tab; film pages to Movies.
const sections: [string, string][] = [["/movie/", "/browse"]];

export function Nav() {
  const path = usePathname();
  const current = (href: string) =>
    href === "/" ? path === "/" : path.startsWith(href) || sections.some(([p, h]) => h === href && path.startsWith(p));
  return (
    <>
      <header className="nav">
        <Link href="/" className="logo" aria-label="Lumen home">
          <LogoMark size={30} />
          <span className="logo-word">Lumen</span>
        </Link>
        <nav aria-label="Main" className="nav-links">
          {links.map((l) => (
            <Link key={l.href} href={l.href} aria-current={current(l.href) ? "page" : undefined}>
              {l.label}
            </Link>
          ))}
        </nav>
        <div className="nav-tools">
          <ThemeToggle />
          <Link href="/browse?focus=1" className="icon-btn" aria-label="Search films">
            <SearchIcon />
          </Link>
          <AccountMenu />
        </div>
      </header>
      <nav aria-label="Tabs" className="tabbar">
        {links.map(({ href, label, icon: Icon }) => (
          <Link key={href} href={href} aria-current={current(href) ? "page" : undefined}><Icon size={22} />{label}</Link>
        ))}
      </nav>
    </>
  );
}

export function Footer() {
  return (
    <footer className="footer">
      <div>
        <p>Every film, book and song hosted on Lumen is in the public domain, openly licensed, or published with the rights holder&apos;s permission. Music by current artists links to their official channels.</p>
        <p>This product uses the TMDB API but is not endorsed or certified by TMDB. Where-to-watch data by JustWatch.</p>
        <p>
          Audiobooks from <a href="https://librivox.org/" target="_blank" rel="noopener noreferrer">LibriVox</a> and texts from Wikisource; music
          from <a href="https://musopen.org/" target="_blank" rel="noopener noreferrer">Musopen</a> via the Internet Archive; release data from
          MusicBrainz, covers from the Cover Art Archive and composer portraits from Wikimedia Commons. AI voices use Piper with the VAIS-1000 corpus (CC BY 4.0).
        </p>
      </div>
      <div className="footer-side">
        <a href="https://www.themoviedb.org/" target="_blank" rel="noopener noreferrer" className="chip" style={{ textDecoration: "none" }}>
          {/* TODO before launch: replace with an approved logo from themoviedb.org/about/logos-attribution */}
          Powered by TMDB
        </a>
      </div>
    </footer>
  );
}
