import { AuthProvider } from "@/lib/auth/context";

export default function AuthLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <AuthProvider>
      <div className="flex min-h-screen flex-col bg-gradient-to-br from-[var(--thittam-primary,#3b82f6)]/5 via-background to-[var(--thittam-secondary,#8b5cf6)]/5">
        {/* Logo header */}
        <header className="flex items-center justify-center px-4 pt-10 pb-4">
          <div className="flex items-center gap-2">
            <div className="flex h-9 w-9 items-center justify-center rounded-lg bg-[var(--thittam-primary,#3b82f6)] text-sm font-bold text-white">
              T
            </div>
            <span className="text-xl font-semibold tracking-tight text-foreground">
              Thittam
            </span>
          </div>
        </header>

        {/* Centered content */}
        <main className="flex flex-1 items-start justify-center">{children}</main>

        {/* Minimal footer */}
        <footer className="py-6 text-center text-xs text-muted-foreground">
          &copy; {new Date().getFullYear()} WeGoFwd2020. All rights reserved.
        </footer>
      </div>
    </AuthProvider>
  );
}
