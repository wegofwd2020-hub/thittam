"use client";

import { useMemo, useState } from "react";
import Link from "next/link";
import { Check, ChevronDown, ChevronUp, Copy, Mail, Phone, Search } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import type { Party, PartyContact } from "@/lib/api/ledger";

type TypeFilter = "all" | "customer" | "supplier";

const TYPE_LABEL: Record<string, string> = {
  customer: "Customer",
  supplier: "Vendor",
  both: "Customer & vendor",
};

const FILTER_LABEL: Record<TypeFilter, string> = {
  all: "All",
  customer: "Customers",
  supplier: "Vendors",
};

// "customer" matches customers and parties that are both; "supplier" likewise.
function matchesType(p: Party, f: TypeFilter): boolean {
  if (f === "all") return true;
  if (f === "customer") return p.party_type === "customer" || p.party_type === "both";
  return p.party_type === "supplier" || p.party_type === "both";
}

function matchesQuery(p: Party, q: string): boolean {
  if (!q) return true;
  const haystack = [
    p.name,
    p.email,
    p.phone,
    p.tax_id,
    p.address,
    ...p.contacts.flatMap((c) => [c.name, c.designation, c.email, c.phone]),
  ]
    .join(" ")
    .toLowerCase();
  return haystack.includes(q);
}

function primaryContact(p: Party): PartyContact | undefined {
  return p.contacts.find((c) => c.is_primary) ?? p.contacts[0];
}

export function PartyRolodex({ parties }: { parties: Party[] }) {
  const [query, setQuery] = useState("");
  const [typeFilter, setTypeFilter] = useState<TypeFilter>("all");

  const visible = useMemo(() => {
    const q = query.trim().toLowerCase();
    return parties.filter((p) => matchesType(p, typeFilter) && matchesQuery(p, q));
  }, [parties, typeFilter, query]);

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center gap-3">
        <div className="relative w-full max-w-xs">
          <Search className="pointer-events-none absolute left-2 top-2.5 h-4 w-4 text-muted-foreground" />
          <Input
            className="pl-8"
            placeholder="Search name, contact, email, phone"
            aria-label="Search customers and vendors"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
          />
        </div>

        <div className="flex gap-1">
          {(Object.keys(FILTER_LABEL) as TypeFilter[]).map((f) => (
            <Button
              key={f}
              size="sm"
              variant={typeFilter === f ? "default" : "outline"}
              onClick={() => setTypeFilter(f)}
            >
              {FILTER_LABEL[f]}
            </Button>
          ))}
        </div>

        <span className="text-sm text-muted-foreground">
          {visible.length} of {parties.length}
        </span>
      </div>

      {visible.length === 0 ? (
        <p className="py-8 text-center text-sm text-muted-foreground">
          {parties.length === 0 ? "No customers or vendors yet." : "No matches."}
        </p>
      ) : (
        <ul className="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-3">
          {visible.map((p) => (
            <PartyCard key={p.id} party={p} />
          ))}
        </ul>
      )}
    </div>
  );
}

function PartyCard({ party }: { party: Party }) {
  const [expanded, setExpanded] = useState(false);
  const primary = primaryContact(party);
  const others = party.contacts.filter((c) => c !== primary);

  return (
    <li className="flex flex-col gap-3 rounded-lg border p-4">
      <div className="flex items-start justify-between gap-2">
        <div className="min-w-0">
          <h3 className="truncate font-medium">
            <Link href={`/ledger/parties/${party.id}`} className="hover:underline">
              {party.name}
            </Link>
          </h3>
          <p className="text-xs text-muted-foreground">
            {TYPE_LABEL[party.party_type] ?? party.party_type}
            {party.default_currency ? ` · ${party.default_currency}` : ""}
          </p>
        </div>
        {!party.is_active && (
          <span className="rounded bg-gray-100 px-2 py-0.5 text-xs text-gray-600">Inactive</span>
        )}
      </div>

      {primary ? (
        <ContactBlock contact={primary} label="Primary contact" />
      ) : (
        <p className="text-sm text-muted-foreground">No contacts yet.</p>
      )}

      {others.length > 0 && (
        <div className="space-y-3">
          <button
            type="button"
            className="flex items-center gap-1 text-xs text-muted-foreground hover:underline"
            aria-expanded={expanded}
            onClick={() => setExpanded((v) => !v)}
          >
            {expanded ? <ChevronUp className="h-3 w-3" /> : <ChevronDown className="h-3 w-3" />}
            {expanded ? "Hide" : "Show"} {others.length}{" "}
            {others.length === 1 ? "more contact" : "more contacts"}
          </button>
          {expanded && (
            <div className="space-y-3 border-t pt-3">
              {others.map((c) => (
                <ContactBlock key={c.id} contact={c} />
              ))}
            </div>
          )}
        </div>
      )}

      {(party.email || party.phone) && (
        <div className="space-y-1 border-t pt-3">
          <p className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
            Organization
          </p>
          <ContactLinks email={party.email} phone={party.phone} />
        </div>
      )}
    </li>
  );
}

function ContactBlock({ contact, label }: { contact: PartyContact; label?: string }) {
  return (
    <div className="space-y-1 text-sm">
      {label && (
        <p className="text-xs font-medium uppercase tracking-wide text-muted-foreground">{label}</p>
      )}
      <p className="font-medium">
        {contact.name}
        {contact.designation && (
          <span className="font-normal text-muted-foreground"> · {contact.designation}</span>
        )}
      </p>
      <ContactLinks email={contact.email} phone={contact.phone} />
    </div>
  );
}

function ContactLinks({ email, phone }: { email: string; phone: string }) {
  return (
    <div className="flex flex-col gap-1 text-sm">
      {email && <CopyableValue icon={<Mail className="h-3.5 w-3.5" />} value={email} />}
      {phone && <CopyableValue icon={<Phone className="h-3.5 w-3.5" />} value={phone} />}
    </div>
  );
}

// Shows a value as plain text with a copy button. It does not open a mail or
// dialer app.
function CopyableValue({ icon, value }: { icon: React.ReactNode; value: string }) {
  const [copied, setCopied] = useState(false);

  async function copy() {
    try {
      await navigator.clipboard.writeText(value);
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    } catch {
      // Clipboard can be unavailable (insecure origin or denied permission).
      setCopied(false);
    }
  }

  return (
    <div className="flex items-center gap-2">
      <span className="text-muted-foreground">{icon}</span>
      <span className="min-w-0 truncate">{value}</span>
      <button
        type="button"
        className="ml-auto shrink-0 rounded p-1 text-muted-foreground hover:bg-muted"
        aria-label={`Copy ${value}`}
        title="Copy"
        onClick={copy}
      >
        {copied ? <Check className="h-3.5 w-3.5" /> : <Copy className="h-3.5 w-3.5" />}
      </button>
    </div>
  );
}
