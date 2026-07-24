"use client";

import { useState } from "react";
import { useParams } from "next/navigation";
import Link from "next/link";
import { ArrowLeft, LogOut, LogIn, Wrench } from "lucide-react";
import { useTheme } from "@/lib/themes/provider";
import { AmountDisplay } from "@/components/ui/amount-display";
import { AssetStatusBadge } from "@/components/inventory/asset-status-badge";
import { DepreciationBadge } from "@/components/inventory/depreciation-badge";
import { CheckoutDialog } from "@/components/inventory/checkout-dialog";
import { CheckinDialog } from "@/components/inventory/checkin-dialog";
import { CheckoutTimeline, type CheckoutEntry } from "@/components/inventory/checkout-timeline";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

type AssetStatus = "available" | "checked_out" | "under_repair" | "retired";

interface AssetDetail {
  id: string;
  asset_code: string;
  name: string;
  category: string;
  depreciation_years: number | null;
  description: string;
  ownership_type: string;
  status: AssetStatus;
  serial_number: string;
  purchase_cost: string;
  purchase_date: string | null;
  currency: string;
  current_assignee: string | null;
  current_production: string | null;
  checkoutHistory: CheckoutEntry[];
}

// ---------------------------------------------------------------------------
// Mock data — detail for "ARRI Alexa Mini LF #1" with 3 checkout entries
// ---------------------------------------------------------------------------

