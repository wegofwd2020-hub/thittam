/**
 * Fixture keys are "<METHOD> <path>", where path is exactly what the API
 * modules build — including any query string from qs().
 */
export function requestKey(method: string, path: string): string {
  return `${method.toUpperCase()} ${path}`;
}

/**
 * Keys to try, most specific first.
 *
 * A capture cannot cover every filter combination, so a request carrying a
 * query string falls back to the unfiltered recording. A bare "?" with nothing
 * after it is not a real query — it collapses to just the unfiltered key.
 */
export function lookupKeys(method: string, path: string): string[] {
  const queryStart = path.indexOf("?");
  if (queryStart === -1) return [requestKey(method, path)];

  const bare = requestKey(method, path.slice(0, queryStart));

  // Nothing after the "?" → no real query, only the bare key.
  if (queryStart === path.length - 1) return [bare];

  return [requestKey(method, path), bare];
}
