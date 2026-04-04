"use client";

import { useCallback, useState } from "react";
import { ArrowLeft, Copy, Eye, EyeOff, ExternalLink, Plus } from "lucide-react";
import { toast } from "sonner";

import { verticals } from "@/lib/themes/verticals";
import { VerticalCard } from "@/components/platform/vertical-card";
import { DemoPreviewAccordion } from "@/components/platform/demo-preview-accordion";
import { ProvisionProgress } from "@/components/platform/provision-progress";
import type { DemoPreview, ProvisionResult } from "@/lib/api/platform";

// ---------------------------------------------------------------------------
// Per-vertical mock data — mirrors pkg/demo templates + seed migrations.
// ---------------------------------------------------------------------------

const VERTICAL_CARD_INFO: Record<
  string,
  {
    description: string;
    companyName: string;
    userCount: number;
    projectCount: number;
    icon: string;
  }
> = {
  "movie-production": {
    description:
      "Film and television production management with crew scheduling, location tracking, and post-production workflows.",
    companyName: "Sunrise Studios",
    userCount: 8,
    projectCount: 3,
    icon: "Clapperboard",
  },
  "software-development": {
    description:
      "Software project management with milestone tracking, sprint planning, cloud infrastructure, and deployment pipelines.",
    companyName: "TechForge Solutions",
    userCount: 7,
    projectCount: 4,
    icon: "FolderKanban",
  },
  construction: {
    description:
      "Construction contract management with work packages, site tracking, material procurement, and safety compliance.",
    companyName: "BuildRight Engineers",
    userCount: 6,
    projectCount: 3,
    icon: "Building2",
  },
  "events-management": {
    description:
      "Event planning and execution with venue management, vendor coordination, ticketing, and guest experience tracking.",
    companyName: "Grandeur Events",
    userCount: 6,
    projectCount: 3,
    icon: "CalendarDays",
  },
};

// -- Chart of accounts (common across verticals, representative subset) --
const COMMON_ACCOUNTS = [
  { code: "1000", name: "Cash & Equivalents", account_type: "Asset" },
  { code: "1100", name: "Accounts Receivable", account_type: "Asset" },
  { code: "1200", name: "Prepaid Expenses", account_type: "Asset" },
  { code: "2000", name: "Accounts Payable", account_type: "Liability" },
  { code: "2100", name: "Accrued Liabilities", account_type: "Liability" },
  { code: "3000", name: "Owner Equity", account_type: "Equity" },
  { code: "3100", name: "Retained Earnings", account_type: "Equity" },
  { code: "4000", name: "Revenue", account_type: "Revenue" },
  { code: "5000", name: "Cost of Goods Sold", account_type: "Expense" },
  { code: "6000", name: "Operating Expenses", account_type: "Expense" },
  { code: "6100", name: "Payroll Expenses", account_type: "Expense" },
  { code: "6200", name: "Equipment & Rentals", account_type: "Expense" },
];

