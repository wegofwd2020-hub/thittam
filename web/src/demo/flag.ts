/**
 * Demo mode: the app serves recorded fixtures instead of calling services.
 *
 * Exactly "1" — no other spelling is accepted, so a typo in a build command
 * fails visibly rather than half-enabling the demo.
 *
 * Next inlines NEXT_PUBLIC_* at build time, so in a normal build this folds to
 * `false` and every `if (isDemo())` branch is eliminated from the bundle.
 */
export function isDemo(): boolean {
  return process.env.NEXT_PUBLIC_DEMO === "1";
}
