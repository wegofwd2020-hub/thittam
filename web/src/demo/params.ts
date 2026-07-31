import fixtures from "./fixtures.generated.json";

const file = fixtures as unknown as { responses: Record<string, unknown> };

/**
 * The `[id]` values to pre-render, derived from what was actually recorded.
 *
 * Reading these from the fixtures rather than a hand-kept list means a
 * re-capture cannot leave the route list stale.
 */
export function idsForCollection(collection: string): string[] {
  const prefix = `GET /api/v1/${collection}/`;
  const ids = new Set<string>();

  for (const key of Object.keys(file.responses)) {
    if (!key.startsWith(prefix)) continue;
    const rest = key.slice(prefix.length);
    // Skip sub-resources like ".../p1/phases" and any trailing-slash artefact.
    if (rest === "" || rest.includes("/")) continue;
    ids.add(rest);
  }

  return [...ids];
}