const mockAssets: Record<string, AssetDetail> = {
  "asset-001": {
    id: "asset-001",
    asset_code: "CAM-001",
    name: "ARRI Alexa Mini LF #1",
    category: "Camera Equipment",
    depreciation_years: 5,
    description: "Primary A-camera unit. Full sensor mode capable. Includes PL mount adapter and ARRI WCU-4 wireless control unit.",
    ownership_type: "owned",
    status: "checked_out",
    serial_number: "ARRI-K1234567",
    purchase_cost: "4500000.00",
    purchase_date: "2024-06-15",
    currency: "INR",
    current_assignee: "Arjun Mehta",
    current_production: "The Last Horizon",
    checkoutHistory: [
      {
        id: "co-001",
        production: "The Last Horizon",
        checkedOutTo: "Arjun Mehta",
        checkedOutAt: "2026-02-01",
        checkedInAt: null,
        conditionOut: "good",
        conditionIn: null,
      },
      {
        id: "co-002",
        production: "Monsoon Chronicles",
        checkedOutTo: "Sanjay Kapoor",
        checkedOutAt: "2025-08-15",
        checkedInAt: "2025-12-20",
        conditionOut: "good",
        conditionIn: "good",
      },
      {
        id: "co-003",
        production: "Neon Nights",
        checkedOutTo: "Priya Iyer",
        checkedOutAt: "2025-03-01",
        checkedInAt: "2025-07-30",
        conditionOut: "good",
        conditionIn: "fair",
      },
    ],
  },
  "asset-002": {
    id: "asset-002",
    asset_code: "CAM-002",
    name: "ARRI Alexa Mini LF #2",
    category: "Camera Equipment",
    depreciation_years: 5,
    description: "Secondary B-camera unit. Matched to CAM-001 for multi-camera setups.",
    ownership_type: "owned",
    status: "checked_out",
    serial_number: "ARRI-K1234568",
    purchase_cost: "4500000.00",
    purchase_date: "2024-06-15",
    currency: "INR",
    current_assignee: "Arjun Mehta",
    current_production: "The Last Horizon",
    checkoutHistory: [
      {
        id: "co-004",
        production: "The Last Horizon",
        checkedOutTo: "Arjun Mehta",
        checkedOutAt: "2026-02-01",
        checkedInAt: null,
        conditionOut: "good",
        conditionIn: null,
      },
    ],
  },
  "asset-003": {
    id: "asset-003",
    asset_code: "CAM-003",
    name: "Sony FX6",
    category: "Camera Equipment",
    depreciation_years: 5,
    description: "Compact cinema camera for run-and-gun and documentary-style shooting.",
    ownership_type: "owned",
    status: "available",
    serial_number: "SONY-FX6-98765",
    purchase_cost: "650000.00",
    purchase_date: "2025-01-10",
    currency: "INR",
    current_assignee: null,
    current_production: null,
    checkoutHistory: [],
  },
  "asset-004": {
    id: "asset-004",
    asset_code: "PRP-001",
    name: "Sci-fi control panels set",
    category: "Props",
    depreciation_years: 3,
    description: "Custom-built control panel props for sci-fi sequences. Includes LED backlighting and functional buttons.",
    ownership_type: "owned",
    status: "checked_out",
    serial_number: "",
    purchase_cost: "180000.00",
    purchase_date: "2025-11-20",
    currency: "INR",
    current_assignee: "Art Department",
    current_production: "The Last Horizon",
    checkoutHistory: [
      {
        id: "co-005",
        production: "The Last Horizon",
        checkedOutTo: "Art Department",
        checkedOutAt: "2026-01-10",
        checkedInAt: null,
        conditionOut: "good",
        conditionIn: null,
      },
    ],
  },
  "asset-005": {
    id: "asset-005",
    asset_code: "PRP-002",
    name: "Period costumes collection",
    category: "Costumes & Wardrobe",
    depreciation_years: null,
    description: "Collection of 1940s-era costumes. Includes 12 hero costumes and 30 background extras sets.",
    ownership_type: "rented",
    status: "checked_out",
    serial_number: "",
    purchase_cost: "0.00",
    purchase_date: null,
    currency: "INR",
    current_assignee: "Wardrobe Department",
    current_production: "The Last Horizon",
    checkoutHistory: [
      {
        id: "co-006",
        production: "The Last Horizon",
        checkedOutTo: "Wardrobe Department",
        checkedOutAt: "2026-01-20",
        checkedInAt: null,
        conditionOut: "good",
        conditionIn: null,
      },
    ],
  },
  "asset-006": {
    id: "asset-006",
    asset_code: "PRP-003",
    name: "Breakaway furniture set",
    category: "Props",
    depreciation_years: 3,
    description: "Pre-scored breakaway chairs, tables, and shelving for stunt sequences.",
    ownership_type: "owned",
    status: "available",
    serial_number: "",
    purchase_cost: "95000.00",
    purchase_date: "2025-09-05",
    currency: "INR",
    current_assignee: null,
    current_production: null,
    checkoutHistory: [],
  },
  "asset-007": {
    id: "asset-007",
    asset_code: "CAM-004",
    name: "DJI Inspire 3 Drone",
    category: "Camera Equipment",
    depreciation_years: 5,
    description: "Aerial cinematography drone with Zenmuse X9 8K camera. DGCA registered.",
    ownership_type: "owned",
    status: "available",
    serial_number: "DJI-INS3-44556",
    purchase_cost: "320000.00",
    purchase_date: "2025-04-22",
    currency: "INR",
    current_assignee: null,
    current_production: null,
    checkoutHistory: [],
  },
  "asset-008": {
    id: "asset-008",
    asset_code: "CAM-005",
    name: "Movi Pro Gimbal",
    category: "Camera Equipment",
    depreciation_years: 5,
    description: "3-axis motorized gimbal stabilizer for handheld and Steadicam-style shots.",
    ownership_type: "owned",
    status: "available",
    serial_number: "MOVI-PRO-33221",
    purchase_cost: "275000.00",
    purchase_date: "2025-02-18",
    currency: "INR",
    current_assignee: null,
    current_production: null,
    checkoutHistory: [],
  },
};

// ---------------------------------------------------------------------------
// Ownership badge
// ---------------------------------------------------------------------------

const OWNERSHIP_STYLES: Record<string, { bg: string; text: string }> = {
  owned: { bg: "bg-emerald-100", text: "text-emerald-700" },
  rented: { bg: "bg-purple-100", text: "text-purple-700" },
  borrowed: { bg: "bg-amber-100", text: "text-amber-700" },
};

function OwnershipBadge({ type }: { type: string }) {
  const style = OWNERSHIP_STYLES[type] ?? { bg: "bg-gray-100", text: "text-gray-600" };
  return (
    <span
      className={`inline-flex items-center rounded-full px-2 py-0.5 text-[10px] font-medium font-heading capitalize ${style.bg} ${style.text}`}
    >
      {type}
    </span>
  );
}

// ---------------------------------------------------------------------------
// Page
// ---------------------------------------------------------------------------

