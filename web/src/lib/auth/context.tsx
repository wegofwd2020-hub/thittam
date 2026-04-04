"use client";

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
} from "react";
import { api, ApiError } from "@/lib/api/client";
import {
  login as apiLogin,
  logout as apiLogout,
  refreshToken as apiRefreshToken,
} from "@/lib/api/auth";
import type { TokenPair } from "@/lib/api/types";

export interface User {
  id: string;
  email: string;
  displayName: string;
  roles: string[];
  permissions: string[];
}

export interface Tenant {
  id: string;
  name: string;
  slug: string;
  plan: string;
  verticalId: string;
}

export interface AuthState {
  user: User | null;
  tenant: Tenant | null;
  isAuthenticated: boolean;
  isLoading: boolean;
  login: (email: string, password: string) => Promise<void>;
  logout: () => Promise<void>;
  verticalId: string | null;
}

const AuthContext = createContext<AuthState | undefined>(undefined);

const ACCESS_TOKEN_KEY = "thittam_access_token";
const REFRESH_TOKEN_KEY = "thittam_refresh_token";
const TENANT_ID_KEY = "thittam_tenant_id";

function storeTokens(pair: TokenPair) {
  localStorage.setItem(ACCESS_TOKEN_KEY, pair.access_token);
  localStorage.setItem(REFRESH_TOKEN_KEY, pair.refresh_token);
}

function clearTokens() {
  localStorage.removeItem(ACCESS_TOKEN_KEY);
  localStorage.removeItem(REFRESH_TOKEN_KEY);
  localStorage.removeItem(TENANT_ID_KEY);
}

function getStoredAccessToken(): string | null {
  return typeof window !== "undefined"
    ? localStorage.getItem(ACCESS_TOKEN_KEY)
    : null;
}

function getStoredRefreshToken(): string | null {
  return typeof window !== "undefined"
    ? localStorage.getItem(REFRESH_TOKEN_KEY)
    : null;
}

export function AuthProvider({ children }: { children: React.ReactNode }) {
  const [user, setUser] = useState<User | null>(null);
  const [tenant, setTenant] = useState<Tenant | null>(null);
  const [isLoading, setIsLoading] = useState(true);

  const clearAuth = useCallback(() => {
    clearTokens();
    api.setToken(null);
    api.setTenantId(null);
    setUser(null);
    setTenant(null);
  }, []);

  const hydrateSession = useCallback(
    async (accessToken: string) => {
      api.setToken(accessToken);
      try {
        const { data } = await api.get<{ user: User; tenant: Tenant }>(
          "/api/v1/auth/me"
        );
        setUser(data.user);
        setTenant(data.tenant);
        api.setTenantId(data.tenant.id);
        localStorage.setItem(TENANT_ID_KEY, data.tenant.id);
      } catch {
        // Token might be expired -- try refresh
        const rt = getStoredRefreshToken();
        if (rt) {
          try {
            const pair = await apiRefreshToken(rt);
            storeTokens(pair);
            api.setToken(pair.access_token);
            const { data } = await api.get<{ user: User; tenant: Tenant }>(
              "/api/v1/auth/me"
            );
            setUser(data.user);
            setTenant(data.tenant);
            api.setTenantId(data.tenant.id);
            localStorage.setItem(TENANT_ID_KEY, data.tenant.id);
          } catch {
            clearAuth();
          }
        } else {
          clearAuth();
        }
      }
    },
    [clearAuth]
  );

  // On mount, check for existing token and load user info
  useEffect(() => {
    const token = getStoredAccessToken();
    if (!token) {
      setIsLoading(false);
      return;
    }

    hydrateSession(token).finally(() => setIsLoading(false));
  }, [hydrateSession]);

  const login = useCallback(
    async (email: string, password: string) => {
      const pair: TokenPair = await apiLogin(email, password);
      storeTokens(pair);
      api.setToken(pair.access_token);

      const { data } = await api.get<{ user: User; tenant: Tenant }>(
        "/api/v1/auth/me"
      );
      setUser(data.user);
      setTenant(data.tenant);
      api.setTenantId(data.tenant.id);
      localStorage.setItem(TENANT_ID_KEY, data.tenant.id);
    },
    []
  );

  const logout = useCallback(async () => {
    const rt = getStoredRefreshToken();
    try {
      if (rt) await apiLogout(rt);
    } catch {
      // Best-effort server-side logout
    } finally {
      clearAuth();
    }
  }, [clearAuth]);

  const value = useMemo<AuthState>(
    () => ({
      user,
      tenant,
      isAuthenticated: !!user,
      isLoading,
      login,
      logout,
      verticalId: tenant?.verticalId ?? null,
    }),
    [user, tenant, isLoading, login, logout]
  );

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

export function useAuth(): AuthState {
  const ctx = useContext(AuthContext);
  if (!ctx) {
    throw new Error("useAuth must be used within an AuthProvider");
  }
  return ctx;
}

export { ApiError };