function buildMockPreview(verticalId: string): DemoPreview {
  const info = VERTICAL_CARD_INFO[verticalId];
  const vDef = verticals[verticalId];
  if (!info || !vDef) throw new Error(`Unknown vertical: ${verticalId}`);

  const slug = info.companyName.toLowerCase().replace(/\s+/g, "-");

  const base: Pick<DemoPreview, "preview_token" | "tenant" | "vertical_id" | "vertical_name" | "entity_labels"> = {
    preview_token: `demo_${verticalId}_${Date.now()}`,
    tenant: { name: info.companyName, slug, plan: "professional" },
    vertical_id: verticalId,
    vertical_name: vDef.theme.name,
    entity_labels: {
      project: vDef.entityLabels.project,
      projectPlural: vDef.entityLabels.projectPlural,
      phase: vDef.entityLabels.phase,
      teamMember: vDef.entityLabels.teamMember,
    },
  };

  if (verticalId === "movie-production") {
    return {
      ...base,
      chart_of_accounts: [
        ...COMMON_ACCOUNTS,
        { code: "6300", name: "Location & Permits", account_type: "Expense" },
        { code: "6400", name: "Talent & Casting", account_type: "Expense" },
        { code: "6500", name: "Post-Production", account_type: "Expense" },
      ],
      users: [
        { name: "Priya Sharma", email: "priya@sunrise-studios.demo", role: "admin", department: "Executive", temp_password: "Demo@Sun2024!" },
        { name: "James Chen", email: "james@sunrise-studios.demo", role: "producer", department: "Production", temp_password: "Demo@Sun2024!" },
        { name: "Maria Lopez", email: "maria@sunrise-studios.demo", role: "finance_director", department: "Finance", temp_password: "Demo@Sun2024!" },
        { name: "Alex Kim", email: "alex@sunrise-studios.demo", role: "line_producer", department: "Production", temp_password: "Demo@Sun2024!" },
        { name: "Sarah Johnson", email: "sarah@sunrise-studios.demo", role: "dept_head", department: "Art", temp_password: "Demo@Sun2024!" },
        { name: "Mike Davis", email: "mike@sunrise-studios.demo", role: "dept_head", department: "Camera", temp_password: "Demo@Sun2024!" },
        { name: "Anika Patel", email: "anika@sunrise-studios.demo", role: "coordinator", department: "Production", temp_password: "Demo@Sun2024!" },
        { name: "Tom Wilson", email: "tom@sunrise-studios.demo", role: "viewer", department: "Post-Production", temp_password: "Demo@Sun2024!" },
      ],
      budget_categories: [
        { id: "cat-above-line", label: "Above the Line", description: "Writer, director, producer, and principal cast fees" },
        { id: "cat-below-line", label: "Below the Line", description: "Crew, equipment, locations, and day-to-day production costs" },
        { id: "cat-post", label: "Post-Production", description: "Editing, VFX, sound design, color grading, and music" },
        { id: "cat-marketing", label: "Marketing & Distribution", description: "Festival submissions, PR, marketing materials" },
        { id: "cat-insurance", label: "Insurance & Legal", description: "Completion bond, E&O, production insurance, legal fees" },
      ],
      budget_templates: [
        { name: "Feature Film", description: "Standard feature film budget with 3-phase breakdown", line_items: Array(24).fill({ category_id: "cat-above-line", description: "Template item", default_amount: "10000.00" }) },
        { name: "TV Pilot", description: "Television pilot episode budget", line_items: Array(18).fill({ category_id: "cat-below-line", description: "Template item", default_amount: "5000.00" }) },
        { name: "Short Film", description: "Low-budget short film template", line_items: Array(12).fill({ category_id: "cat-above-line", description: "Template item", default_amount: "2000.00" }) },
      ],
      expense_categories: [
        { id: "ec-talent", label: "Talent & Casting", tax_treatment: "deductible", requires_po: true },
        { id: "ec-crew", label: "Crew Labor", tax_treatment: "deductible", requires_po: false },
        { id: "ec-equipment", label: "Equipment Rental", tax_treatment: "deductible", requires_po: true },
        { id: "ec-locations", label: "Locations & Permits", tax_treatment: "deductible", requires_po: true },
        { id: "ec-catering", label: "Catering & Craft Services", tax_treatment: "deductible", requires_po: false },
        { id: "ec-transport", label: "Transportation", tax_treatment: "deductible", requires_po: false },
        { id: "ec-post", label: "Post-Production Services", tax_treatment: "deductible", requires_po: true },
        { id: "ec-petty", label: "Petty Cash", tax_treatment: "deductible", requires_po: false },
      ],
      approval_workflow: {
        limits: [
          { role: "Department Head", max_amount: "5000.00" },
          { role: "Production Manager", max_amount: "25000.00" },
          { role: "Finance Director", max_amount: "100000.00" },
          { role: "Executive Producer", max_amount: "500000.00" },
        ],
        dual_approval_above: "50000.00",
      },
      inventory_categories: [
        { id: "inv-camera", label: "Camera Equipment", is_trackable: true },
        { id: "inv-lighting", label: "Lighting & Grip", is_trackable: true },
        { id: "inv-sound", label: "Sound Equipment", is_trackable: true },
        { id: "inv-props", label: "Props", is_trackable: true },
        { id: "inv-wardrobe", label: "Wardrobe", is_trackable: true },
        { id: "inv-vehicles", label: "Vehicles", is_trackable: true },
        { id: "inv-consumables", label: "Consumables", is_trackable: false },
      ],
      projects: [
        { title: "The Last Horizon", description: "Sci-fi feature film, 45-day shoot", phase: "Production", budget_total: "2500000.00", expense_count: 142, status: "active" },
        { title: "City Lights (Season 2)", description: "8-episode dramedy series", phase: "Pre-Production", budget_total: "4800000.00", expense_count: 67, status: "planning" },
        { title: "Echoes", description: "Independent short film, 10-day shoot", phase: "Post-Production", budget_total: "85000.00", expense_count: 38, status: "active" },
      ],
    };
  }

  if (verticalId === "software-development") {
    return {
      ...base,
      chart_of_accounts: [
        ...COMMON_ACCOUNTS,
        { code: "6300", name: "Cloud Infrastructure", account_type: "Expense" },
        { code: "6400", name: "Software Licenses", account_type: "Expense" },
        { code: "6500", name: "Contractor Services", account_type: "Expense" },
      ],
      users: [
        { name: "David Park", email: "david@techforge.demo", role: "admin", department: "Engineering", temp_password: "Demo@Tech2024!" },
        { name: "Lisa Wang", email: "lisa@techforge.demo", role: "project_manager", department: "Product", temp_password: "Demo@Tech2024!" },
        { name: "Ryan Cooper", email: "ryan@techforge.demo", role: "finance_director", department: "Finance", temp_password: "Demo@Tech2024!" },
        { name: "Emma Taylor", email: "emma@techforge.demo", role: "tech_lead", department: "Engineering", temp_password: "Demo@Tech2024!" },
        { name: "Carlos Reyes", email: "carlos@techforge.demo", role: "dept_head", department: "Design", temp_password: "Demo@Tech2024!" },
        { name: "Nina Petrova", email: "nina@techforge.demo", role: "coordinator", department: "Operations", temp_password: "Demo@Tech2024!" },
        { name: "Jake Brooks", email: "jake@techforge.demo", role: "viewer", department: "QA", temp_password: "Demo@Tech2024!" },
      ],
      budget_categories: [
        { id: "cat-personnel", label: "Personnel", description: "Full-time engineers, designers, product managers" },
        { id: "cat-infra", label: "Infrastructure", description: "Cloud hosting, CI/CD, monitoring, databases" },
        { id: "cat-tools", label: "Tools & Licenses", description: "IDEs, SaaS subscriptions, design tools" },
        { id: "cat-contractors", label: "Contractors", description: "External developers, consultants, agencies" },
        { id: "cat-qa", label: "QA & Testing", description: "Testing infrastructure, security audits, load testing" },
      ],
      budget_templates: [
        { name: "SaaS Product", description: "Standard SaaS product development budget", line_items: Array(20).fill({ category_id: "cat-personnel", description: "Template item", default_amount: "8000.00" }) },
        { name: "Mobile App", description: "iOS + Android app development", line_items: Array(16).fill({ category_id: "cat-personnel", description: "Template item", default_amount: "6000.00" }) },
        { name: "Infrastructure Migration", description: "Cloud migration project budget", line_items: Array(14).fill({ category_id: "cat-infra", description: "Template item", default_amount: "12000.00" }) },
      ],
      expense_categories: [
        { id: "ec-cloud", label: "Cloud Services (AWS/GCP)", tax_treatment: "deductible", requires_po: true },
        { id: "ec-saas", label: "SaaS Subscriptions", tax_treatment: "deductible", requires_po: false },
        { id: "ec-contractors", label: "Contract Development", tax_treatment: "deductible", requires_po: true },
        { id: "ec-hardware", label: "Hardware & Devices", tax_treatment: "capitalizable", requires_po: true },
        { id: "ec-training", label: "Training & Conferences", tax_treatment: "deductible", requires_po: false },
        { id: "ec-security", label: "Security & Compliance", tax_treatment: "deductible", requires_po: true },
      ],
      approval_workflow: {
        limits: [
          { role: "Team Lead", max_amount: "5000.00" },
          { role: "Engineering Manager", max_amount: "25000.00" },
          { role: "VP Engineering", max_amount: "100000.00" },
          { role: "CTO", max_amount: "500000.00" },
        ],
        dual_approval_above: "50000.00",
      },
      inventory_categories: [
        { id: "inv-laptops", label: "Laptops & Workstations", is_trackable: true },
        { id: "inv-monitors", label: "Monitors & Peripherals", is_trackable: true },
        { id: "inv-servers", label: "Server Hardware", is_trackable: true },
        { id: "inv-networking", label: "Networking Equipment", is_trackable: true },
        { id: "inv-licenses", label: "Software Licenses", is_trackable: false },
        { id: "inv-office", label: "Office Supplies", is_trackable: false },
      ],
      projects: [
        { title: "Forge Platform v3", description: "Major platform rewrite with microservices architecture", phase: "Milestone 3", budget_total: "1200000.00", expense_count: 89, status: "active" },
        { title: "Mobile App (iOS/Android)", description: "Native mobile clients for core platform features", phase: "Milestone 1", budget_total: "450000.00", expense_count: 34, status: "active" },
        { title: "Data Pipeline Migration", description: "Move from batch to real-time streaming", phase: "Planning", budget_total: "320000.00", expense_count: 12, status: "planning" },
        { title: "SOC-2 Compliance", description: "Security audit and compliance certification", phase: "Milestone 2", budget_total: "180000.00", expense_count: 21, status: "active" },
      ],
    };
  }

  if (verticalId === "construction") {
    return {
      ...base,
      chart_of_accounts: [
        ...COMMON_ACCOUNTS,
        { code: "6300", name: "Materials & Supplies", account_type: "Expense" },
        { code: "6400", name: "Subcontractor Services", account_type: "Expense" },
        { code: "6500", name: "Permits & Inspections", account_type: "Expense" },
      ],
      users: [
        { name: "Robert Singh", email: "robert@buildright.demo", role: "admin", department: "Executive", temp_password: "Demo@Build2024!" },
        { name: "Jennifer Wu", email: "jennifer@buildright.demo", role: "project_manager", department: "Operations", temp_password: "Demo@Build2024!" },
        { name: "Mark Thompson", email: "mark@buildright.demo", role: "finance_director", department: "Finance", temp_password: "Demo@Build2024!" },
        { name: "Angela Cruz", email: "angela@buildright.demo", role: "site_manager", department: "Field", temp_password: "Demo@Build2024!" },
        { name: "Dave Miller", email: "dave@buildright.demo", role: "estimator", department: "Pre-Construction", temp_password: "Demo@Build2024!" },
        { name: "Karen Lee", email: "karen@buildright.demo", role: "coordinator", department: "Safety", temp_password: "Demo@Build2024!" },
      ],
      budget_categories: [
        { id: "cat-labor", label: "Labor", description: "Direct labor, overtime, benefits for site workers" },
        { id: "cat-materials", label: "Materials", description: "Concrete, steel, lumber, finishings, fixtures" },
        { id: "cat-equipment", label: "Equipment", description: "Heavy equipment rental, fuel, maintenance" },
        { id: "cat-subcontractors", label: "Subcontractors", description: "Electrical, plumbing, HVAC, specialty trades" },
        { id: "cat-overhead", label: "Overhead & Site Costs", description: "Site office, temporary facilities, safety equipment" },
      ],
      budget_templates: [
        { name: "Commercial Building", description: "Mid-rise commercial construction budget", line_items: Array(28).fill({ category_id: "cat-labor", description: "Template item", default_amount: "15000.00" }) },
        { name: "Residential Complex", description: "Multi-unit residential development", line_items: Array(22).fill({ category_id: "cat-materials", description: "Template item", default_amount: "20000.00" }) },
        { name: "Infrastructure", description: "Roads, bridges, and utility projects", line_items: Array(18).fill({ category_id: "cat-equipment", description: "Template item", default_amount: "25000.00" }) },
      ],
      expense_categories: [
        { id: "ec-materials", label: "Raw Materials", tax_treatment: "deductible", requires_po: true },
        { id: "ec-subcontract", label: "Subcontractor Invoices", tax_treatment: "deductible", requires_po: true },
        { id: "ec-equipment-rental", label: "Equipment Rental", tax_treatment: "deductible", requires_po: true },
        { id: "ec-permits", label: "Permits & Fees", tax_treatment: "deductible", requires_po: false },
        { id: "ec-safety", label: "Safety Equipment", tax_treatment: "deductible", requires_po: false },
        { id: "ec-fuel", label: "Fuel & Utilities", tax_treatment: "deductible", requires_po: false },
        { id: "ec-insurance", label: "Insurance", tax_treatment: "deductible", requires_po: true },
      ],
      approval_workflow: {
        limits: [
          { role: "Foreman", max_amount: "5000.00" },
          { role: "Site Manager", max_amount: "25000.00" },
          { role: "Project Director", max_amount: "150000.00" },
          { role: "VP Operations", max_amount: "500000.00" },
        ],
        dual_approval_above: "75000.00",
      },
      inventory_categories: [
        { id: "inv-heavy", label: "Heavy Equipment", is_trackable: true },
        { id: "inv-tools", label: "Power Tools", is_trackable: true },
        { id: "inv-safety", label: "Safety Gear", is_trackable: true },
        { id: "inv-vehicles", label: "Vehicles", is_trackable: true },
        { id: "inv-materials", label: "Bulk Materials", is_trackable: false },
        { id: "inv-consumables", label: "Consumables", is_trackable: false },
      ],
      projects: [
        { title: "Metro Tower Phase 2", description: "22-story mixed-use commercial tower", phase: "Foundation", budget_total: "18500000.00", expense_count: 234, status: "active" },
        { title: "Riverside Apartments", description: "120-unit residential complex with amenities", phase: "Framing", budget_total: "8200000.00", expense_count: 156, status: "active" },
        { title: "Highway 42 Bridge Repair", description: "Structural reinforcement and deck replacement", phase: "Pre-Construction", budget_total: "3400000.00", expense_count: 45, status: "planning" },
      ],
    };
  }

  // events-management (default fallthrough)
  return {
    ...base,
    chart_of_accounts: [
      ...COMMON_ACCOUNTS,
      { code: "6300", name: "Venue & Facilities", account_type: "Expense" },
      { code: "6400", name: "Entertainment & Talent", account_type: "Expense" },
      { code: "6500", name: "Catering & Hospitality", account_type: "Expense" },
    ],
    users: [
      { name: "Sophie Martinez", email: "sophie@grandeur-events.demo", role: "admin", department: "Executive", temp_password: "Demo@Grand2024!" },
      { name: "Chris Anderson", email: "chris@grandeur-events.demo", role: "event_director", department: "Events", temp_password: "Demo@Grand2024!" },
      { name: "Diana Fox", email: "diana@grandeur-events.demo", role: "finance_director", department: "Finance", temp_password: "Demo@Grand2024!" },
      { name: "Raj Gupta", email: "raj@grandeur-events.demo", role: "coordinator", department: "Logistics", temp_password: "Demo@Grand2024!" },
      { name: "Olivia Brown", email: "olivia@grandeur-events.demo", role: "vendor_manager", department: "Procurement", temp_password: "Demo@Grand2024!" },
      { name: "Leo Tanaka", email: "leo@grandeur-events.demo", role: "viewer", department: "Marketing", temp_password: "Demo@Grand2024!" },
    ],
    budget_categories: [
      { id: "cat-venue", label: "Venue & Facilities", description: "Venue hire, setup, teardown, power, AV" },
      { id: "cat-catering", label: "Catering & Hospitality", description: "Food, beverages, service staff, tableware" },
      { id: "cat-entertainment", label: "Entertainment", description: "Performers, DJs, MCs, talent fees" },
      { id: "cat-decor", label: "Decor & Styling", description: "Floral, lighting design, staging, signage" },
      { id: "cat-marketing", label: "Marketing & Promotion", description: "Advertising, social media, printing, PR" },
    ],
    budget_templates: [
      { name: "Corporate Gala", description: "Large-scale corporate event (500+ guests)", line_items: Array(22).fill({ category_id: "cat-venue", description: "Template item", default_amount: "8000.00" }) },
      { name: "Wedding", description: "Full-service wedding event", line_items: Array(18).fill({ category_id: "cat-catering", description: "Template item", default_amount: "5000.00" }) },
      { name: "Conference", description: "Multi-day conference with speakers and exhibitions", line_items: Array(20).fill({ category_id: "cat-venue", description: "Template item", default_amount: "6000.00" }) },
    ],
    expense_categories: [
      { id: "ec-venue", label: "Venue Hire", tax_treatment: "deductible", requires_po: true },
      { id: "ec-catering", label: "Catering", tax_treatment: "deductible", requires_po: true },
      { id: "ec-entertainment", label: "Entertainment & Talent", tax_treatment: "deductible", requires_po: true },
      { id: "ec-decor", label: "Decor & Flowers", tax_treatment: "deductible", requires_po: false },
      { id: "ec-av", label: "AV & Technical", tax_treatment: "deductible", requires_po: true },
      { id: "ec-transport", label: "Guest Transportation", tax_treatment: "deductible", requires_po: false },
      { id: "ec-staff", label: "Event Staff", tax_treatment: "deductible", requires_po: false },
    ],
    approval_workflow: {
      limits: [
        { role: "Event Coordinator", max_amount: "5000.00" },
        { role: "Event Director", max_amount: "30000.00" },
        { role: "Finance Director", max_amount: "100000.00" },
        { role: "Managing Director", max_amount: "500000.00" },
      ],
      dual_approval_above: "50000.00",
    },
    inventory_categories: [
      { id: "inv-av", label: "AV Equipment", is_trackable: true },
      { id: "inv-furniture", label: "Event Furniture", is_trackable: true },
      { id: "inv-lighting", label: "Lighting Rigs", is_trackable: true },
      { id: "inv-decor", label: "Decor Items", is_trackable: true },
      { id: "inv-tableware", label: "Tableware & Linens", is_trackable: false },
      { id: "inv-consumables", label: "Consumables", is_trackable: false },
    ],
    projects: [
      { title: "TechCorp Annual Gala 2024", description: "800-person corporate gala at Grand Pavilion", phase: "Execution", budget_total: "320000.00", expense_count: 87, status: "active" },
      { title: "Chen-Williams Wedding", description: "Luxury outdoor wedding, 250 guests", phase: "Planning", budget_total: "145000.00", expense_count: 42, status: "planning" },
      { title: "InnovateConf 2024", description: "3-day tech conference, 2000 attendees", phase: "Pre-Event", budget_total: "580000.00", expense_count: 112, status: "active" },
    ],
  };
}

