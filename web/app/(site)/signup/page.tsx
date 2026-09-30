import type { Metadata } from "next";
import { Suspense } from "react";
import { SignUpForm } from "@/components/AuthForms";

export const metadata: Metadata = { title: "Create account", robots: { index: false } };

export default function SignupPage() {
  return <Suspense><SignUpForm /></Suspense>;
}
