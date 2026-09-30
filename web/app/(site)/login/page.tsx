import type { Metadata } from "next";
import { Suspense } from "react";
import { SignInForm } from "@/components/AuthForms";

export const metadata: Metadata = { title: "Sign in", robots: { index: false } };

export default function LoginPage() {
  return <Suspense><SignInForm /></Suspense>;
}
