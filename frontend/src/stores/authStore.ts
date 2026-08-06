import { create } from "zustand";
import {
  getAuthMe,
  postAuthLogin,
  postAuthLogout,
  postAuthRefresh,
} from "@/services/hotkey/hotkey-server/identity";
import { setAccessToken, clearAccessToken, getAccessToken } from "@/lib/authSession";
import { AuthStatus } from "@/lib/domainEnums";

interface AuthState {
  status: AuthStatus;
  user: HotKeyAPI.UserResponse | null;
  error: string | null;

  initialize(): Promise<void>;
  establishSession(data: HotKeyAPI.AuthenticationResponse): Promise<void>;
  login(input: { email: string; password: string }): Promise<void>;
  logout(): Promise<void>;
  clearSession(): void;
}

export const useAuthStore = create<AuthState>()((set, get) => ({
  status: AuthStatus.Initializing,
  user: null,
  error: null,

  initialize: async () => {
    const token = getAccessToken();
    if (!token) {
      // A reload clears the in-memory access token. Recover only through
      // the server-managed HttpOnly refresh cookie.
      try {
        const res = await postAuthRefresh();
        const newToken = res.data?.access_token ?? "";
        if (!newToken) throw new Error("no token");
        setAccessToken(newToken, 900);
      } catch {
        set({ status: AuthStatus.Unauthenticated, user: null, error: null });
        return;
      }
    }
    // Validate the resulting access token and fetch the current user.
    try {
      const res = await getAuthMe();
      set({ status: AuthStatus.Authenticated, user: res.data ?? null, error: null });
    } catch {
      clearAccessToken();
      set({ status: AuthStatus.Unauthenticated, user: null, error: null });
    }
  },

  establishSession: async (data) => {
    const token = data.access_token;
    if (!token) throw new Error("登录响应无效");
    setAccessToken(token, 900);
    const meRes = await getAuthMe();
    set({ status: AuthStatus.Authenticated, user: meRes.data ?? null, error: null });
  },

  login: async (input) => {
    try {
      const res = await postAuthLogin(input);
      const token = res.data?.access_token;
      const userData = res.data?.user;
      if (!token || !userData) throw new Error("登录响应无效");

      await get().establishSession({ access_token: token, user: userData });
    } catch (err: any) {
      set({ status: AuthStatus.Unauthenticated, user: null, error: err.message ?? null });
      throw err;
    }
  },

  logout: async () => {
    const { status } = get();
    if (status === AuthStatus.Unauthenticated) return;
    try {
      await postAuthLogout();
    } catch {
      // API failure ignored — clear local state regardless
    }
    clearAccessToken();
    set({ status: AuthStatus.Unauthenticated, user: null, error: null });
  },

  clearSession: () => {
    clearAccessToken();
    set({ status: AuthStatus.Unauthenticated, user: null, error: null });
  },
}));
