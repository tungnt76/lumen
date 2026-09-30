import type { Metadata } from "next";
import { Catalog } from "@/components/Catalog";

export const metadata: Metadata = { title: "Audiobooks", description: "Free public-domain audiobooks in Vietnamese and English." };

type Props = { searchParams: Promise<{ q?: string; lang?: string; page?: string }> };

export default async function BooksPage({ searchParams }: Props) {
  return <Catalog kind="book" searchParams={await searchParams} />;
}
