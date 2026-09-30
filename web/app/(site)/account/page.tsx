import type { Metadata } from "next";
import { AccountSettings } from "@/components/AccountSettings";

export const metadata: Metadata = { title: "Profile & settings", robots: { index: false } };

export default function AccountPage() {
  return <AccountSettings />;
}
