import type { Metadata } from "next";
import { loadWork, WorkPage, workMetadata } from "@/components/WorkPage";

export const revalidate = 300;

type Params = { params: Promise<{ id: string }> };

export async function generateMetadata({ params }: Params): Promise<Metadata> {
  return workMetadata(await loadWork("album", (await params).id));
}

export default async function AlbumPage({ params }: Params) {
  return <WorkPage work={await loadWork("album", (await params).id)} />;
}
