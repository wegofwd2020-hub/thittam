"use client";

import { useState } from "react";
import { useTheme } from "@/lib/themes/provider";
import {
  LayoutDashboard,
  FolderKanban,
  DollarSign,
  Receipt,
  Package,
  BarChart3,
  Users,
  Settings,
  ChevronDown,
  ChevronRight,
  CheckCircle2,
  ArrowRight,
  Shield,
  Rocket,
  HelpCircle,
  BookOpen,
} from "lucide-react";

// ─── Tutorial Section Component ───────────────────────────────────────────────

interface TutorialSectionProps {
  title: string;
  icon: React.ReactNode;
  children: React.ReactNode;
  defaultOpen?: boolean;
}

function TutorialSection({ title, icon, children, defaultOpen = false }: TutorialSectionProps) {
  const [isOpen, setIsOpen] = useState(defaultOpen);

  return (
    <div className="border rounded-lg overflow-hidden" style={{ borderColor: "var(--thittam-border, #e2e8f0)" }}>
      <button
        onClick={() => setIsOpen(!isOpen)}
        className="w-full flex items-center gap-3 p-4 text-left hover:bg-gray-50 transition-colors"
      >
        <span style={{ color: "var(--thittam-primary, #2563eb)" }}>{icon}</span>
        <span className="font-heading font-semibold text-lg flex-1">{title}</span>
        {isOpen ? <ChevronDown className="h-5 w-5 text-gray-400" /> : <ChevronRight className="h-5 w-5 text-gray-400" />}
      </button>
      {isOpen && (
        <div className="p-4 pt-0 border-t" style={{ borderColor: "var(--thittam-border, #e2e8f0)" }}>
          {children}
        </div>
      )}
    </div>
  );
}

// ─── Flow Step Component ──────────────────────────────────────────────────────

interface FlowStepProps {
  number: number;
  title: string;
  description: string;
  isLast?: boolean;
}

function FlowStep({ number, title, description, isLast = false }: FlowStepProps) {
  return (
    <div className="flex gap-4">
      <div className="flex flex-col items-center">
        <div
          className="w-8 h-8 rounded-full flex items-center justify-center text-white font-mono text-sm font-bold"
          style={{ backgroundColor: "var(--thittam-primary, #2563eb)" }}
        >
          {number}
        </div>
        {!isLast && <div className="w-0.5 h-full min-h-[2rem] bg-gray-200" />}
      </div>
      <div className="pb-6">
        <h4 className="font-heading font-semibold">{title}</h4>
        <p className="font-body text-sm mt-1" style={{ color: "var(--thittam-muted-foreground, #64748b)" }}>
          {description}
        </p>
      </div>
    </div>
  );
}

// ─── Role Card ────────────────────────────────────────────────────────────────

interface RoleCardProps {
  role: string;
  canSee: string;
  canDo: string;
  approvalLimit: string;
}

function RoleCard({ role, canSee, canDo, approvalLimit }: RoleCardProps) {
  return (
    <div className="border rounded-lg p-4" style={{ borderColor: "var(--thittam-border, #e2e8f0)" }}>
      <h4 className="font-heading font-semibold mb-2">{role}</h4>
      <div className="space-y-1 text-sm">
        <p className="font-body"><span className="font-heading text-gray-500">See:</span> {canSee}</p>
        <p className="font-body"><span className="font-heading text-gray-500">Do:</span> {canDo}</p>
        <p className="font-body"><span className="font-heading text-gray-500">Approve:</span> <span className="font-mono">{approvalLimit}</span></p>
      </div>
    </div>
  );
}

// ─── Navigation Item ──────────────────────────────────────────────────────────

interface NavItemProps {
  icon: React.ReactNode;
  label: string;
  description: string;
}

function NavItem({ icon, label, description }: NavItemProps) {
  return (
    <div className="flex items-start gap-3 p-3 rounded-lg hover:bg-gray-50">
      <span style={{ color: "var(--thittam-primary, #2563eb)" }}>{icon}</span>
      <div>
        <span className="font-heading font-medium">{label}</span>
        <p className="font-body text-sm" style={{ color: "var(--thittam-muted-foreground, #64748b)" }}>
          {description}
        </p>
      </div>
    </div>
  );
}

