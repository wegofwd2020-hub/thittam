"use client";

import { useState } from "react";
import { X, Send } from "lucide-react";

// ---------------------------------------------------------------------------
// InviteUserDialog — modal for inviting a new user to the tenant.
// ---------------------------------------------------------------------------

interface InviteUserDialogProps {
  roles: { id: string; name: string }[];
  onConfirm: (data: { email: string; role: string; department?: string }) => void;
  onCancel: () => void;
}

function isValidEmail(email: string): boolean {
  return /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email);
}

export function InviteUserDialog({
  roles,
  onConfirm,
  onCancel,
}: InviteUserDialogProps) {
  const [email, setEmail] = useState("");
  const [role, setRole] = useState(roles[0]?.name ?? "");
  const [department, setDepartment] = useState("");
  const [emailError, setEmailError] = useState<string | null>(null);

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();

    if (!email.trim()) {
      setEmailError("Email is required");
      return;
    }
    if (!isValidEmail(email)) {
      setEmailError("Enter a valid email address");
      return;
    }

    setEmailError(null);
    onConfirm({
      email: email.trim(),
      role,
      department: department.trim() || undefined,
    });
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50">
      <div
        className="relative w-full max-w-md rounded-xl p-6 shadow-lg"
        style={{
          backgroundColor: "var(--thittam-background, #fff)",
          border: "1px solid var(--thittam-border, #e2e8f0)",
        }}
      >
        {/* Close button */}
        <button
          onClick={onCancel}
          className="absolute right-3 top-3 rounded-md p-1 transition-colors hover:opacity-70"
          style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
        >
          <X className="h-4 w-4" />
        </button>

        {/* Title */}
        <h2
          className="mb-4 font-heading text-lg font-semibold"
          style={{ color: "var(--thittam-foreground, #0f172a)" }}
        >
          Invite User
        </h2>

        <form onSubmit={handleSubmit} className="flex flex-col gap-4">
          {/* Email */}
          <div>
            <label
              className="mb-1 block text-xs font-heading font-medium"
              style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
            >
              Email Address
            </label>
            <input
              type="email"
              value={email}
              onChange={(e) => {
                setEmail(e.target.value);
                setEmailError(null);
              }}
              placeholder="user@company.com"
              className="w-full rounded-lg px-3 py-2 font-body text-sm"
              style={{
                backgroundColor: "var(--thittam-background, #fff)",
                border: `1px solid ${emailError ? "#ef4444" : "var(--thittam-border, #e2e8f0)"}`,
                color: "var(--thittam-foreground, #0f172a)",
              }}
            />
            {emailError && (
              <p className="mt-1 text-xs font-body text-red-600">{emailError}</p>
            )}
          </div>

          {/* Role */}
          <div>
            <label
              className="mb-1 block text-xs font-heading font-medium"
              style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
            >
              Role
            </label>
            <select
              value={role}
              onChange={(e) => setRole(e.target.value)}
              className="w-full rounded-lg px-3 py-2 font-heading text-sm"
              style={{
                backgroundColor: "var(--thittam-background, #fff)",
                border: "1px solid var(--thittam-border, #e2e8f0)",
                color: "var(--thittam-foreground, #0f172a)",
              }}
            >
              {roles.map((r) => (
                <option key={r.id} value={r.name}>
                  {r.name.replace(/_/g, " ").replace(/\b\w/g, (c) => c.toUpperCase())}
                </option>
              ))}
            </select>
          </div>

          {/* Department */}
          <div>
            <label
              className="mb-1 block text-xs font-heading font-medium"
              style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
            >
              Department
              <span
                className="ml-1 font-normal"
                style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
              >
                (optional)
              </span>
            </label>
            <input
              type="text"
              value={department}
              onChange={(e) => setDepartment(e.target.value)}
              placeholder="e.g. Camera, Art, Production Office"
              className="w-full rounded-lg px-3 py-2 font-body text-sm"
              style={{
                backgroundColor: "var(--thittam-background, #fff)",
                border: "1px solid var(--thittam-border, #e2e8f0)",
                color: "var(--thittam-foreground, #0f172a)",
              }}
            />
          </div>

          {/* Actions */}
          <div className="flex items-center justify-end gap-3 pt-2">
            <button
              type="button"
              onClick={onCancel}
              className="rounded-lg px-4 py-2 text-sm font-heading font-medium transition-colors hover:opacity-80"
              style={{
                backgroundColor: "var(--thittam-muted, #f1f5f9)",
                color: "var(--thittam-foreground, #0f172a)",
              }}
            >
              Cancel
            </button>
            <button
              type="submit"
              className="inline-flex items-center gap-2 rounded-lg px-4 py-2 text-sm font-heading font-medium text-white shadow-sm transition-colors hover:opacity-90"
              style={{ backgroundColor: "var(--thittam-primary, #3b82f6)" }}
            >
              <Send className="h-4 w-4" />
              Send Invitation
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}
