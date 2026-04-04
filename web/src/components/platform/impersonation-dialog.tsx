"use client";

import { useState } from "react";
import { Clock, AlertTriangle, X, Shield } from "lucide-react";

// ---------------------------------------------------------------------------
// ImpersonationDialog — modal for platform admins to impersonate tenant users.
// Requires a reason (audit trail) and warns about the 30-minute session limit.
// ---------------------------------------------------------------------------

interface TenantInfo {
  id: string;
  name: string;
}

interface TenantUser {
  id: string;
  name: string;
  email: string;
  role: string;
}

interface ImpersonationDialogProps {
  tenant: TenantInfo;
  users: TenantUser[];
  isOpen: boolean;
  onClose: () => void;
  onConfirm: (userId: string, reason: string) => void;
}

export function ImpersonationDialog({
  tenant,
  users,
  isOpen,
  onClose,
  onConfirm,
}: ImpersonationDialogProps) {
  const [selectedUserId, setSelectedUserId] = useState("");
  const [reason, setReason] = useState("");

  if (!isOpen) return null;

  const isValid = selectedUserId !== "" && reason.trim().length >= 10;

  function handleConfirm() {
    if (!isValid) return;
    onConfirm(selectedUserId, reason.trim());
    setSelectedUserId("");
    setReason("");
  }

  function handleClose() {
    setSelectedUserId("");
    setReason("");
    onClose();
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center">
      {/* Backdrop */}
      <div
        className="absolute inset-0 bg-black/60"
        onClick={handleClose}
      />

      {/* Dialog */}
      <div className="relative z-10 w-full max-w-lg rounded-xl bg-white shadow-2xl">
        {/* Header */}
        <div className="flex items-center justify-between border-b border-slate-200 px-6 py-4">
          <div className="flex items-center gap-2">
            <Shield className="h-5 w-5 text-orange-500" />
            <h2 className="font-heading text-lg font-semibold text-slate-900">
              Impersonate User
            </h2>
          </div>
          <button
            onClick={handleClose}
            className="flex h-8 w-8 items-center justify-center rounded-md text-slate-400 hover:text-slate-600"
            aria-label="Close"
          >
            <X className="h-5 w-5" />
          </button>
        </div>

        {/* Body */}
        <div className="px-6 py-5 space-y-5">
          {/* Tenant name */}
          <div>
            <p className="font-body text-sm text-slate-500">Tenant</p>
            <p className="font-heading text-base font-medium text-slate-900">
              {tenant.name}
            </p>
          </div>

          {/* User select */}
          <div>
            <label
              htmlFor="impersonate-user"
              className="mb-1.5 block font-heading text-sm font-medium text-slate-700"
            >
              Select User
            </label>
            <select
              id="impersonate-user"
              value={selectedUserId}
              onChange={(e) => setSelectedUserId(e.target.value)}
              className="w-full rounded-lg border border-slate-300 bg-white px-3 py-2 font-body text-sm text-slate-900 focus:border-orange-500 focus:outline-none focus:ring-1 focus:ring-orange-500"
            >
              <option value="">Choose a user...</option>
              {users.map((u) => (
                <option key={u.id} value={u.id}>
                  {u.name} ({u.role}) &mdash; {u.email}
                </option>
              ))}
            </select>
          </div>

          {/* Reason */}
          <div>
            <label
              htmlFor="impersonate-reason"
              className="mb-1.5 block font-heading text-sm font-medium text-slate-700"
            >
              Reason <span className="text-red-500">*</span>
            </label>
            <textarea
              id="impersonate-reason"
              value={reason}
              onChange={(e) => setReason(e.target.value)}
              placeholder="Describe why you need to impersonate this user (min 10 characters)..."
              rows={3}
              className="w-full rounded-lg border border-slate-300 bg-white px-3 py-2 font-body text-sm text-slate-900 placeholder:text-slate-400 focus:border-orange-500 focus:outline-none focus:ring-1 focus:ring-orange-500"
            />
            {reason.length > 0 && reason.trim().length < 10 && (
              <p className="mt-1 font-body text-xs text-red-500">
                Reason must be at least 10 characters.
              </p>
            )}
          </div>

          {/* Warnings */}
          <div className="space-y-2">
            <div className="flex items-start gap-2 rounded-lg bg-amber-50 px-3 py-2.5">
              <Clock className="mt-0.5 h-4 w-4 shrink-0 text-amber-600" />
              <p className="font-body text-sm text-amber-800">
                Impersonation sessions are limited to <strong>30 minutes</strong>.
                You will be automatically returned to the platform admin view after
                the session expires.
              </p>
            </div>
            <div className="flex items-start gap-2 rounded-lg bg-slate-50 px-3 py-2.5">
              <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-slate-500" />
              <p className="font-body text-sm text-slate-600">
                This action will be logged in the audit trail with your identity,
                the target user, reason, and timestamp.
              </p>
            </div>
          </div>
        </div>

        {/* Footer */}
        <div className="flex items-center justify-end gap-3 border-t border-slate-200 px-6 py-4">
          <button
            onClick={handleClose}
            className="rounded-lg px-4 py-2 font-heading text-sm font-medium text-slate-700 hover:bg-slate-100"
          >
            Cancel
          </button>
          <button
            onClick={handleConfirm}
            disabled={!isValid}
            className="rounded-lg bg-orange-600 px-4 py-2 font-heading text-sm font-medium text-white shadow-sm transition-colors hover:bg-orange-700 disabled:cursor-not-allowed disabled:opacity-50"
          >
            Start Impersonation
          </button>
        </div>
      </div>
    </div>
  );
}