// ─── Main Help Page ───────────────────────────────────────────────────────────

export default function HelpPage() {
  const { entityLabels } = useTheme();

  const projectLabel = entityLabels?.project || "Project";
  const projectsLabel = entityLabels?.projectPlural || "Projects";
  const teamLabel = entityLabels?.teamMemberPlural || "Team Members";
  const phaseLabel = entityLabels?.phasePlural || "Phases";

  return (
    <div className="max-w-4xl mx-auto p-6">
      {/* Header */}
      <div className="mb-8">
        <div className="flex items-center gap-3 mb-2">
          <BookOpen className="h-8 w-8" style={{ color: "var(--thittam-primary, #2563eb)" }} />
          <h1 className="text-3xl font-heading font-bold">Help & Tutorials</h1>
        </div>
        <p className="font-body text-lg" style={{ color: "var(--thittam-muted-foreground, #64748b)" }}>
          Learn how to use Thittam to manage your {projectsLabel.toLowerCase()}, budgets, expenses, and team.
        </p>
      </div>

      {/* Tutorial Sections */}
      <div className="space-y-4">

        {/* Getting Started */}
        <TutorialSection title="Getting Started" icon={<Rocket className="h-5 w-5" />} defaultOpen>
          <div className="mt-4">
            <FlowStep number={1} title="Accept your invitation" description="Click the link in your invitation email to create your account." />
            <FlowStep number={2} title="Set your password" description="Choose a secure password, or sign in with your company's SSO (Google, Microsoft, Okta)." />
            <FlowStep number={3} title="Explore the dashboard" description={`Your dashboard shows an overview of all ${projectsLabel.toLowerCase()}, budget health, pending approvals, and team utilization.`} />
            <FlowStep number={4} title={`Create your first ${projectLabel}`} description={`Go to ${projectsLabel} → New ${projectLabel}. Fill in the title, description, and dates.`} isLast />
          </div>
        </TutorialSection>

        {/* Daily Workflow */}
        <TutorialSection title="Your Daily Workflow" icon={<CheckCircle2 className="h-5 w-5" />}>
          <div className="mt-4 grid grid-cols-1 md:grid-cols-2 gap-4">
            <div className="border rounded-lg p-4" style={{ borderColor: "var(--thittam-border, #e2e8f0)" }}>
              <h4 className="font-heading font-semibold mb-2 flex items-center gap-2">
                <span className="w-6 h-6 rounded-full bg-blue-100 text-blue-600 flex items-center justify-center text-xs font-mono">1</span>
                View Dashboard
              </h4>
              <p className="font-body text-sm" style={{ color: "var(--thittam-muted-foreground, #64748b)" }}>
                Check KPI cards, budget health, and pending approvals at a glance.
              </p>
            </div>
            <div className="border rounded-lg p-4" style={{ borderColor: "var(--thittam-border, #e2e8f0)" }}>
              <h4 className="font-heading font-semibold mb-2 flex items-center gap-2">
                <span className="w-6 h-6 rounded-full bg-blue-100 text-blue-600 flex items-center justify-center text-xs font-mono">2</span>
                Plan Budgets
              </h4>
              <p className="font-body text-sm" style={{ color: "var(--thittam-muted-foreground, #64748b)" }}>
                Create budgets from templates or scratch. Add line items by category. Submit for approval.
              </p>
            </div>
            <div className="border rounded-lg p-4" style={{ borderColor: "var(--thittam-border, #e2e8f0)" }}>
              <h4 className="font-heading font-semibold mb-2 flex items-center gap-2">
                <span className="w-6 h-6 rounded-full bg-blue-100 text-blue-600 flex items-center justify-center text-xs font-mono">3</span>
                Submit Expenses
              </h4>
              <p className="font-body text-sm" style={{ color: "var(--thittam-muted-foreground, #64748b)" }}>
                Select a category, attach PO if required, enter amount. The system checks budget availability automatically.
              </p>
            </div>
            <div className="border rounded-lg p-4" style={{ borderColor: "var(--thittam-border, #e2e8f0)" }}>
              <h4 className="font-heading font-semibold mb-2 flex items-center gap-2">
                <span className="w-6 h-6 rounded-full bg-blue-100 text-blue-600 flex items-center justify-center text-xs font-mono">4</span>
                Track Reports
              </h4>
              <p className="font-body text-sm" style={{ color: "var(--thittam-muted-foreground, #64748b)" }}>
                View budget vs actual, burn rate trends, and export reports as CSV.
              </p>
            </div>
          </div>
        </TutorialSection>

        {/* Project Lifecycle */}
        <TutorialSection title={`${projectLabel} Lifecycle`} icon={<FolderKanban className="h-5 w-5" />}>
          <div className="mt-4">
            <p className="font-body mb-4" style={{ color: "var(--thittam-muted-foreground, #64748b)" }}>
              Each {projectLabel.toLowerCase()} moves through {phaseLabel.toLowerCase()} defined by your industry.
              The system enforces valid transitions — you can&apos;t skip steps.
            </p>
            <div className="bg-gray-50 rounded-lg p-4">
              <h4 className="font-heading font-semibold mb-3">How {phaseLabel} Work</h4>
              <div className="space-y-2 font-body text-sm">
                <div className="flex items-center gap-2">
                  <CheckCircle2 className="h-4 w-4 text-green-500" />
                  <span>Each {projectLabel.toLowerCase()} starts at the first phase</span>
                </div>
                <div className="flex items-center gap-2">
                  <ArrowRight className="h-4 w-4 text-blue-500" />
                  <span>Transition buttons show only valid next steps</span>
                </div>
                <div className="flex items-center gap-2">
                  <CheckCircle2 className="h-4 w-4 text-green-500" />
                  <span>Some phases are billable (marked with a badge)</span>
                </div>
                <div className="flex items-center gap-2">
                  <CheckCircle2 className="h-4 w-4 text-green-500" />
                  <span>The timeline shows progress visually</span>
                </div>
              </div>
            </div>
          </div>
        </TutorialSection>

        {/* Budget & Expenses */}
        <TutorialSection title="Budgets & Expenses" icon={<DollarSign className="h-5 w-5" />}>
          <div className="mt-4 space-y-4">
            <div>
              <h4 className="font-heading font-semibold mb-2">Budget Flow</h4>
              <div className="flex items-center gap-2 flex-wrap font-heading text-sm">
                <span className="px-3 py-1 bg-gray-100 rounded-full">Draft</span>
                <ArrowRight className="h-4 w-4 text-gray-400" />
                <span className="px-3 py-1 bg-blue-100 text-blue-700 rounded-full">Submitted</span>
                <ArrowRight className="h-4 w-4 text-gray-400" />
                <span className="px-3 py-1 bg-green-100 text-green-700 rounded-full">Approved</span>
                <ArrowRight className="h-4 w-4 text-gray-400" />
                <span className="px-3 py-1 bg-purple-100 text-purple-700 rounded-full">Locked</span>
              </div>
            </div>
            <div>
              <h4 className="font-heading font-semibold mb-2">Expense Checks (Automatic)</h4>
              <ul className="font-body text-sm space-y-1" style={{ color: "var(--thittam-muted-foreground, #64748b)" }}>
                <li>✓ Category is valid for your industry vertical</li>
                <li>✓ Purchase order attached (if category requires it)</li>
                <li>✓ Budget line has sufficient funds</li>
                <li>✓ Approver has sufficient authority (role-based limits)</li>
                <li>✓ Dual approval triggered for large amounts</li>
              </ul>
            </div>
          </div>
        </TutorialSection>

        {/* Roles */}
        <TutorialSection title="Roles & Permissions" icon={<Shield className="h-5 w-5" />}>
          <div className="mt-4 grid grid-cols-1 md:grid-cols-2 gap-3">
            <RoleCard role="Super Admin" canSee="Everything" canDo="Everything" approvalLimit="Unlimited" />
            <RoleCard role="Executive Producer" canSee="Everything" canDo="Create/Edit" approvalLimit="Up to ₹10L" />
            <RoleCard role="Line Producer" canSee={`Own ${projectsLabel.toLowerCase()}`} canDo="Create/Edit" approvalLimit="Up to ₹2L" />
            <RoleCard role="Production Accountant" canSee="Financials" canDo="Budgets/Reports" approvalLimit="None" />
            <RoleCard role="Production Manager" canSee={`Own ${projectsLabel.toLowerCase()}`} canDo={`${phaseLabel}/Crew`} approvalLimit="None" />
            <RoleCard role={entityLabels?.teamMember || "Crew Member"} canSee="Own assignments" canDo="Submit expenses" approvalLimit="None" />
          </div>
          <p className="font-body text-xs mt-3" style={{ color: "var(--thittam-muted-foreground, #64748b)" }}>
            💡 Approval limits vary by industry vertical. Software development has different thresholds than film production.
          </p>
        </TutorialSection>

        {/* Navigation */}
        <TutorialSection title="Navigation Guide" icon={<HelpCircle className="h-5 w-5" />}>
          <div className="mt-4 space-y-1">
            <NavItem icon={<LayoutDashboard className="h-5 w-5" />} label="Dashboard" description="KPI cards, charts, pending approvals, team utilization" />
            <NavItem icon={<FolderKanban className="h-5 w-5" />} label={projectsLabel} description={`List, create, manage ${projectsLabel.toLowerCase()} with ${phaseLabel.toLowerCase()} and ${teamLabel.toLowerCase()}`} />
            <NavItem icon={<DollarSign className="h-5 w-5" />} label="Budgets" description="Create from templates, manage line items, approval workflow" />
            <NavItem icon={<Receipt className="h-5 w-5" />} label="Expenses" description="Submit expenses, manage POs, petty cash, track approvals" />
            <NavItem icon={<Package className="h-5 w-5" />} label="Inventory" description="Track assets, check out/in equipment, damage reports" />
            <NavItem icon={<BarChart3 className="h-5 w-5" />} label="Reports" description="Industry-specific reports, budget variance, CSV export" />
            <NavItem icon={<Users className="h-5 w-5" />} label="Team" description={`${teamLabel} list, availability, assignments`} />
            <NavItem icon={<Settings className="h-5 w-5" />} label="Settings" description="Users, roles, auth configuration, theme customization" />
          </div>
        </TutorialSection>

        {/* Keyboard Shortcuts */}
        <TutorialSection title="Keyboard Shortcuts" icon={<span className="font-mono text-sm">⌨</span>}>
          <div className="mt-4">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b" style={{ borderColor: "var(--thittam-border, #e2e8f0)" }}>
                  <th className="text-left py-2 font-heading">Shortcut</th>
                  <th className="text-left py-2 font-heading">Action</th>
                </tr>
              </thead>
              <tbody className="font-body">
                <tr className="border-b" style={{ borderColor: "var(--thittam-border, #e2e8f0)" }}>
                  <td className="py-2"><kbd className="font-mono bg-gray-100 px-2 py-0.5 rounded text-xs">/</kbd></td>
                  <td className="py-2">Focus search bar</td>
                </tr>
                <tr className="border-b" style={{ borderColor: "var(--thittam-border, #e2e8f0)" }}>
                  <td className="py-2"><kbd className="font-mono bg-gray-100 px-2 py-0.5 rounded text-xs">n</kbd></td>
                  <td className="py-2">New (create in current section)</td>
                </tr>
                <tr className="border-b" style={{ borderColor: "var(--thittam-border, #e2e8f0)" }}>
                  <td className="py-2"><kbd className="font-mono bg-gray-100 px-2 py-0.5 rounded text-xs">Esc</kbd></td>
                  <td className="py-2">Close dialog / go back</td>
                </tr>
                <tr>
                  <td className="py-2"><kbd className="font-mono bg-gray-100 px-2 py-0.5 rounded text-xs">Ctrl+A</kbd></td>
                  <td className="py-2">Toggle accessibility (dyslexia mode)</td>
                </tr>
              </tbody>
            </table>
          </div>
        </TutorialSection>
      </div>

      {/* Footer */}
      <div className="mt-8 p-4 bg-gray-50 rounded-lg text-center">
        <p className="font-body text-sm" style={{ color: "var(--thittam-muted-foreground, #64748b)" }}>
          Need more help? Contact <a href="mailto:support@thittam.io" className="underline" style={{ color: "var(--thittam-primary, #2563eb)" }}>support@thittam.io</a>
        </p>
      </div>
    </div>
  );
}
