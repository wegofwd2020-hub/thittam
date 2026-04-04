"use client";

import { useState } from "react";
import { Loader2, CheckCircle2, XCircle, Save } from "lucide-react";

// ---------------------------------------------------------------------------
// OidcConfigForm — OIDC SSO configuration form with test & save.
// ---------------------------------------------------------------------------

interface OidcFormValues {
  issuer_url: string;
  client_id: string;
  client_secret: string;
  scopes: string[];
  email_claim: string;
  display_name_claim: string;
  groups_claim: string;
  auto_provision: boolean;
  default_role: string;
}

interface OidcConfigFormProps {
  initialValues?: Partial<OidcFormValues>;
  roles: { id: string; name: string }[];
  onSave: (values: OidcFormValues) => void;
}

type TestState = "idle" | "testing" | "success" | "error";

function formatRoleName(name: string): string {
  return name.replace(/_/g, " ").replace(/\b\w/g, (c) => c.toUpperCase());
}

export function OidcConfigForm({
  initialValues,
  roles,
  onSave,
}: OidcConfigFormProps) {
  const [values, setValues] = useState<OidcFormValues>({
    issuer_url: initialValues?.issuer_url ?? "",
    client_id: initialValues?.client_id ?? "",
    client_secret: initialValues?.client_secret ?? "",
    scopes: initialValues?.scopes ?? ["openid", "email", "profile"],
    email_claim: initialValues?.email_claim ?? "email",
    display_name_claim: initialValues?.display_name_claim ?? "name",
    groups_claim: initialValues?.groups_claim ?? "",
    auto_provision: initialValues?.auto_provision ?? false,
    default_role: initialValues?.default_role ?? roles[0]?.name ?? "crew_member",
  });

  const [scopeInput, setScopeInput] = useState("");
  const [testState, setTestState] = useState<TestState>("idle");

  function update<K extends keyof OidcFormValues>(
    key: K,
    value: OidcFormValues[K],
  ) {
    setValues((prev) => ({ ...prev, [key]: value }));
  }

  function handleAddScope(e: React.KeyboardEvent<HTMLInputElement>) {
    if (e.key === "Enter" || e.key === ",") {
      e.preventDefault();
      const scope = scopeInput.trim().replace(/,$/, "");
      if (scope && !values.scopes.includes(scope)) {
        update("scopes", [...values.scopes, scope]);
      }
      setScopeInput("");
    }
  }

  function handleRemoveScope(scope: string) {
    update(
      "scopes",
      values.scopes.filter((s) => s !== scope),
    );
  }

  function handleTestConnection() {
    setTestState("testing");
    // Mock: simulate a 1s test that succeeds
    setTimeout(() => {
      setTestState("success");
      setTimeout(() => setTestState("idle"), 3000);
    }, 1000);
  }

  function handleSave(e: React.FormEvent) {
    e.preventDefault();
    onSave(values);
  }

  const inputStyle = {
    backgroundColor: "var(--thittam-background, #fff)",
    border: "1px solid var(--thittam-border, #e2e8f0)",
    color: "var(--thittam-foreground, #0f172a)",
  };

  return (
    <form onSubmit={handleSave} className="flex flex-col gap-5">
      {/* Issuer URL */}
      <div>
        <label
          className="mb-1 block text-xs font-heading font-medium"
          style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
        >
          Issuer URL
        </label>
        <input
          type="url"
          value={values.issuer_url}
          onChange={(e) => update("issuer_url", e.target.value)}
          placeholder="https://accounts.google.com"
          className="w-full rounded-lg px-3 py-2 font-mono text-sm"
          style={inputStyle}
        />
      </div>

      {/* Client ID */}
      <div>
        <label
          className="mb-1 block text-xs font-heading font-medium"
          style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
        >
          Client ID
        </label>
        <input
          type="text"
          value={values.client_id}
          onChange={(e) => update("client_id", e.target.value)}
          placeholder="your-client-id"
          className="w-full rounded-lg px-3 py-2 font-mono text-sm"
          style={inputStyle}
        />
      </div>

      {/* Client Secret */}
      <div>
        <label
          className="mb-1 block text-xs font-heading font-medium"
          style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
        >
          Client Secret
        </label>
        <input
          type="password"
          value={values.client_secret}
          onChange={(e) => update("client_secret", e.target.value)}
          placeholder="********"
          className="w-full rounded-lg px-3 py-2 font-mono text-sm"
          style={inputStyle}
        />
      </div>

      {/* Scopes (tag input) */}
      <div>
        <label
          className="mb-1 block text-xs font-heading font-medium"
          style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
        >
          Scopes
        </label>
        <div
          className="flex flex-wrap items-center gap-1.5 rounded-lg px-2 py-1.5"
          style={{
            backgroundColor: "var(--thittam-background, #fff)",
            border: "1px solid var(--thittam-border, #e2e8f0)",
            minHeight: "38px",
          }}
        >
          {values.scopes.map((scope) => (
            <span
              key={scope}
              className="inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-xs font-mono"
              style={{
                backgroundColor: "var(--thittam-muted, #f1f5f9)",
                color: "var(--thittam-foreground, #0f172a)",
              }}
            >
              {scope}
              <button
                type="button"
                onClick={() => handleRemoveScope(scope)}
                className="ml-0.5 hover:opacity-70"
                style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
              >
                x
              </button>
            </span>
          ))}
          <input
            type="text"
            value={scopeInput}
            onChange={(e) => setScopeInput(e.target.value)}
            onKeyDown={handleAddScope}
            placeholder={values.scopes.length === 0 ? "Type scope and press Enter" : ""}
            className="flex-1 bg-transparent px-1 py-0.5 font-mono text-sm outline-none"
            style={{ color: "var(--thittam-foreground, #0f172a)", minWidth: "120px" }}
          />
        </div>
      </div>

      {/* Claim fields */}
      <div className="grid gap-4 sm:grid-cols-2">
        <div>
          <label
            className="mb-1 block text-xs font-heading font-medium"
            style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
          >
            Email Claim
          </label>
          <input
            type="text"
            value={values.email_claim}
            onChange={(e) => update("email_claim", e.target.value)}
            className="w-full rounded-lg px-3 py-2 font-mono text-sm"
            style={inputStyle}
          />
        </div>
        <div>
          <label
            className="mb-1 block text-xs font-heading font-medium"
            style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
          >
            Display Name Claim
          </label>
          <input
            type="text"
            value={values.display_name_claim}
            onChange={(e) => update("display_name_claim", e.target.value)}
            className="w-full rounded-lg px-3 py-2 font-mono text-sm"
            style={inputStyle}
          />
        </div>
      </div>

      {/* Groups Claim */}
      <div>
        <label
          className="mb-1 block text-xs font-heading font-medium"
          style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
        >
          Groups Claim
          <span
            className="ml-1 font-normal"
            style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
          >
            (optional)
          </span>
        </label>
        <input
          type="text"
          value={values.groups_claim}
          onChange={(e) => update("groups_claim", e.target.value)}
          placeholder="groups"
          className="w-full rounded-lg px-3 py-2 font-mono text-sm"
          style={inputStyle}
        />
      </div>

      {/* Auto-provision toggle */}
      <div className="flex items-center justify-between">
        <div>
          <span
            className="block text-xs font-heading font-medium"
            style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
          >
            Auto-provision Users (JIT)
          </span>
          <span
            className="text-[11px] font-body"
            style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
          >
            Automatically create user accounts on first SSO login
          </span>
        </div>
        <button
          type="button"
          role="switch"
          aria-checked={values.auto_provision}
          onClick={() => update("auto_provision", !values.auto_provision)}
          className={`relative inline-flex h-6 w-11 shrink-0 cursor-pointer rounded-full transition-colors ${
            values.auto_provision ? "" : "bg-gray-200"
          }`}
          style={
            values.auto_provision
              ? { backgroundColor: "var(--thittam-primary, #3b82f6)" }
              : {}
          }
        >
          <span
            className={`pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow-sm ring-0 transition-transform ${
              values.auto_provision ? "translate-x-5" : "translate-x-0.5"
            } mt-0.5`}
          />
        </button>
      </div>

      {/* Default role for JIT users */}
      {values.auto_provision && (
        <div>
          <label
            className="mb-1 block text-xs font-heading font-medium"
            style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
          >
            Default Role (for new JIT users)
          </label>
          <select
            value={values.default_role}
            onChange={(e) => update("default_role", e.target.value)}
            className="w-full rounded-lg px-3 py-2 font-heading text-sm"
            style={inputStyle}
          >
            {roles.map((r) => (
              <option key={r.id} value={r.name}>
                {formatRoleName(r.name)}
              </option>
            ))}
          </select>
        </div>
      )}

      {/* Actions */}
      <div className="flex items-center gap-3 pt-2">
        <button
          type="button"
          onClick={handleTestConnection}
          disabled={testState === "testing"}
          className="inline-flex items-center gap-2 rounded-lg px-4 py-2 text-sm font-heading font-medium transition-colors hover:opacity-80"
          style={{
            backgroundColor: "var(--thittam-muted, #f1f5f9)",
            color: "var(--thittam-foreground, #0f172a)",
          }}
        >
          {testState === "testing" && (
            <Loader2 className="h-4 w-4 animate-spin" />
          )}
          {testState === "success" && (
            <CheckCircle2 className="h-4 w-4 text-green-600" />
          )}
          {testState === "error" && (
            <XCircle className="h-4 w-4 text-red-600" />
          )}
          {testState === "idle" && null}
          {testState === "testing"
            ? "Testing..."
            : testState === "success"
              ? "Connection OK"
              : testState === "error"
                ? "Connection Failed"
                : "Test Connection"}
        </button>

        <button
          type="submit"
          className="inline-flex items-center gap-2 rounded-lg px-4 py-2 text-sm font-heading font-medium text-white shadow-sm transition-colors hover:opacity-90"
          style={{ backgroundColor: "var(--thittam-primary, #3b82f6)" }}
        >
          <Save className="h-4 w-4" />
          Save Configuration
        </button>
      </div>
    </form>
  );
}
