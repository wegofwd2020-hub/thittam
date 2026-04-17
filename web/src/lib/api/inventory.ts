import { api } from "./client";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export interface Asset {
  id: string;
  tenant_id: string;
  asset_code: string;
  name: string;
  category_id: string;
  description: string;
  ownership_type: string; // owned | rented | borrowed
  status: string; // available | checked_out | under_repair | retired
  purchase_date: string | null;
  purchase_cost: string; // decimal string
  serial_number: string;
  created_at: string;
  updated_at: string;
}

export interface AssetCheckout {
  id: string;
  asset_id: string;
  production_id: string;
  tenant_id: string;
  checked_out_to: string;
  checked_out_at: string;
  expected_return: string | null;
  checked_in_at: string | null;
  condition_out: string;
  condition_in: string | null;
}

export interface InventoryCategory {
  id: string;
  label: string;
  is_trackable: boolean;
  depreciation_years: number | null;
}

// ---------------------------------------------------------------------------
// Request payloads
// ---------------------------------------------------------------------------

export interface CreateAssetInput {
  name: string;
  category_id: string;
  description?: string;
  asset_code?: string;
  serial_number?: string;
  ownership_type: string;
  purchase_cost?: string;
  purchase_date?: string;
}

export interface CheckOutAssetInput {
  production_id: string;
  checked_out_to: string;
  expected_return?: string;
  condition_out: string;
}

export interface CheckInAssetInput {
  condition_in: string;
  notes?: string;
  report_damage?: boolean;
  damage_severity?: string;
  damage_description?: string;
  repair_cost?: string;
}

// ---------------------------------------------------------------------------
// Query-string helper
// ---------------------------------------------------------------------------

function qs(params?: Record<string, string | number | undefined>): string {
  if (!params) return "";
  const entries = Object.entries(params).filter(
    ([, v]) => v !== undefined && v !== "",
  );
  if (entries.length === 0) return "";
  const search = new URLSearchParams(
    entries.map(([k, v]) => [k, String(v)]),
  );
  return `?${search.toString()}`;
}

// ---------------------------------------------------------------------------
// Assets
// ---------------------------------------------------------------------------

const BASE = "/api/v1/assets";

export async function listAssets(
  params?: { status?: string; category_id?: string; search?: string; limit?: number; after?: string },
): Promise<Asset[]> {
  return api.getList<Asset[]>(`${BASE}${qs(params)}`);
}

export async function getAsset(id: string): Promise<Asset> {
  return api.get<Asset>(`${BASE}/${id}`);
}

export async function createAsset(data: CreateAssetInput): Promise<Asset> {
  return api.post<Asset>(BASE, data);
}

// ---------------------------------------------------------------------------
// Checkouts
// ---------------------------------------------------------------------------

export async function listCheckouts(
  assetId: string,
  params?: { limit?: number; after?: string },
): Promise<AssetCheckout[]> {
  return api.getList<AssetCheckout[]>(
    `${BASE}/${assetId}/checkouts${qs(params)}`,
  );
}

export async function checkOutAsset(
  assetId: string,
  data: CheckOutAssetInput,
): Promise<AssetCheckout> {
  return api.post<AssetCheckout>(
    `${BASE}/${assetId}/check-out`,
    data,
  );
}

export async function checkInAsset(
  assetId: string,
  data: CheckInAssetInput,
): Promise<AssetCheckout> {
  return api.post<AssetCheckout>(
    `${BASE}/${assetId}/check-in`,
    data,
  );
}

// ---------------------------------------------------------------------------
// Categories (vertical config)
// ---------------------------------------------------------------------------

export async function getInventoryCategories(): Promise<InventoryCategory[]> {
  return api.getList<InventoryCategory[]>(
    "/api/v1/config/inventory-categories",
  );
}
