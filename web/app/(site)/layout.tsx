import { Footer, Nav } from "@/components/Nav";

export default function SiteLayout({ children }: { children: React.ReactNode }) {
  return (
    <>
      <a href="#content" className="sr-only">Skip to content</a>
      <Nav />
      <main id="content">{children}</main>
      <Footer />
    </>
  );
}
