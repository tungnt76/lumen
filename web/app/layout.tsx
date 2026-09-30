import type { Metadata, Viewport } from "next";
import "./globals.css";
import { themeScript } from "@/lib/theme";

export const metadata: Metadata = {
  title: { default: "Lumen · Free films, audiobooks and music", template: "%s · Lumen" },
  description: "Watch public-domain films and listen to free audiobooks and music, no account needed.",
};

export const viewport: Viewport = { themeColor: "#0e0e10" };

// Favicon, touch icon and tab icon come from app/icon.svg, app/favicon.ico and app/apple-icon.png.

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en" suppressHydrationWarning>
      <head>
        {/* Applies a saved light choice before first paint (dark is the default). */}
        <script dangerouslySetInnerHTML={{ __html: themeScript }} />
        <link rel="preconnect" href="https://fonts.googleapis.com" />
        <link rel="preconnect" href="https://fonts.gstatic.com" crossOrigin="" />
        {/* eslint-disable-next-line @next/next/no-page-custom-font */}
        <link
          rel="stylesheet"
          href="https://fonts.googleapis.com/css2?family=Bricolage+Grotesque:opsz,wght@12..96,500;12..96,700&family=DM+Sans:wght@400;500;600&display=swap"
        />
        <link rel="preconnect" href="https://image.tmdb.org" />
      </head>
      <body>{children}</body>
    </html>
  );
}
