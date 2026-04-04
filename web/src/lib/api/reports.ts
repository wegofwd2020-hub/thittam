// ---------------------------------------------------------------------------
// Reports API — mock implementation until backend is available.
// ---------------------------------------------------------------------------

export interface ReportDefinition {
  id: string;
  name: string;
  description: string;
  data_sources: string[];
  default_columns: string[];
  verticals: string[]; // which verticals this report applies to ("*" = all)
}

export interface ReportData {
  columns: string[];
  rows: Record<string, string>[];
}

// ---------------------------------------------------------------------------
// Mock data — vertical-specific report definitions
// ---------------------------------------------------------------------------

const MOCK_DEFINITIONS: ReportDefinition[] = [
  // ── Movie Production ────────────────────────────────────────────
  {
    id: "cost_report",
    name: "Daily Cost Report",
    description:
      "Summarises daily expenditure across all active productions, broken down by budget category.",
    data_sources: ["expense-tracking", "budget-planning"],
    default_columns: ["Category", "Budgeted", "Actual", "Variance", "% Used"],
    verticals: ["movie-production"],
  },
  {
    id: "budget_variance",
    name: "Budget Variance Report",
    description:
      "Compares approved budgets against actuals with drill-down by line item and phase.",
    data_sources: ["budget-planning", "general-ledger"],
    default_columns: ["Line Item", "Approved Budget", "Actual Spend", "Variance", "Status"],
    verticals: ["*"],
  },

  // ── Software Development ────────────────────────────────────────
  {
    id: "burn_rate",
    name: "Burn Rate",
    description:
      "Monthly burn rate across all active projects, comparing planned vs actual spend over time.",
    data_sources: ["budget-planning", "expense-tracking"],
    default_columns: ["Month", "Planned", "Actual", "Variance", "Cumulative"],
    verticals: ["software-development"],
  },
  {
    id: "project_pnl",
    name: "Project P&L",
    description:
      "Profit and loss statement per project including revenue, cost of delivery, and margin.",
    data_sources: ["general-ledger", "budget-planning"],
    default_columns: ["Project", "Revenue", "Cost", "Gross Margin", "Margin %"],
    verticals: ["software-development"],
  },
  {
    id: "resource_utilisation",
    name: "Resource Utilisation",
    description:
      "Team member allocation and billable utilisation across all active projects.",
    data_sources: ["project-management", "reporting-analytics"],
    default_columns: ["Team Member", "Allocated Hrs", "Billable Hrs", "Utilisation %", "Project"],
    verticals: ["software-development"],
  },
  {
    id: "invoice_tracker",
    name: "Invoice Tracker",
    description:
      "Tracks client invoices, payment status, and ageing for all active projects.",
    data_sources: ["general-ledger", "expense-tracking"],
    default_columns: ["Invoice #", "Client", "Amount", "Due Date", "Status"],
    verticals: ["software-development"],
  },

  // ── Construction ────────────────────────────────────────────────
  {
    id: "boq_variance",
    name: "BOQ Variance",
    description:
      "Bill of Quantities variance analysis comparing estimated vs actual quantities and rates.",
    data_sources: ["budget-planning", "inventory-management"],
    default_columns: ["Item", "Est. Qty", "Actual Qty", "Est. Rate", "Actual Rate", "Variance"],
    verticals: ["construction"],
  },
  {
    id: "progress_bill",
    name: "Progress Bill",
    description:
      "Running account bill tracking work completed vs certified amounts per work package.",
    data_sources: ["project-management", "general-ledger"],
    default_columns: ["Work Package", "Contract Value", "Completed", "Certified", "Retention"],
    verticals: ["construction"],
  },
  {
    id: "retention_schedule",
    name: "Retention Schedule",
    description:
      "Tracks retention amounts held and release dates across all active contracts.",
    data_sources: ["general-ledger", "project-management"],
    default_columns: ["Contract", "Retention Held", "Release Date", "Status", "Remarks"],
    verticals: ["construction"],
  },

  // ── Events Management ──────────────────────────────────────────
  {
    id: "event_pnl",
    name: "Event P&L",
    description:
      "Per-event revenue, expenses, and margin analysis with category breakdown.",
    data_sources: ["general-ledger", "budget-planning"],
    default_columns: ["Event", "Revenue", "Expenses", "Net Margin", "Margin %"],
    verticals: ["events-management"],
  },
  {
    id: "vendor_tracker",
    name: "Vendor Tracker",
    description:
      "Tracks vendor contracts, payment milestones, and delivery status for events.",
    data_sources: ["expense-tracking", "project-management"],
    default_columns: ["Vendor", "Service", "Contract Value", "Paid", "Balance"],
    verticals: ["events-management"],
  },
  {
    id: "cost_per_head",
    name: "Cost Per Head",
    description:
      "Per-attendee cost analysis across catering, venue, entertainment, and logistics.",
    data_sources: ["expense-tracking", "reporting-analytics"],
    default_columns: ["Category", "Total Cost", "Attendees", "Cost/Head", "% of Total"],
    verticals: ["events-management"],
  },
];

