import type { Metadata } from "next";
import { Catalog } from "@/components/Catalog";

export const metadata: Metadata = { title: "Music", description: "Free public-domain music, and where to listen to Vietnamese artists legally." };

type Props = { searchParams: Promise<{ q?: string; page?: string }> };

export default async function MusicPage({ searchParams }: Props) {
  return <Catalog kind="album" searchParams={await searchParams} />;
}
