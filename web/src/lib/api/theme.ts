import { api } from "./client";
import type { ApiResponse } from "./types";
import type { ThemeColors } from "@/lib/themes/types";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export interface ThemeOverride {
  colors?: Partial<ThemeColors>;
  branding?: {
    logo?: string;
    favicon?: string;
    loginBg?: string;
  };
}

interface UploadResult {
  url: string;
}

// ---------------------------------------------------------------------------
// Theme override CRUD
// ---------------------------------------------------------------------------

const THEME_BASE = "/api/v1/settings/theme";

export async function getThemeOverride(): Promise<ThemeOverride | null> {
  try {
    const res = await api.get<ThemeOverride>(THEME_BASE);
    return res.data;
  } catch {
    // 404 means no override saved yet
    return null;
  }
}

export async function saveThemeOverride(
  override: ThemeOverride,
): Promise<void> {
  await api.post<void>(THEME_BASE, override);
}

export async function resetThemeOverride(): Promise<void> {
  await api.delete(THEME_BASE);
}

// ---------------------------------------------------------------------------
// File uploads
// ---------------------------------------------------------------------------

/**
 * Uploads a logo image. The API returns a URL for the stored file.
 * Note: This bypasses the JSON-only ApiClient because we need multipart.
 */
async function uploadFile(
  endpoint: string,
  file: File,
): Promise<UploadResult> {
  const formData = new FormData();
  formData.append("file", file);

  const res = await fetch(endpoint, {
    method: "POST",
    body: formData,
  });

  if (!res.ok) {
    throw new Error(`Upload failed: ${res.statusText}`);
  }

  const json = (await res.json()) as ApiResponse<UploadResult>;
  return json.data;
}

export async function uploadLogo(file: File): Promise<UploadResult> {
  return uploadFile(`${THEME_BASE}/logo`, file);
}

export async function uploadFavicon(file: File): Promise<UploadResult> {
  return uploadFile(`${THEME_BASE}/favicon`, file);
}
