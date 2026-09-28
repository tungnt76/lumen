import type { Metadata, Viewport } from "next";
import "./globals.css";

export const metadata: Metadata = {
  title: { default: "Lumen · Free classic films", template: "%s · Lumen" },
  description: "Watch public-domain classic films free, no account needed.",
};

export const viewport: Viewport = { themeColor: "#0e0e10" };

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en">
      <head>
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
