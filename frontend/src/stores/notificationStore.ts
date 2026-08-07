import { create } from "zustand";

export type NotificationTransport = "idle" | "connecting" | "live" | "polling";

const MAX_ITEMS = 100;

interface PersistedNotificationState {
  lastEventID: number;
  readThroughID: number;
}

interface NotificationState extends PersistedNotificationState {
  userID: number | null;
  items: HotKeyAPI.NotificationResponse[];
  unreadCount: number;
  transport: NotificationTransport;
  initializeUser(userID: number): void;
  ingest(items: HotKeyAPI.NotificationResponse[]): HotKeyAPI.NotificationResponse[];
  markAllRead(): void;
  setTransport(transport: NotificationTransport): void;
  reset(): void;
}

function storageKey(userID: number) {
  return `hotkey.notifications.${userID}`;
}

function readPersisted(userID: number): PersistedNotificationState {
  if (typeof window === "undefined") return { lastEventID: 0, readThroughID: 0 };
  try {
    const parsed = JSON.parse(localStorage.getItem(storageKey(userID)) ?? "{}") as Partial<PersistedNotificationState>;
    return {
      lastEventID: Number.isSafeInteger(parsed.lastEventID) && (parsed.lastEventID ?? -1) >= 0 ? parsed.lastEventID! : 0,
      readThroughID: Number.isSafeInteger(parsed.readThroughID) && (parsed.readThroughID ?? -1) >= 0 ? parsed.readThroughID! : 0,
    };
  } catch {
    return { lastEventID: 0, readThroughID: 0 };
  }
}

function persist(userID: number | null, state: PersistedNotificationState) {
  if (typeof window === "undefined" || userID == null) return;
  localStorage.setItem(storageKey(userID), JSON.stringify(state));
}

const initialState = {
  userID: null,
  items: [] as HotKeyAPI.NotificationResponse[],
  lastEventID: 0,
  readThroughID: 0,
  unreadCount: 0,
  transport: "idle" as NotificationTransport,
};

export const useNotificationStore = create<NotificationState>()((set, get) => ({
  ...initialState,
  initializeUser: (userID) => {
    if (get().userID === userID) return;
    const persisted = readPersisted(userID);
    set({ ...initialState, userID, ...persisted });
  },
  ingest: (incoming) => {
    const current = get();
    const known = new Set(current.items.map((item) => item.id).filter((id): id is number => Number.isSafeInteger(id)));
    const accepted: HotKeyAPI.NotificationResponse[] = [];
    for (const item of incoming) {
      if (!Number.isSafeInteger(item.id) || (item.id ?? 0) <= 0 || known.has(item.id!)) continue;
      known.add(item.id!);
      accepted.push(item);
    }
    if (accepted.length === 0) return accepted;
    const items = [...accepted, ...current.items]
      .sort((left, right) => (right.id ?? 0) - (left.id ?? 0))
      .slice(0, MAX_ITEMS);
    const lastEventID = Math.max(current.lastEventID, ...accepted.map((item) => item.id ?? 0));
    const unreadCount = items.filter((item) => (item.id ?? 0) > current.readThroughID).length;
    set({ items, lastEventID, unreadCount });
    persist(current.userID, { lastEventID, readThroughID: current.readThroughID });
    return accepted;
  },
  markAllRead: () => {
    const current = get();
    const readThroughID = Math.max(current.readThroughID, current.lastEventID);
    set({ readThroughID, unreadCount: 0 });
    persist(current.userID, { lastEventID: current.lastEventID, readThroughID });
  },
  setTransport: (transport) => set({ transport }),
  reset: () => set(initialState),
}));
