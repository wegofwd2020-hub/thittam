import { api } from "./client";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export interface Production {
  id: string;
  tenant_id: string;
  title: string;
  slug: string;
  description: string;
  genre: string;
  language: string;
  status: string;
  start_date: string | null;
  end_date: string | null;
  created_by: string;
  created_at: string;
  updated_at: string;
}

export interface Phase {
  id: string;
  production_id: string;
  phase_type: string;
  status: string;
  start_date: string | null;
  end_date: string | null;
}

export interface PhaseType {
  id: string;
  label: string;
  order: number;
  is_billable: boolean;
  allowed_transitions: string[];
}

export interface CrewMember {
  id: string;
  production_id: string;
  user_id: string | null;
  name: string;
  role: string;
  department: string;
  day_rate: string;
  currency: string;
  start_date: string | null;
  end_date: string | null;
}

export interface EntityLabels {
  [key: string]: string;
}

// ---------------------------------------------------------------------------
// Request payloads
// ---------------------------------------------------------------------------

export interface CreateProductionInput {
  title: string;
  description?: string;
  genre?: string;
  start_date?: string;
}

export interface AddCrewMemberInput {
  name: string;
  role: string;
  department?: string;
  day_rate?: string;
}

// ---------------------------------------------------------------------------
// Query-string helper
// ---------------------------------------------------------------------------

function qs(params?: Record<string, string | number | undefined>): string {
  if (!params) return "";
  const entries = Object.entries(params).filter(
    ([, v]) => v !== undefined && v !== "",
  );
  if (entries.length === 0) return "";
  const search = new URLSearchParams(
    entries.map(([k, v]) => [k, String(v)]),
  );
  return `?${search.toString()}`;
}

// ---------------------------------------------------------------------------
// Productions
// ---------------------------------------------------------------------------

const BASE = "/api/v1/productions";

export interface ListProductionsResponse {
  productions: Production[];
}

export interface ListPhasesResponse {
  phases: Phase[];
}

export interface GetPhaseTypesResponse {
  phase_types: PhaseType[];
}

export interface ListCrewMembersResponse {
  members: CrewMember[];
}

export async function listProductions(
  params?: { status?: string; limit?: number; after?: string },
): Promise<ListProductionsResponse> {
  return api.getList<ListProductionsResponse>(`${BASE}${qs(params)}`);
}

export async function getProduction(id: string): Promise<Production> {
  return api.get<Production>(`${BASE}/${id}`);
}

export async function createProduction(
  data: CreateProductionInput,
): Promise<Production> {
  return api.post<Production>(BASE, data);
}

export async function updateProduction(
  id: string,
  data: Partial<Production>,
): Promise<Production> {
  return api.patch<Production>(`${BASE}/${id}`, data);
}

export async function archiveProduction(id: string): Promise<void> {
  await api.post<void>(`${BASE}/${id}/archive`, {});
}

// ---------------------------------------------------------------------------
// Phases
// ---------------------------------------------------------------------------

export async function listPhases(productionId: string): Promise<Phase[]> {
  const res = await api.getList<ListPhasesResponse>(
    `${BASE}/${productionId}/phases`,
  );
  return res.phases ?? [];
}

export async function createPhase(
  productionId: string,
  data: { phase_type: string },
): Promise<Phase> {
  return api.post<Phase>(`${BASE}/${productionId}/phases`, data);
}

export async function updatePhaseStatus(
  phaseId: string,
  newPhaseType: string,
): Promise<Phase> {
  // Body field matches the proto (UpdatePhaseStatusRequest.new_phase_type),
  // which the grpc-gateway decoder maps using UseProtoNames. Sending
  // `status` here would be silently dropped.
  return api.patch<Phase>(`/api/v1/phases/${phaseId}`, {
    new_phase_type: newPhaseType,
  });
}

// ---------------------------------------------------------------------------
// Phase types & entity labels (vertical config)
// ---------------------------------------------------------------------------

export async function getPhaseTypes(): Promise<PhaseType[]> {
  const res = await api.getList<GetPhaseTypesResponse>(
    "/api/v1/config/phase-types",
  );
  return res.phase_types ?? [];
}

export async function getEntityLabels(): Promise<EntityLabels> {
  return api.get<EntityLabels>("/api/v1/config/entity-labels");
}

// ---------------------------------------------------------------------------
// Crew members
// ---------------------------------------------------------------------------

export async function listCrewMembers(
  productionId: string,
): Promise<CrewMember[]> {
  const res = await api.getList<ListCrewMembersResponse>(
    `${BASE}/${productionId}/crew`,
  );
  return res.members ?? [];
}

export async function addCrewMember(
  productionId: string,
  data: AddCrewMemberInput,
): Promise<CrewMember> {
  return api.post<CrewMember>(`${BASE}/${productionId}/crew`, data);
}

export async function removeCrewMember(id: string): Promise<void> {
  await api.delete(`/api/v1/crew/${id}`);
}
