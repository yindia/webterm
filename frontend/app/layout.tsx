import "./globals.css";

import type { ReactNode } from "react";

export const metadata = {
  title: "webterm",
  description: "Self-hosted browser terminal",
  icons: {
    icon: "/icon.svg",
  },
};

export default function RootLayout({ children }: { children: ReactNode }) {
  return (
    <html lang="en">
      <body>{children}</body>
    </html>
  );
}
