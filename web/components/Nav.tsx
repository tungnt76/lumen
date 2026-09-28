"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { BookmarkIcon, HomeIcon, SearchIcon } from "./icons";

const links = [
  { href: "/", label: "Home" },
  { href: "/browse", label: "Movies" },
  { href: "/my-list", label: "My list" },
];

export function Nav() {
  const path = usePathname();
  const current = (href: string) => (href === "/" ? path === "/" : path.startsWith(href));
  return (
    <>
      <header className="nav">
        <Link href="/" className="logo" aria-label="Lumen home">
          <span className="logo-mark" />
          <span className="logo-word">Lumen</span>
        </Link>
        <nav aria-label="Main" className="nav-links">
          {links.map((l) => (
            <Link key={l.href} href={l.href} aria-current={current(l.href) ? "page" : undefined}>
              {l.label}
            </Link>
          ))}
        </nav>
        <Link href="/browse?focus=1" className="icon-btn" aria-label="Search" style={{ marginLeft: "auto" }}>
          <SearchIcon />
        </Link>
      </header>
      <nav aria-label="Tabs" className="tabbar">
        <Link href="/" aria-current={current("/") ? "page" : undefined}><HomeIcon size={22} />Home</Link>
        <Link href="/browse" aria-current={current("/browse") ? "page" : undefined}><SearchIcon size={22} />Browse</Link>
        <Link href="/my-list" aria-current={current("/my-list") ? "page" : undefined}><BookmarkIcon size={22} />My list</Link>
      </nav>
    </>
  );
}

export function Footer() {
  return (
    <footer className="footer">
      <div>
        <p>Every film on Lumen is in the public domain or published with the rights holder&apos;s permission.</p>
        <p>This product uses the TMDB API but is not endorsed or certified by TMDB. Where-to-watch data by JustWatch.</p>
      </div>
      <a href="https://www.themoviedb.org/" target="_blank" rel="noopener noreferrer" className="chip" style={{ alignSelf: "center", textDecoration: "none" }}>
        {/* TODO before launch: replace with an approved logo from themoviedb.org/about/logos-attribution */}
        Powered by TMDB
      </a>
    </footer>
  );
}
