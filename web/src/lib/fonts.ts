// Typography system — 3 fonts + dyslexia-friendly mode.
// All fonts are SIL Open Font License (free, no restrictions).
//
// Default mode:
//   Headings/labels: Inter (sans-serif)
//   Body text:       Merriweather (serif)
//   Numbers/mono:    JetBrains Mono (monospace)
//
// Dyslexia mode:
//   All text:        OpenDyslexic / OpenDyslexic Mono

import { Inter, Merriweather, JetBrains_Mono } from "next/font/google";
import localFont from "next/font/local";

// --- Default fonts ---

export const inter = Inter({
  subsets: ["latin"],
  variable: "--font-heading",
  display: "swap",
});

export const merriweather = Merriweather({
  subsets: ["latin"],
  weight: ["300", "400", "700"],
  variable: "--font-body",
  display: "swap",
});

export const jetbrainsMono = JetBrains_Mono({
  subsets: ["latin"],
  variable: "--font-mono",
  display: "swap",
});

// --- Font CSS class string (applied to <body>) ---

export const defaultFontClasses = `${inter.variable} ${merriweather.variable} ${jetbrainsMono.variable}`;
