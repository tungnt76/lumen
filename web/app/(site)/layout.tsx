import { Footer, Nav } from "@/components/Nav";
import { AudioProvider } from "@/components/AudioPlayer";

export default function SiteLayout({ children }: { children: React.ReactNode }) {
  return (
    <AudioProvider>
      <a href="#content" className="sr-only">Skip to content</a>
      <Nav />
      <main id="content">{children}</main>
      <Footer />
    </AudioProvider>
  );
}
