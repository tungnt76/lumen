import type { Metadata } from "next";
import { CodeHome } from "@/components/CodeHome";

export const metadata: Metadata = { title: "Code", description: "A VS Code-style editor in the browser. Sign in to save projects with a commit history." };

export default function CodePage() {
  return <CodeHome />;
}
