"use client";

import { useState } from "react";
import { Trash2, Plus, X } from "lucide-react";
import { useTheme } from "@/lib/themes/provider";

export interface CrewMember {
  id: string;
  name: string;
  role: string;
  department: string;
  dayRate: number;
  startDate: string;
  endDate?: string;
}

interface CrewTableProps {
  members: CrewMember[];
  onAdd?: (member: Omit<CrewMember, "id">) => void;
  onRemove?: (memberId: string) => void;
}

function formatCurrency(amount: number): string {
  return new Intl.NumberFormat("en-US", {
    style: "currency",
    currency: "USD",
    minimumFractionDigits: 2,
  }).format(amount);
}

export function CrewTable({ members, onAdd, onRemove }: CrewTableProps) {
  const { entityLabels } = useTheme();
  const [showForm, setShowForm] = useState(false);
  const [formData, setFormData] = useState({
    name: "",
    role: "",
    department: "",
    dayRate: "",
    startDate: "",
  });
  const [formErrors, setFormErrors] = useState<Record<string, string>>({});

  function validateForm(): boolean {
    const errors: Record<string, string> = {};
    if (!formData.name.trim()) errors.name = "Name is required";
    if (!formData.role.trim()) errors.role = "Role is required";
    if (!formData.department.trim()) errors.department = "Department is required";
    if (!formData.dayRate || Number(formData.dayRate) <= 0)
      errors.dayRate = "Valid rate is required";
    if (!formData.startDate) errors.startDate = "Start date is required";
    setFormErrors(errors);
    return Object.keys(errors).length === 0;
  }

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (!validateForm()) return;
    onAdd?.({
      name: formData.name.trim(),
      role: formData.role.trim(),
      department: formData.department.trim(),
      dayRate: Number(formData.dayRate),
      startDate: formData.startDate,
    });
    setFormData({ name: "", role: "", department: "", dayRate: "", startDate: "" });
    setFormErrors({});
    setShowForm(false);
  }

  return (
    <div>
      {/* Header */}
      <div className="mb-4 flex items-center justify-between">
        <h3
          className="font-heading text-sm font-semibold"
          style={{ color: "var(--thittam-foreground, #0f172a)" }}
        >
          {entityLabels.teamMemberPlural}
        </h3>
        <button
          onClick={() => setShowForm(true)}
          className="inline-flex items-center gap-1.5 rounded-md px-3 py-1.5 text-xs font-heading font-medium text-white"
          style={{ backgroundColor: "var(--thittam-primary, #3b82f6)" }}
        >
          <Plus className="h-3.5 w-3.5" />
          Add Member
        </button>
      </div>

      {/* Slide-out form */}
      {showForm && (
        <form
          onSubmit={handleSubmit}
          className="mb-4 rounded-lg p-4"
          style={{
            backgroundColor: "var(--thittam-muted, #f1f5f9)",
            border: "1px solid var(--thittam-border, #e2e8f0)",
          }}
        >
          <div className="mb-3 flex items-center justify-between">
            <span
              className="font-heading text-sm font-medium"
              style={{ color: "var(--thittam-foreground, #0f172a)" }}
            >
              New {entityLabels.teamMember}
            </span>
            <button
              type="button"
              onClick={() => {
                setShowForm(false);
                setFormErrors({});
              }}
              className="text-gray-400 hover:text-gray-600"
            >
              <X className="h-4 w-4" />
            </button>
          </div>

          <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-5">
            {/* Name */}
            <div>
              <label className="mb-1 block text-xs font-heading font-medium text-gray-600">
                Name
              </label>
              <input
                type="text"
                value={formData.name}
                onChange={(e) =>
                  setFormData((p) => ({ ...p, name: e.target.value }))
                }
                className="w-full rounded-md border px-2.5 py-1.5 text-sm font-body"
                style={{ borderColor: formErrors.name ? "#DC2626" : "var(--thittam-border, #e2e8f0)" }}
              />
              {formErrors.name && (
                <p className="mt-0.5 text-[11px] text-red-600">{formErrors.name}</p>
              )}
            </div>

            {/* Role */}
            <div>
              <label className="mb-1 block text-xs font-heading font-medium text-gray-600">
                Role
              </label>
              <input
                type="text"
                value={formData.role}
                onChange={(e) =>
                  setFormData((p) => ({ ...p, role: e.target.value }))
                }
                className="w-full rounded-md border px-2.5 py-1.5 text-sm font-body"
                style={{ borderColor: formErrors.role ? "#DC2626" : "var(--thittam-border, #e2e8f0)" }}
              />
              {formErrors.role && (
                <p className="mt-0.5 text-[11px] text-red-600">{formErrors.role}</p>
              )}
            </div>

            {/* Department */}
            <div>
              <label className="mb-1 block text-xs font-heading font-medium text-gray-600">
                Department
              </label>
              <input
                type="text"
                value={formData.department}
                onChange={(e) =>
                  setFormData((p) => ({ ...p, department: e.target.value }))
                }
                className="w-full rounded-md border px-2.5 py-1.5 text-sm font-body"
                style={{ borderColor: formErrors.department ? "#DC2626" : "var(--thittam-border, #e2e8f0)" }}
              />
              {formErrors.department && (
                <p className="mt-0.5 text-[11px] text-red-600">{formErrors.department}</p>
              )}
            </div>

            {/* Day Rate */}
            <div>
              <label className="mb-1 block text-xs font-heading font-medium text-gray-600">
                {entityLabels.rateLabel}
              </label>
              <input
                type="number"
                step="0.01"
                min="0"
                value={formData.dayRate}
                onChange={(e) =>
                  setFormData((p) => ({ ...p, dayRate: e.target.value }))
                }
                className="w-full rounded-md border px-2.5 py-1.5 text-sm font-mono"
                style={{ borderColor: formErrors.dayRate ? "#DC2626" : "var(--thittam-border, #e2e8f0)" }}
              />
              {formErrors.dayRate && (
                <p className="mt-0.5 text-[11px] text-red-600">{formErrors.dayRate}</p>
              )}
            </div>

            {/* Start Date */}
            <div>
              <label className="mb-1 block text-xs font-heading font-medium text-gray-600">
                Start Date
              </label>
              <input
                type="date"
                value={formData.startDate}
                onChange={(e) =>
                  setFormData((p) => ({ ...p, startDate: e.target.value }))
                }
                className="w-full rounded-md border px-2.5 py-1.5 text-sm font-mono"
                style={{ borderColor: formErrors.startDate ? "#DC2626" : "var(--thittam-border, #e2e8f0)" }}
              />
              {formErrors.startDate && (
                <p className="mt-0.5 text-[11px] text-red-600">{formErrors.startDate}</p>
              )}
            </div>
          </div>

          <div className="mt-3 flex justify-end gap-2">
            <button
              type="button"
              onClick={() => {
                setShowForm(false);
                setFormErrors({});
              }}
              className="rounded-md px-3 py-1.5 text-xs font-heading font-medium"
              style={{
                color: "var(--thittam-muted-foreground, #64748b)",
                border: "1px solid var(--thittam-border, #e2e8f0)",
              }}
            >
              Cancel
            </button>
            <button
              type="submit"
              className="rounded-md px-3 py-1.5 text-xs font-heading font-medium text-white"
              style={{ backgroundColor: "var(--thittam-primary, #3b82f6)" }}
            >
              Add {entityLabels.teamMember}
            </button>
          </div>
        </form>
      )}

      {/* Table */}
      {members.length === 0 ? (
        <div
          className="rounded-lg border-dashed p-8 text-center"
          style={{ border: "2px dashed var(--thittam-border, #e2e8f0)" }}
        >
          <p
            className="text-sm font-body"
            style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
          >
            No {entityLabels.teamMemberPlural.toLowerCase()} assigned
          </p>
        </div>
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full text-left text-sm">
            <thead>
              <tr
                style={{
                  borderBottom: "1px solid var(--thittam-border, #e2e8f0)",
                }}
              >
                <th className="pb-2 pr-4 font-heading text-xs font-medium text-gray-500">
                  Name
                </th>
                <th className="pb-2 pr-4 font-heading text-xs font-medium text-gray-500">
                  Role
                </th>
                <th className="pb-2 pr-4 font-heading text-xs font-medium text-gray-500">
                  Department
                </th>
                <th className="pb-2 pr-4 font-heading text-xs font-medium text-gray-500">
                  {entityLabels.rateLabel}
                </th>
                <th className="pb-2 pr-4 font-heading text-xs font-medium text-gray-500">
                  Start Date
                </th>
                <th className="pb-2 font-heading text-xs font-medium text-gray-500">
                  {/* Actions */}
                </th>
              </tr>
            </thead>
            <tbody>
              {members.map((member) => (
                <tr
                  key={member.id}
                  className="group"
                  style={{
                    borderBottom: "1px solid var(--thittam-border, #e2e8f0)",
                  }}
                >
                  <td
                    className="py-3 pr-4 font-heading text-sm font-medium"
                    style={{ color: "var(--thittam-foreground, #0f172a)" }}
                  >
                    {member.name}
                  </td>
                  <td
                    className="py-3 pr-4 font-body text-sm"
                    style={{
                      color: "var(--thittam-muted-foreground, #64748b)",
                    }}
                  >
                    {member.role}
                  </td>
                  <td
                    className="py-3 pr-4 font-body text-sm"
                    style={{
                      color: "var(--thittam-muted-foreground, #64748b)",
                    }}
                  >
                    {member.department}
                  </td>
                  <td
                    className="py-3 pr-4 font-mono text-sm"
                    style={{
                      color: "var(--thittam-foreground, #0f172a)",
                    }}
                  >
                    {formatCurrency(member.dayRate)}
                    <span
                      className="text-xs"
                      style={{
                        color: "var(--thittam-muted-foreground, #64748b)",
                      }}
                    >
                      {entityLabels.rateUnit}
                    </span>
                  </td>
                  <td
                    className="py-3 pr-4 font-mono text-sm"
                    style={{
                      color: "var(--thittam-muted-foreground, #64748b)",
                    }}
                  >
                    {member.startDate}
                  </td>
                  <td className="py-3">
                    <button
                      onClick={() => onRemove?.(member.id)}
                      className="rounded p-1 text-gray-400 opacity-0 transition-opacity hover:bg-red-50 hover:text-red-600 group-hover:opacity-100"
                      title="Remove"
                    >
                      <Trash2 className="h-4 w-4" />
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