export default function AssetDetailPage() {
  const { entityLabels } = useTheme();
  const params = useParams<{ id: string }>();

  const [asset, setAsset] = useState<AssetDetail | null>(
    () => mockAssets[params.id] ?? null,
  );
  const [showCheckout, setShowCheckout] = useState(false);
  const [showCheckin, setShowCheckin] = useState(false);

  if (!asset) {
    return (
      <div className="mx-auto max-w-4xl py-12 text-center">
        <h1
          className="font-heading text-xl font-semibold"
          style={{ color: "var(--thittam-foreground, #0f172a)" }}
        >
          Asset not found
        </h1>
        <p
          className="mt-2 font-body text-sm"
          style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
        >
          The asset you are looking for does not exist or has been removed.
        </p>
        <Link
          href="/inventory"
          className="mt-4 inline-flex items-center gap-1.5 text-sm font-heading font-medium"
          style={{ color: "var(--thittam-primary, #3b82f6)" }}
        >
          <ArrowLeft className="h-4 w-4" />
          Back to Inventory
        </Link>
      </div>
    );
  }

  function handleCheckOut(data: {
    production_id: string;
    checked_out_to: string;
    expected_return: string;
    condition_out: string;
  }) {
    setAsset((prev) =>
      prev
        ? {
            ...prev,
            status: "checked_out" as AssetStatus,
            current_assignee: data.checked_out_to,
            current_production: "Production",
          }
        : prev,
    );
    setShowCheckout(false);
  }

  function handleCheckIn() {
    setAsset((prev) =>
      prev
        ? {
            ...prev,
            status: "available" as AssetStatus,
            current_assignee: null,
            current_production: null,
          }
        : prev,
    );
    setShowCheckin(false);
  }

  function handleMarkAvailable() {
    setAsset((prev) =>
      prev ? { ...prev, status: "available" as AssetStatus } : prev,
    );
  }

  const isAvailable = asset.status === "available";
  const isCheckedOut = asset.status === "checked_out";
  const isUnderRepair = asset.status === "under_repair";

  return (
    <div className="mx-auto max-w-5xl">
      {/* Back link */}
      <Link
        href="/inventory"
        className="mb-4 inline-flex items-center gap-1.5 text-sm font-heading font-medium transition-colors hover:opacity-80"
        style={{ color: "var(--thittam-primary, #3b82f6)" }}
      >
        <ArrowLeft className="h-4 w-4" />
        Inventory
      </Link>

      {/* Header */}
      <div className="mb-6 flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div className="flex items-center gap-3">
          <h1
            className="font-heading text-2xl font-semibold tracking-tight"
            style={{ color: "var(--thittam-foreground, #0f172a)" }}
          >
            {asset.name}
          </h1>
          <AssetStatusBadge status={asset.status} />
        </div>

        {/* Action buttons based on status */}
        <div className="flex items-center gap-2">
          {isAvailable && (
            <button
              onClick={() => setShowCheckout(true)}
              className="inline-flex items-center gap-1.5 rounded-lg px-3 py-1.5 text-sm font-heading font-medium text-white transition-colors hover:opacity-90"
              style={{ backgroundColor: "var(--thittam-primary, #3b82f6)" }}
            >
              <LogOut className="h-4 w-4" />
              Check Out
            </button>
          )}
          {isCheckedOut && (
            <button
              onClick={() => setShowCheckin(true)}
              className="inline-flex items-center gap-1.5 rounded-lg px-3 py-1.5 text-sm font-heading font-medium text-white transition-colors hover:opacity-90"
              style={{ backgroundColor: "var(--thittam-status-on-track, #16A34A)" }}
            >
              <LogIn className="h-4 w-4" />
              Check In
            </button>
          )}
          {isUnderRepair && (
            <button
              onClick={handleMarkAvailable}
              className="inline-flex items-center gap-1.5 rounded-lg px-3 py-1.5 text-sm font-heading font-medium text-white transition-colors hover:opacity-90"
              style={{ backgroundColor: "var(--thittam-status-on-track, #16A34A)" }}
            >
              <Wrench className="h-4 w-4" />
              Mark Available
            </button>
          )}
          {/* Retired assets have no action buttons */}
        </div>
      </div>

      {/* Detail card */}
      <div
        className="mb-6 rounded-xl p-5"
        style={{
          backgroundColor: "var(--thittam-background, #fff)",
          border: "1px solid var(--thittam-border, #e2e8f0)",
        }}
      >
        <div className="grid gap-6 sm:grid-cols-2 lg:grid-cols-3">
          {/* Category */}
          <div>
            <span
              className="block text-xs font-heading font-medium"
              style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
            >
              Category
            </span>
            <div className="mt-1 flex items-center gap-2">
              <span
                className="text-sm font-heading font-medium"
                style={{ color: "var(--thittam-foreground, #0f172a)" }}
              >
                {asset.category}
              </span>
              <DepreciationBadge depreciationYears={asset.depreciation_years} />
            </div>
          </div>

          {/* Ownership */}
          <div>
            <span
              className="block text-xs font-heading font-medium"
              style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
            >
              Ownership
            </span>
            <div className="mt-1">
              <OwnershipBadge type={asset.ownership_type} />
            </div>
          </div>

          {/* Asset code */}
          <div>
            <span
              className="block text-xs font-heading font-medium"
              style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
            >
              Asset Code
            </span>
            <span
              className="mt-1 text-sm font-mono tabular-nums"
              style={{ color: "var(--thittam-foreground, #0f172a)" }}
            >
              {asset.asset_code}
            </span>
          </div>

          {/* Serial number */}
          {asset.serial_number && (
            <div>
              <span
                className="block text-xs font-heading font-medium"
                style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
              >
                Serial Number
              </span>
              <span
                className="mt-1 text-sm font-mono tabular-nums"
                style={{ color: "var(--thittam-foreground, #0f172a)" }}
              >
                {asset.serial_number}
              </span>
            </div>
          )}

          {/* Purchase cost */}
          {asset.purchase_cost !== "0.00" && (
            <div>
              <span
                className="block text-xs font-heading font-medium"
                style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
              >
                Purchase Cost
              </span>
              <div className="mt-1">
                <AmountDisplay amount={asset.purchase_cost} currency={asset.currency} size="lg" />
              </div>
            </div>
          )}

          {/* Purchase date */}
          {asset.purchase_date && (
            <div>
              <span
                className="block text-xs font-heading font-medium"
                style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
              >
                Purchase Date
              </span>
              <span
                className="mt-1 text-sm font-mono tabular-nums"
                style={{ color: "var(--thittam-foreground, #0f172a)" }}
              >
                {asset.purchase_date}
              </span>
            </div>
          )}

          {/* Current status */}
          <div>
            <span
              className="block text-xs font-heading font-medium"
              style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
            >
              Current Status
            </span>
            <div className="mt-1">
              <AssetStatusBadge status={asset.status} />
            </div>
          </div>

          {/* Current assignee (if checked out) */}
          {asset.current_assignee && (
            <div>
              <span
                className="block text-xs font-heading font-medium"
                style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
              >
                Current Assignee
              </span>
              <span
                className="mt-1 text-sm font-body"
                style={{ color: "var(--thittam-foreground, #0f172a)" }}
              >
                {asset.current_assignee}
              </span>
            </div>
          )}

          {/* Current production (if checked out) */}
          {asset.current_production && (
            <div>
              <span
                className="block text-xs font-heading font-medium"
                style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
              >
                Current {entityLabels.project}
              </span>
              <span
                className="mt-1 text-sm font-body"
                style={{ color: "var(--thittam-primary, #3b82f6)" }}
              >
                {asset.current_production}
              </span>
            </div>
          )}
        </div>

        {/* Description */}
        {asset.description && (
          <div className="mt-5 border-t pt-4" style={{ borderColor: "var(--thittam-border, #e2e8f0)" }}>
            <span
              className="block text-xs font-heading font-medium"
              style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
            >
              Description
            </span>
            <p
              className="mt-1 text-sm font-body"
              style={{ color: "var(--thittam-foreground, #0f172a)" }}
            >
              {asset.description}
            </p>
          </div>
        )}
      </div>

      {/* Checkout history */}
      <div
        className="mb-6 rounded-xl p-5"
        style={{
          backgroundColor: "var(--thittam-background, #fff)",
          border: "1px solid var(--thittam-border, #e2e8f0)",
        }}
      >
        <h2
          className="mb-4 font-heading text-sm font-semibold"
          style={{ color: "var(--thittam-foreground, #0f172a)" }}
        >
          Checkout History
        </h2>
        <CheckoutTimeline entries={asset.checkoutHistory} />
      </div>

      {/* Checkout dialog */}
      {showCheckout && (
        <CheckoutDialog
          assetName={asset.name}
          onConfirm={handleCheckOut}
          onCancel={() => setShowCheckout(false)}
        />
      )}

      {/* Check-in dialog */}
      {showCheckin && (
        <CheckinDialog
          assetName={asset.name}
          onConfirm={() => handleCheckIn()}
          onCancel={() => setShowCheckin(false)}
        />
      )}
    </div>
  );
}
