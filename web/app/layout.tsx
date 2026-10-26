import type { ReactNode } from "react";
import "./globals.css";

export const metadata = { title: "Lakehouse Platform" };

export default function RootLayout({ children }: { children: ReactNode }) {
  return (
    <html lang="en">
      <body>
        <nav className="nav">
          <a href="/" className="brand">Lakehouse</a>
          <a href="/">Workspaces</a>
          <a href="http://localhost:8000" target="_blank" rel="noreferrer">Notebooks</a>
          <a href="http://localhost:5000" target="_blank" rel="noreferrer">Experiments</a>
        </nav>
        <main className="main">{children}</main>
      </body>
    </html>
  );
}
