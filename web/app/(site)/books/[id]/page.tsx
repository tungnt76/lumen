import type { Metadata } from "next";
import { loadWork, WorkPage, workMetadata } from "@/components/WorkPage";

export const revalidate = 300;

type Params = { params: Promise<{ id: string }> };

export async function generateMetadata({ params }: Params): Promise<Metadata> {
  return workMetadata(await loadWork("book", (await params).id));
}

export default async function BookPage({ params }: Params) {
  return <WorkPage work={await loadWork("book", (await params).id)} />;
}
