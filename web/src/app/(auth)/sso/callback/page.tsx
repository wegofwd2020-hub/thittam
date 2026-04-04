"use client";

import { Suspense, useEffect, useState } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import { Loader2 } from "lucide-react";
import { env } from "@/env";

const ACCESS_TOKEN_KEY = "thittam_access_token";
const REFRESH_TOKEN_KEY = "thittam_refresh_token";

function SsoCallbackContent() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const code = searchParams.get("code");
    const state = searchParams.get("state");
    const errorParam = searchParams.get("error");

    if (errorParam) {
      setError(
        searchParams.get("error_description") ??
          "SSO authentication failed. Please try again."
      );
      return;
    }

    if (!code) {
      setError("Missing authorization code. Please try signing in again.");
      return;
    }

    async function exchangeCode() {
      try {
        const res = await fetch(
          `${env.platformApiUrl}/api/v1/auth/sso/callback`,
          {
            method: "POST",
            headers: {
              "Content-Type": "application/json",
              Accept: "application/json",
            },
            body: JSON.stringify({ code, state }),
          }
        );

        if (!res.ok) {
          const body = await res.json().catch(() => ({}));
          throw new Error(
            body.message ?? `SSO token exchange failed (${res.status})`
          );
        }

        const data = await res.json();
        localStorage.setItem(ACCESS_TOKEN_KEY, data.data.access_token);
        localStorage.setItem(REFRESH_TOKEN_KEY, data.data.refresh_token);
        router.replace("/");
      } catch (err) {
        setError(
          err instanceof Error
            ? err.message
            : "An unexpected error occurred during SSO sign-in."
        );
      }
    }

    exchangeCode();
  }, [searchParams, router]);

  if (error) {
    return (
      <div className="flex min-h-full flex-1 items-center justify-center px-4">
        <div className="w-full max-w-sm text-center">
          <div className="rounded-xl border border-red-200 bg-red-50 p-8 dark:border-red-800 dark:bg-red-950">
            <h1 className="mb-2 text-lg font-semibold text-red-700 dark:text-red-300">
              Sign-in failed
            </h1>
            <p className="mb-6 text-sm text-red-600 dark:text-red-400">
              {error}
            </p>
            <a
              href="/login"
              className="inline-flex h-9 items-center rounded-md bg-[var(--thittam-primary,#3b82f6)] px-4 text-sm font-medium text-white hover:opacity-90"
            >
              Back to login
            </a>
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className="flex min-h-full flex-1 items-center justify-center px-4">
      <div className="flex flex-col items-center gap-4 text-center">
        <Loader2 className="h-8 w-8 animate-spin text-[var(--thittam-primary,#3b82f6)]" />
        <p className="text-sm text-[var(--thittam-muted-foreground,#64748b)]">
          Signing you in...
        </p>
      </div>
    </div>
  );
}

function SsoCallbackFallback() {
  return (
    <div className="flex min-h-full flex-1 items-center justify-center px-4">
      <div className="flex flex-col items-center gap-4 text-center">
        <Loader2 className="h-8 w-8 animate-spin text-[var(--thittam-primary,#3b82f6)]" />
        <p className="text-sm text-[var(--thittam-muted-foreground,#64748b)]">
          Signing you in...
        </p>
      </div>
    </div>
  );
}

export default function SsoCallbackPage() {
  return (
    <Suspense fallback={<SsoCallbackFallback />}>
      <SsoCallbackContent />
    </Suspense>
  );
}
