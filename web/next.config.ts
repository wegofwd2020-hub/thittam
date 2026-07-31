import type { NextConfig } from "next";

const demo = process.env.NEXT_PUBLIC_DEMO === "1";

const nextConfig: NextConfig = demo
  ? {
      output: "export",
      basePath: "/demos/thittam",
      // nginx serves directories; trailing slashes make try_files $uri $uri/ work.
      trailingSlash: true,
      // The Image Optimization API needs a server. A static export has none.
      images: { unoptimized: true },
    }
  : {};

export default nextConfig;
