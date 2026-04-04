import { api } from "./client";
import type { ApiResponse } from "./types";

// ---------------------------------------------------------------------------
// Dashboard API — functions for the reporting-analytics service endpoints.
// Types match services/reporting/dashboard_models.go.
// ---------------------------------------------------------------------------

// ── Response types ──────────────────────────────────────────────────────────

export interface BudgetHealthCount {
  on_track: number;
  at_risk: number;
  over_budget: number;
}

export interface ProjectSummary {
  id: string;
  title: string;
  status: string;
  current_phase: string;
  budget_total: string;
  actual_spend: string;
  budget_used_pct: string;
  health: string;
  start_date?: string;
}

export interface PortfolioOverview {
  tenant_id: string;
  project_label: string;
  total_active: number;
  total_budget: string;
  total_actual: string;
  health_counts: BudgetHealthCount;
  projects: ProjectSummary[];
}

export interface CategorySpend {
  category_id: string;
  category_name: string;
  budgeted: string;
  actual: string;
  percentage: string;
}

export interface MonthlySpend {
  month: string;
  actual: string;
  budget: string;
}

export interface FinancialSummary {
  tenant_id: string;
  total_budgeted: string;
  total_actual: string;
  total_committed: string;
  total_available: string;
  burn_rate_per_month: string;
  by_category: CategorySpend[];
  monthly_trend: MonthlySpend[];
}

export interface ApprovalAgeBands {
  under_24h: number;
  "1_to_3_days": number;
  "3_to_7_days": number;
  over_7_days: number;
}

export interface PendingApproval {
  id: string;
  type: string;
  description: string;
  amount: string;
  submitted_by: string;
  submitted_at: string;
  project_title: string;
  age_hours: number;
}

export interface ApprovalPipeline {
  tenant_id: string;
  total_pending: number;
  total_amount: string;
  by_age: ApprovalAgeBands;
  pending_items: PendingApproval[];
}

export interface ProjectTeam {
  project_id: string;
  project_title: string;
  member_count: number;
}

export interface TeamUtilization {
  tenant_id: string;
  team_member_label: string;
  total_members: number;
  assigned: number;
  available: number;
  utilization_pct: string;
  by_project: ProjectTeam[];
}

export interface ComplianceItem {
  id: string;
  type: string;
  description: string;
  severity: string;
  detected_at: string;
  project_title: string;
}

export interface ComplianceStatus {
  tenant_id: string;
  overdue_approvals: number;
  policy_violations: number;
  audit_flags: number;
  recent_violations: ComplianceItem[];
}

// ── Composite summary (all sections in a single call) ───────────────────────

export interface DashboardSummary {
  portfolio: PortfolioOverview;
  financial: FinancialSummary;
  approvals: ApprovalPipeline;
  team: TeamUtilization;
  compliance: ComplianceStatus;
}

// ── API functions ───────────────────────────────────────────────────────────

const BASE = "/v1/reports/dashboard";

export async function getPortfolioOverview(): Promise<PortfolioOverview> {
  const res = await api.get<PortfolioOverview>(`${BASE}/portfolio`);
  return (res as ApiResponse<PortfolioOverview>).data;
}

export async function getFinancialSummary(): Promise<FinancialSummary> {
  const res = await api.get<FinancialSummary>(`${BASE}/financial`);
  return (res as ApiResponse<FinancialSummary>).data;
}

export async function getApprovalPipeline(): Promise<ApprovalPipeline> {
  const res = await api.get<ApprovalPipeline>(`${BASE}/approvals`);
  return (res as ApiResponse<ApprovalPipeline>).data;
}

export async function getTeamUtilization(): Promise<TeamUtilization> {
  const res = await api.get<TeamUtilization>(`${BASE}/team`);
  return (res as ApiResponse<TeamUtilization>).data;
}

export async function getComplianceStatus(): Promise<ComplianceStatus> {
  const res = await api.get<ComplianceStatus>(`${BASE}/compliance`);
  return (res as ApiResponse<ComplianceStatus>).data;
}

export async function getDashboardSummary(): Promise<DashboardSummary> {
  const res = await api.get<DashboardSummary>(`${BASE}/summary`);
  return (res as ApiResponse<DashboardSummary>).data;
}
