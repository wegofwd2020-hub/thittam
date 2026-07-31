"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { useForm } from "react-hook-form";
import { z } from "zod/v4";
import { zodResolver } from "@hookform/resolvers/zod";
import { LogIn, ShieldCheck, Loader2 } from "lucide-react";
import { useAuth, ApiError } from "@/lib/auth/context";
import { isDemo } from "@/demo/flag";
import { DEMO_LANDING } from "@/demo/nav";
import { demoMeta } from "@/demo/transport";

const loginSchema = z.object({
  email: z.email("Please enter a valid email address"),
  password: z.string().min(1, "Password is required"),
});

type LoginValues = z.infer<typeof loginSchema>;

export default function LoginPage() {
  const { login } = useAuth();
  const router = useRouter();
  const [serverError, setServerError] = useState<string | null>(null);

  const {
    register,
    handleSubmit,
    formState: { errors, isSubmitting },
  } = useForm<LoginValues>({
    resolver: zodResolver(loginSchema),
    defaultValues: isDemo()
      ? { email: demoMeta().demoEmail, password: "demo1234" }
      : { email: "", password: "" },
  });

  async function onSubmit(values: LoginValues) {
    setServerError(null);
    try {
      await login(values.email, values.password);
      // "/" is the dashboard, which is driven by reporting-analytics — a
      // service with no grpc-gateway, so nothing could be recorded for it.
      router.replace(isDemo() ? DEMO_LANDING : "/");
    } catch (err) {
      if (err instanceof ApiError) {
        if (err.code === "TENANT_SUSPENDED") {
          setServerError(
            "Your organization has been suspended. Contact support."
          );
        } else if (err.status === 401) {
          setServerError("Invalid email or password.");
        } else {
          setServerError(err.message);
        }
      } else {
        setServerError("An unexpected error occurred. Please try again.");
      }
    }
  }

  function handleSsoLogin() {
    // Redirect to OIDC authorization endpoint
    const ssoUrl = `${process.env.NEXT_PUBLIC_PLATFORM_API_URL ?? "http://localhost:8086"}/api/v1/auth/sso/authorize`;
    window.location.href = ssoUrl;
  }

  return (
    <div className="flex min-h-full flex-1 items-center justify-center px-4 py-12">
      <div className="w-full max-w-sm">
        <div className="rounded-xl border border-[var(--thittam-border,#e2e8f0)] bg-white p-8 shadow-sm dark:bg-zinc-900">
          <h1 className="mb-1 text-2xl font-semibold tracking-tight text-[var(--thittam-foreground,#0f172a)]">
            Sign in
          </h1>
          <p className="mb-6 text-sm text-[var(--thittam-muted-foreground,#64748b)]">
            Enter your credentials to access your account.
          </p>

          {isDemo() && (
            <div className="mb-6 rounded-md border border-blue-200 bg-blue-50 px-4 py-3 text-sm text-blue-800 dark:border-blue-800 dark:bg-blue-950 dark:text-blue-200">
              <p className="font-medium">Demo — read-only</p>
              <p className="mt-1">
                Sign in as <code>{demoMeta().demoEmail}</code> with password{" "}
                <code>demo1234</code>. Data is a recorded snapshot; changes
                cannot be saved.
              </p>
            </div>
          )}

          {serverError && (
            <div className="mb-4 rounded-md border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700 dark:border-red-800 dark:bg-red-950 dark:text-red-300">
              {serverError}
            </div>
          )}

          <form onSubmit={handleSubmit(onSubmit)} className="flex flex-col gap-4">
            <div>
              <label
                htmlFor="email"
                className="mb-1 block text-sm font-medium text-[var(--thittam-foreground,#0f172a)]"
              >
                Email
              </label>
              <input
                id="email"
                type="email"
                autoComplete="email"
                placeholder="you@company.com"
                className="w-full rounded-md border border-[var(--thittam-border,#e2e8f0)] bg-[var(--thittam-background,#fff)] px-3 py-2 text-sm text-[var(--thittam-foreground,#0f172a)] placeholder:text-[var(--thittam-muted-foreground,#64748b)] focus:outline-none focus:ring-2 focus:ring-[var(--thittam-primary,#3b82f6)]"
                {...register("email")}
              />
              {errors.email && (
                <p className="mt-1 text-xs text-red-600 dark:text-red-400">
                  {errors.email.message}
                </p>
              )}
            </div>

            <div>
              <label
                htmlFor="password"
                className="mb-1 block text-sm font-medium text-[var(--thittam-foreground,#0f172a)]"
              >
                Password
              </label>
              <input
                id="password"
                type="password"
                autoComplete="current-password"
                placeholder="Enter your password"
                className="w-full rounded-md border border-[var(--thittam-border,#e2e8f0)] bg-[var(--thittam-background,#fff)] px-3 py-2 text-sm text-[var(--thittam-foreground,#0f172a)] placeholder:text-[var(--thittam-muted-foreground,#64748b)] focus:outline-none focus:ring-2 focus:ring-[var(--thittam-primary,#3b82f6)]"
                {...register("password")}
              />
              {errors.password && (
                <p className="mt-1 text-xs text-red-600 dark:text-red-400">
                  {errors.password.message}
                </p>
              )}
            </div>

            <button
              type="submit"
              disabled={isSubmitting}
              className="flex h-10 w-full items-center justify-center gap-2 rounded-md bg-[var(--thittam-primary,#3b82f6)] px-4 text-sm font-medium text-[var(--thittam-primary-foreground,#fff)] transition-colors hover:opacity-90 disabled:opacity-50"
            >
              {isSubmitting ? (
                <Loader2 className="h-4 w-4 animate-spin" />
              ) : (
                <LogIn className="h-4 w-4" />
              )}
              Sign in
            </button>
          </form>

          {/* SSO redirects to NEXT_PUBLIC_PLATFORM_API_URL, a dead host in a
              static build. The divider goes with it, or the card ends on a
              rule with nothing under it. */}
          {!isDemo() && (
            <>
              <div className="relative my-6">
                <div className="absolute inset-0 flex items-center">
                  <div className="w-full border-t border-[var(--thittam-border,#e2e8f0)]" />
                </div>
                <div className="relative flex justify-center text-xs uppercase">
                  <span className="bg-white px-2 text-[var(--thittam-muted-foreground,#64748b)] dark:bg-zinc-900">
                    or
                  </span>
                </div>
              </div>

              <button
                type="button"
                onClick={handleSsoLogin}
                className="flex h-10 w-full items-center justify-center gap-2 rounded-md border border-[var(--thittam-border,#e2e8f0)] bg-[var(--thittam-background,#fff)] px-4 text-sm font-medium text-[var(--thittam-foreground,#0f172a)] transition-colors hover:bg-[var(--thittam-muted,#f1f5f9)]"
              >
                <ShieldCheck className="h-4 w-4" />
                Sign in with SSO
              </button>
            </>
          )}

          {/* There is no /forgot-password route in src/app at all, so this
              404s in the demo — and in the product. See Follow-ups. */}
          {!isDemo() && (
            <p className="mt-6 text-center text-xs text-[var(--thittam-muted-foreground,#64748b)]">
              <a
                href="/forgot-password"
                className="font-medium text-[var(--thittam-primary,#3b82f6)] hover:underline"
              >
                Forgot your password?
              </a>
            </p>
          )}
        </div>
      </div>
    </div>
  );
}
