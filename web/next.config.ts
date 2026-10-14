import type { NextConfig } from "next";
import { execSync } from "node:child_process";

// Release version shown in the sidebar ticker: the latest git tag (e.g. v0.22.0).
// Falls back to "dev" when git or tags are unavailable (CI tarballs, fresh clones).
function appVersion(): string {
  try {
    return execSync("git describe --tags --abbrev=0", { stdio: ["ignore", "pipe", "ignore"] })
      .toString()
      .trim();
  } catch {
    return "dev";
  }
}

const nextConfig: NextConfig = {
  env: {
    NEXT_PUBLIC_APP_VERSION: appVersion(),
  },
};

export default nextConfig;
