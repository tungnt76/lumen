import type { Metadata } from "next";
import { DrawHome } from "@/components/DrawHome";

export const metadata: Metadata = {
  title: "Excalidraw",
  description: "A free whiteboard: sketch diagrams and ideas. Sign in to save your drawings.",
};

export default function DrawPage() {
  return <DrawHome />;
}
