import type { Metadata } from "next";
import "./globals.css";
import { Providers } from "./providers";
import { defaultFontClasses } from "@/lib/fonts";

export const metadata: Metadata = {
  title: "Thittam - Production Management",
  description:
    "Multi-tenant SaaS platform for production management by WeGoFwd2020.",
};

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html lang="en" className={`${defaultFontClasses} h-full antialiased`}>
      <body className="min-h-full flex flex-col font-body">
        <Providers>{children}</Providers>
      </body>
    </html>
  );
}