// ---------------------------------------------------------------------------
// Provisioning simulation steps
// ---------------------------------------------------------------------------

const PROVISION_STEPS = [
  "Creating tenant schema...",
  "Creating admin & demo users...",
  "Binding vertical configuration...",
  "Seeding chart of accounts...",
  "Creating sample projects...",
  "Seeding budget data...",
  "Generating sample expenses...",
  "Finalizing setup...",
];

// ---------------------------------------------------------------------------
// Page component — 3-step demo provisioning flow.
// ---------------------------------------------------------------------------

type FlowStep = "select" | "preview" | "provisioning" | "complete";

export default function DemoPage() {
  const [step, setStep] = useState<FlowStep>("select");
  const [selectedVertical, setSelectedVertical] = useState<string | null>(null);
  const [preview, setPreview] = useState<DemoPreview | null>(null);
  const [provisionResult, setProvisionResult] = useState<ProvisionResult | null>(null);
  const [showPassword, setShowPassword] = useState(false);

  // Provisioning progress
  const [provisionStepIndex, setProvisionStepIndex] = useState(0);
  const [provisionSteps, setProvisionSteps] = useState<
    { label: string; status: "pending" | "active" | "completed" | "failed" }[]
  >(PROVISION_STEPS.map((label) => ({ label, status: "pending" })));

  // -- Step 1 -> Step 2: generate preview --
  const handleSelectVertical = useCallback((verticalId: string) => {
    setSelectedVertical(verticalId);
  }, []);

  const handleContinueToPreview = useCallback(() => {
    if (!selectedVertical) return;
    const mockPreview = buildMockPreview(selectedVertical);
    setPreview(mockPreview);
    setStep("preview");
  }, [selectedVertical]);

  // -- Preview: update company name --
  const handleCompanyNameChange = useCallback(
    (name: string) => {
      if (!preview) return;
      const slug = name.toLowerCase().replace(/\s+/g, "-");
      setPreview({
        ...preview,
        tenant: { ...preview.tenant, name, slug },
      });
    },
    [preview],
  );

  // -- Step 2 -> Step 3: provision --
  const handleProvision = useCallback(() => {
    if (!preview) return;
    setStep("provisioning");
    setProvisionStepIndex(0);

    const steps = PROVISION_STEPS.map((label) => ({
      label,
      status: "pending" as "pending" | "active" | "completed" | "failed",
    }));

    // Simulate step-by-step provisioning
    let currentIdx = 0;
    steps[0].status = "active";
    setProvisionSteps([...steps]);

    const interval = setInterval(() => {
      steps[currentIdx].status = "completed";
      currentIdx++;

      if (currentIdx >= steps.length) {
        setProvisionSteps([...steps]);
        setProvisionStepIndex(currentIdx);
        clearInterval(interval);

        // Build simulated result
        const adminEmail = preview.users[0]?.email ?? "admin@demo.example";
        setProvisionResult({
          tenant_id: `tn_${crypto.randomUUID().slice(0, 8)}`,
          admin_email: adminEmail,
          admin_password: preview.users[0]?.temp_password ?? "Demo@2024!",
          user_count: preview.users.length,
          project_count: preview.projects.length,
          created_at: new Date().toISOString(),
        });
        setStep("complete");
        return;
      }

      steps[currentIdx].status = "active";
      setProvisionSteps([...steps]);
      setProvisionStepIndex(currentIdx);
    }, 600);

    return () => clearInterval(interval);
  }, [preview]);

  // -- Reset --
  const handleReset = useCallback(() => {
    setStep("select");
    setSelectedVertical(null);
    setPreview(null);
    setProvisionResult(null);
    setShowPassword(false);
    setProvisionStepIndex(0);
    setProvisionSteps(
      PROVISION_STEPS.map((label) => ({ label, status: "pending" as const })),
    );
  }, []);

  // -- Copy helper --
  const copyToClipboard = useCallback((text: string, label: string) => {
    navigator.clipboard.writeText(text).then(() => {
      toast.success(`${label} copied to clipboard`);
    });
  }, []);

  const verticalIds = Object.keys(verticals);

  return (
    <div className="mx-auto max-w-5xl px-6 py-8">
      {/* Header */}
      <div className="mb-8">
        <h1 className="text-2xl font-bold font-heading" style={{ color: "#0f172a" }}>
          Demo Provisioning
        </h1>
        <p className="mt-1 text-sm font-body" style={{ color: "#64748b" }}>
          Create a fully populated demo tenant with sample data for any industry vertical.
        </p>
      </div>

      {/* Step indicator */}
      <div className="mb-8 flex items-center gap-2 text-xs font-heading" style={{ color: "#94a3b8" }}>
        <span className={step === "select" ? "font-semibold text-blue-600" : "text-green-600"}>
          1. Select Vertical
        </span>
        <span aria-hidden="true">{">"}</span>
        <span className={step === "preview" ? "font-semibold text-blue-600" : step === "provisioning" || step === "complete" ? "text-green-600" : ""}>
          2. Preview
        </span>
        <span aria-hidden="true">{">"}</span>
        <span className={step === "provisioning" ? "font-semibold text-blue-600" : step === "complete" ? "text-green-600" : ""}>
          3. Provision
        </span>
      </div>

      {/* ── Step 1: Select Vertical ────────────────────────────────────── */}
      {step === "select" && (
        <div>
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
            {verticalIds.map((vId) => {
              const info = VERTICAL_CARD_INFO[vId];
              const vDef = verticals[vId];
              if (!info || !vDef) return null;

              return (
                <VerticalCard
                  key={vId}
                  verticalId={vId}
                  name={vDef.theme.name}
                  description={info.description}
                  companyName={info.companyName}
                  userCount={info.userCount}
                  projectCount={info.projectCount}
                  color={vDef.theme.colors.primary}
                  icon={info.icon}
                  isSelected={selectedVertical === vId}
                  onSelect={() => handleSelectVertical(vId)}
                />
              );
            })}
          </div>

          <div className="mt-6 flex justify-end">
            <button
              type="button"
              disabled={!selectedVertical}
              onClick={handleContinueToPreview}
              className="inline-flex items-center gap-2 rounded-lg px-6 py-2.5 text-sm font-semibold font-heading text-white transition-opacity disabled:opacity-40"
              style={{
                backgroundColor: selectedVertical
                  ? verticals[selectedVertical]?.theme.colors.primary
                  : "#94a3b8",
              }}
            >
              Continue to Preview
            </button>
          </div>
        </div>
      )}

      {/* ── Step 2: Preview ────────────────────────────────────────────── */}
      {step === "preview" && preview && (
        <div>
          <button
            type="button"
            onClick={() => setStep("select")}
            className="mb-6 inline-flex items-center gap-1 text-sm font-heading transition-colors hover:text-blue-600"
            style={{ color: "#64748b" }}
          >
            <ArrowLeft className="h-4 w-4" />
            Back to selection
          </button>

          <DemoPreviewAccordion
            preview={preview}
            onCompanyNameChange={handleCompanyNameChange}
          />

          <div className="mt-6 flex items-center justify-between">
            <button
              type="button"
              onClick={() => setStep("select")}
              className="inline-flex items-center gap-1 rounded-lg border px-4 py-2 text-sm font-heading transition-colors hover:bg-gray-50"
              style={{ borderColor: "var(--thittam-border, #e2e8f0)", color: "#475569" }}
            >
              <ArrowLeft className="h-4 w-4" />
              Back
            </button>
            <button
              type="button"
              onClick={handleProvision}
              className="inline-flex items-center gap-2 rounded-lg px-6 py-2.5 text-sm font-semibold font-heading text-white transition-opacity"
              style={{
                backgroundColor: selectedVertical
                  ? verticals[selectedVertical]?.theme.colors.primary
                  : "#2563eb",
              }}
            >
              Confirm & Provision
            </button>
          </div>
        </div>
      )}

      {/* ── Step 3: Provisioning ───────────────────────────────────────── */}
      {step === "provisioning" && (
        <div className="mx-auto max-w-md py-8">
          <h2 className="mb-6 text-lg font-semibold font-heading" style={{ color: "#0f172a" }}>
            Provisioning Demo Tenant...
          </h2>
          <ProvisionProgress currentStep={provisionStepIndex} steps={provisionSteps} />
        </div>
      )}

      {/* ── Step 4: Complete ───────────────────────────────────────────── */}
      {step === "complete" && provisionResult && (
        <div className="mx-auto max-w-lg py-4">
          <div
            className="rounded-xl border-2 p-6"
            style={{
              borderColor: selectedVertical
                ? verticals[selectedVertical]?.theme.colors.primary
                : "#16a34a",
            }}
          >
            <div className="mb-4 flex items-center gap-3">
              <div className="flex h-10 w-10 items-center justify-center rounded-full bg-green-100">
                <svg className="h-6 w-6 text-green-600" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                  <path strokeLinecap="round" strokeLinejoin="round" d="M5 13l4 4L19 7" />
                </svg>
              </div>
              <div>
                <h2 className="text-lg font-bold font-heading" style={{ color: "#0f172a" }}>
                  Demo Tenant Ready
                </h2>
                <p className="text-xs font-body" style={{ color: "#64748b" }}>
                  {provisionResult.user_count} users and {provisionResult.project_count} sample projects created
                </p>
              </div>
            </div>

            <div className="space-y-4">
              {/* Tenant ID */}
              <div>
                <label className="block text-xs font-semibold font-heading" style={{ color: "#64748b" }}>
                  Tenant ID
                </label>
                <p className="mt-1 text-sm font-mono" style={{ color: "#0f172a" }}>
                  {provisionResult.tenant_id}
                </p>
              </div>

              {/* Admin Email */}
              <div>
                <label className="block text-xs font-semibold font-heading" style={{ color: "#64748b" }}>
                  Admin Email
                </label>
                <div className="mt-1 flex items-center gap-2">
                  <p className="text-sm font-mono" style={{ color: "#0f172a" }}>
                    {provisionResult.admin_email}
                  </p>
                  <button
                    type="button"
                    onClick={() => copyToClipboard(provisionResult.admin_email, "Email")}
                    className="rounded p-1 transition-colors hover:bg-gray-100"
                    aria-label="Copy email"
                  >
                    <Copy className="h-3.5 w-3.5" style={{ color: "#64748b" }} />
                  </button>
                </div>
              </div>

              {/* Temp Password */}
              <div>
                <label className="block text-xs font-semibold font-heading" style={{ color: "#64748b" }}>
                  Temporary Password
                </label>
                <div className="mt-1 flex items-center gap-2">
                  <p className="text-sm font-mono" style={{ color: "#0f172a" }}>
                    {showPassword ? provisionResult.admin_password : "••••••••••••"}
                  </p>
                  <button
                    type="button"
                    onClick={() => setShowPassword((s) => !s)}
                    className="rounded p-1 transition-colors hover:bg-gray-100"
                    aria-label={showPassword ? "Hide password" : "Show password"}
                  >
                    {showPassword ? (
                      <EyeOff className="h-3.5 w-3.5" style={{ color: "#64748b" }} />
                    ) : (
                      <Eye className="h-3.5 w-3.5" style={{ color: "#64748b" }} />
                    )}
                  </button>
                  <button
                    type="button"
                    onClick={() => copyToClipboard(provisionResult.admin_password, "Password")}
                    className="rounded p-1 transition-colors hover:bg-gray-100"
                    aria-label="Copy password"
                  >
                    <Copy className="h-3.5 w-3.5" style={{ color: "#64748b" }} />
                  </button>
                </div>
              </div>

              {/* Created at */}
              <div>
                <label className="block text-xs font-semibold font-heading" style={{ color: "#64748b" }}>
                  Created At
                </label>
                <p className="mt-1 text-sm font-mono tabular-nums" style={{ color: "#0f172a" }}>
                  {new Date(provisionResult.created_at).toLocaleString()}
                </p>
              </div>
            </div>

            {/* Actions */}
            <div className="mt-6 flex flex-col gap-3 sm:flex-row">
              <a
                href={`/auth/login?email=${encodeURIComponent(provisionResult.admin_email)}`}
                className="inline-flex flex-1 items-center justify-center gap-2 rounded-lg px-4 py-2.5 text-sm font-semibold font-heading text-white transition-opacity"
                style={{
                  backgroundColor: selectedVertical
                    ? verticals[selectedVertical]?.theme.colors.primary
                    : "#2563eb",
                }}
              >
                <ExternalLink className="h-4 w-4" />
                Open Demo
              </a>
              <button
                type="button"
                onClick={handleReset}
                className="inline-flex flex-1 items-center justify-center gap-2 rounded-lg border px-4 py-2.5 text-sm font-heading transition-colors hover:bg-gray-50"
                style={{ borderColor: "var(--thittam-border, #e2e8f0)", color: "#475569" }}
              >
                <Plus className="h-4 w-4" />
                Provision Another
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