// ---------------------------------------------------------------------------
// Mock tabular data per report
// ---------------------------------------------------------------------------

const MOCK_ROWS: Record<string, Record<string, string>[]> = {
  cost_report: [
    { Category: "Above the Line", Budgeted: "5,00,00,000", Actual: "4,85,00,000", Variance: "15,00,000", "% Used": "97.0%" },
    { Category: "Below the Line", Budgeted: "8,00,00,000", Actual: "6,20,00,000", Variance: "1,80,00,000", "% Used": "77.5%" },
    { Category: "Post Production", Budgeted: "4,50,00,000", Actual: "3,45,00,000", Variance: "1,05,00,000", "% Used": "76.7%" },
    { Category: "Marketing & Distribution", Budgeted: "3,00,00,000", Actual: "2,50,00,000", Variance: "50,00,000", "% Used": "83.3%" },
    { Category: "Contingency", Budgeted: "2,00,00,000", Actual: "1,50,00,000", Variance: "50,00,000", "% Used": "75.0%" },
  ],
  budget_variance: [
    { "Line Item": "Director Fee", "Approved Budget": "75,00,000", "Actual Spend": "75,00,000", Variance: "0", Status: "On Track" },
    { "Line Item": "Lead Cast", "Approved Budget": "2,00,00,000", "Actual Spend": "2,10,00,000", Variance: "-10,00,000", Status: "Over Budget" },
    { "Line Item": "Camera & Lighting", "Approved Budget": "1,20,00,000", "Actual Spend": "95,00,000", Variance: "25,00,000", Status: "Under Budget" },
    { "Line Item": "Set Design", "Approved Budget": "80,00,000", "Actual Spend": "78,00,000", Variance: "2,00,000", Status: "On Track" },
    { "Line Item": "VFX", "Approved Budget": "3,00,00,000", "Actual Spend": "2,40,00,000", Variance: "60,00,000", Status: "Under Budget" },
  ],
  burn_rate: [
    { Month: "Nov 2025", Planned: "40,00,000", Actual: "38,00,000", Variance: "2,00,000", Cumulative: "38,00,000" },
    { Month: "Dec 2025", Planned: "40,00,000", Actual: "45,00,000", Variance: "-5,00,000", Cumulative: "83,00,000" },
    { Month: "Jan 2026", Planned: "45,00,000", Actual: "52,00,000", Variance: "-7,00,000", Cumulative: "1,35,00,000" },
    { Month: "Feb 2026", Planned: "50,00,000", Actual: "58,00,000", Variance: "-8,00,000", Cumulative: "1,93,00,000" },
    { Month: "Mar 2026", Planned: "55,00,000", Actual: "61,00,000", Variance: "-6,00,000", Cumulative: "2,54,00,000" },
    { Month: "Apr 2026", Planned: "55,00,000", Actual: "40,00,000", Variance: "15,00,000", Cumulative: "2,94,00,000" },
  ],
  project_pnl: [
    { Project: "Acme Dashboard", Revenue: "1,20,00,000", Cost: "85,00,000", "Gross Margin": "35,00,000", "Margin %": "29.2%" },
    { Project: "Nova API", Revenue: "95,00,000", Cost: "72,00,000", "Gross Margin": "23,00,000", "Margin %": "24.2%" },
    { Project: "Zenith Mobile", Revenue: "60,00,000", Cost: "55,00,000", "Gross Margin": "5,00,000", "Margin %": "8.3%" },
    { Project: "Apex Analytics", Revenue: "1,50,00,000", Cost: "1,10,00,000", "Gross Margin": "40,00,000", "Margin %": "26.7%" },
    { Project: "CloudOps Infra", Revenue: "80,00,000", Cost: "68,00,000", "Gross Margin": "12,00,000", "Margin %": "15.0%" },
  ],
  resource_utilisation: [
    { "Team Member": "Ravi S.", "Allocated Hrs": "160", "Billable Hrs": "144", "Utilisation %": "90.0%", Project: "Acme Dashboard" },
    { "Team Member": "Anita K.", "Allocated Hrs": "160", "Billable Hrs": "128", "Utilisation %": "80.0%", Project: "Nova API" },
    { "Team Member": "Karthik R.", "Allocated Hrs": "160", "Billable Hrs": "100", "Utilisation %": "62.5%", Project: "Zenith Mobile" },
    { "Team Member": "Lakshmi P.", "Allocated Hrs": "160", "Billable Hrs": "152", "Utilisation %": "95.0%", Project: "Apex Analytics" },
    { "Team Member": "Dev M.", "Allocated Hrs": "160", "Billable Hrs": "136", "Utilisation %": "85.0%", Project: "CloudOps Infra" },
  ],
  invoice_tracker: [
    { "Invoice #": "INV-2026-041", Client: "Acme Corp", Amount: "25,00,000", "Due Date": "2026-04-15", Status: "Pending" },
    { "Invoice #": "INV-2026-038", Client: "Nova Inc", Amount: "18,50,000", "Due Date": "2026-04-10", Status: "Overdue" },
    { "Invoice #": "INV-2026-035", Client: "Zenith Ltd", Amount: "12,00,000", "Due Date": "2026-03-31", Status: "Paid" },
    { "Invoice #": "INV-2026-042", Client: "Apex Group", Amount: "30,00,000", "Due Date": "2026-04-20", Status: "Pending" },
    { "Invoice #": "INV-2026-037", Client: "CloudOps", Amount: "15,00,000", "Due Date": "2026-04-05", Status: "Paid" },
  ],
  boq_variance: [
    { Item: "Reinforcement Steel", "Est. Qty": "500 MT", "Actual Qty": "520 MT", "Est. Rate": "65,000/MT", "Actual Rate": "68,000/MT", Variance: "-19,60,000" },
    { Item: "M30 Concrete", "Est. Qty": "2000 m\u00B3", "Actual Qty": "1950 m\u00B3", "Est. Rate": "6,500/m\u00B3", "Actual Rate": "6,800/m\u00B3", Variance: "-3,60,000" },
    { Item: "Formwork", "Est. Qty": "8000 m\u00B2", "Actual Qty": "7800 m\u00B2", "Est. Rate": "450/m\u00B2", "Actual Rate": "420/m\u00B2", Variance: "3,24,000" },
    { Item: "Bricks (Class A)", "Est. Qty": "1,00,000", "Actual Qty": "95,000", "Est. Rate": "12/unit", "Actual Rate": "13/unit", Variance: "-3,50,000" },
    { Item: "Electrical Wiring", "Est. Qty": "15,000 m", "Actual Qty": "14,200 m", "Est. Rate": "85/m", "Actual Rate": "90/m", Variance: "3,000" },
  ],
  progress_bill: [
    { "Work Package": "Foundation", "Contract Value": "2,50,00,000", Completed: "100%", Certified: "2,45,00,000", Retention: "12,50,000" },
    { "Work Package": "Superstructure", "Contract Value": "4,00,00,000", Completed: "75%", Certified: "2,80,00,000", Retention: "14,00,000" },
    { "Work Package": "MEP", "Contract Value": "1,80,00,000", Completed: "40%", Certified: "65,00,000", Retention: "3,25,000" },
    { "Work Package": "Finishing", "Contract Value": "1,20,00,000", Completed: "10%", Certified: "10,00,000", Retention: "50,000" },
    { "Work Package": "External Works", "Contract Value": "80,00,000", Completed: "0%", Certified: "0", Retention: "0" },
  ],
  retention_schedule: [
    { Contract: "ABC Builders — Tower A", "Retention Held": "45,00,000", "Release Date": "2026-09-15", Status: "Held", Remarks: "DLP ends Sep 2026" },
    { Contract: "DEF Infra — Road Works", "Retention Held": "12,00,000", "Release Date": "2026-06-30", Status: "Held", Remarks: "50% at completion, 50% after DLP" },
    { Contract: "GHI Electricals", "Retention Held": "3,50,000", "Release Date": "2026-05-01", Status: "Due", Remarks: "DLP completed, pending release" },
    { Contract: "JKL Plumbing", "Retention Held": "2,80,000", "Release Date": "2026-04-15", Status: "Overdue", Remarks: "Release approved, awaiting payment" },
    { Contract: "MNO Landscaping", "Retention Held": "5,00,000", "Release Date": "2026-12-31", Status: "Held", Remarks: "Warranty period active" },
  ],
  event_pnl: [
    { Event: "Tech Summit 2026", Revenue: "45,00,000", Expenses: "32,00,000", "Net Margin": "13,00,000", "Margin %": "28.9%" },
    { Event: "Annual Gala Dinner", Revenue: "28,00,000", Expenses: "25,50,000", "Net Margin": "2,50,000", "Margin %": "8.9%" },
    { Event: "Product Launch — Zenith", Revenue: "15,00,000", Expenses: "12,00,000", "Net Margin": "3,00,000", "Margin %": "20.0%" },
    { Event: "Corporate Retreat", Revenue: "18,00,000", Expenses: "16,80,000", "Net Margin": "1,20,000", "Margin %": "6.7%" },
    { Event: "Charity Concert", Revenue: "35,00,000", Expenses: "28,00,000", "Net Margin": "7,00,000", "Margin %": "20.0%" },
  ],
  vendor_tracker: [
    { Vendor: "SoundWave Audio", Service: "PA System", "Contract Value": "8,00,000", Paid: "5,00,000", Balance: "3,00,000" },
    { Vendor: "GreenLeaf Caterers", Service: "F&B — 500 pax", "Contract Value": "12,00,000", Paid: "12,00,000", Balance: "0" },
    { Vendor: "StageKraft", Service: "Stage & Lighting", "Contract Value": "6,50,000", Paid: "3,25,000", Balance: "3,25,000" },
    { Vendor: "PrintMax", Service: "Invitations & Banners", "Contract Value": "1,50,000", Paid: "1,50,000", Balance: "0" },
    { Vendor: "SecureGuard", Service: "Event Security", "Contract Value": "2,00,000", Paid: "1,00,000", Balance: "1,00,000" },
  ],
  cost_per_head: [
    { Category: "Catering", "Total Cost": "12,00,000", Attendees: "500", "Cost/Head": "2,400", "% of Total": "38.7%" },
    { Category: "Venue Rental", "Total Cost": "5,00,000", Attendees: "500", "Cost/Head": "1,000", "% of Total": "16.1%" },
    { Category: "Entertainment", "Total Cost": "8,00,000", Attendees: "500", "Cost/Head": "1,600", "% of Total": "25.8%" },
    { Category: "Logistics & Transport", "Total Cost": "3,00,000", Attendees: "500", "Cost/Head": "600", "% of Total": "9.7%" },
    { Category: "Decor & Branding", "Total Cost": "3,00,000", Attendees: "500", "Cost/Head": "600", "% of Total": "9.7%" },
  ],
};

// ---------------------------------------------------------------------------
// API functions (mock — returns promises to match real API shape)
// ---------------------------------------------------------------------------

export async function listReportDefinitions(
  verticalId?: string,
): Promise<ReportDefinition[]> {
  // Simulate network latency
  await new Promise((r) => setTimeout(r, 250));

  if (!verticalId) return MOCK_DEFINITIONS;

  return MOCK_DEFINITIONS.filter(
    (d) => d.verticals.includes("*") || d.verticals.includes(verticalId),
  );
}

export async function getReportDefinition(
  id: string,
): Promise<ReportDefinition> {
  await new Promise((r) => setTimeout(r, 150));
  const def = MOCK_DEFINITIONS.find((d) => d.id === id);
  if (!def) throw new Error(`Report definition "${id}" not found`);
  return def;
}

export async function runReport(
  id: string,
  _params: { production_id?: string; from?: string; to?: string },
): Promise<ReportData> {
  await new Promise((r) => setTimeout(r, 400));

  const def = MOCK_DEFINITIONS.find((d) => d.id === id);
  const rows = MOCK_ROWS[id];

  if (def && rows) {
    return { columns: def.default_columns, rows };
  }

  return { columns: [], rows: [] };
}
