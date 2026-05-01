import type {
  ApiResponse,
  AuthUser,
  CheckRecord,
  ComponentItem,
  DashboardSummary,
  GlobalSubscriber,
  LatestVersionInfo,
  MailAuthStatus,
  NotificationRecord,
  PageData,
  Subscriber,
  SystemRun,
} from '../types/domain';

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(path, {
    headers: { 'Content-Type': 'application/json', ...init?.headers },
    credentials: 'same-origin',
    ...init,
  });
  const payload = (await response.json()) as ApiResponse<T>;
  if (!response.ok || payload.code !== 0) {
    throw new Error(payload.message || `request failed: ${response.status}`);
  }
  return payload.data;
}

export const api = {
  me: () => request<AuthUser>('/api/auth/me'),
  login: (username: string, password: string) =>
    request<AuthUser>('/api/auth/login', {
      method: 'POST',
      body: JSON.stringify({ username, password }),
    }),
  logout: () => request<{ logged_out: boolean }>('/api/auth/logout', { method: 'POST' }),
  dashboard: () => request<DashboardSummary>('/api/dashboard/summary'),
  components: (params?: Record<string, string | number | boolean | undefined>) =>
    request<PageData<ComponentItem>>(`/api/components?${query({ page: 1, page_size: 100, ...params })}`),
  createComponent: (component: Partial<ComponentItem>) =>
    request<ComponentItem>('/api/components', {
      method: 'POST',
      body: JSON.stringify(component),
    }),
  latestComponentVersion: (params: { repo_url: string; check_strategy?: string }) =>
    request<LatestVersionInfo>(`/api/components/latest-version?${query(params)}`),
  updateComponent: (id: number, component: Partial<ComponentItem>) =>
    request<ComponentItem>(`/api/components/${id}`, {
      method: 'PUT',
      body: JSON.stringify(component),
    }),
  deleteComponent: (id: number) =>
    request<{ deleted: boolean }>(`/api/components/${id}`, { method: 'DELETE' }),
  checkComponent: (id: number) =>
    request<CheckRecord>(`/api/components/${id}/check`, { method: 'POST' }),
  systemRuns: () => request<PageData<SystemRun>>('/api/system-runs?page=1&page_size=10'),
  subscribers: (componentId: number) =>
    request<Subscriber[]>(`/api/components/${componentId}/subscribers`),
  createSubscriber: (componentId: number, subscriber: Partial<Subscriber>) =>
    request<Subscriber>(`/api/components/${componentId}/subscribers`, {
      method: 'POST',
      body: JSON.stringify(subscriber),
    }),
  updateSubscriber: (id: number, subscriber: Partial<Subscriber>) =>
    request<Subscriber>(`/api/subscribers/${id}`, {
      method: 'PUT',
      body: JSON.stringify(subscriber),
    }),
  deleteSubscriber: (id: number) =>
    request<{ deleted: boolean }>(`/api/subscribers/${id}`, { method: 'DELETE' }),
  globalSubscribers: () => request<GlobalSubscriber[]>('/api/global-subscribers'),
  globalSubscriber: (id: number) => request<GlobalSubscriber>(`/api/global-subscribers/${id}`),
  createGlobalSubscriber: (subscriber: Partial<GlobalSubscriber>) =>
    request<GlobalSubscriber>('/api/global-subscribers', {
      method: 'POST',
      body: JSON.stringify(subscriber),
    }),
  updateGlobalSubscriber: (id: number, subscriber: Partial<GlobalSubscriber>) =>
    request<GlobalSubscriber>(`/api/global-subscribers/${id}`, {
      method: 'PUT',
      body: JSON.stringify(subscriber),
    }),
  updateGlobalSubscriberComponents: (id: number, payload: { all_components: boolean; component_ids: number[] }) =>
    request<GlobalSubscriber>(`/api/global-subscribers/${id}/components`, {
      method: 'PUT',
      body: JSON.stringify(payload),
    }),
  deleteGlobalSubscriber: (id: number) =>
    request<{ deleted: boolean }>(`/api/global-subscribers/${id}`, { method: 'DELETE' }),
  checkRecords: (params?: Record<string, string | number | boolean | undefined>) =>
    request<PageData<CheckRecord>>(`/api/check-records?${query({ page: 1, page_size: 50, ...params })}`),
  checkRecord: (id: number) => request<CheckRecord>(`/api/check-records/${id}`),
  notifications: (params?: Record<string, string | number | boolean | undefined>) =>
    request<PageData<NotificationRecord>>(`/api/notification-records?${query({ page: 1, page_size: 50, ...params })}`),
  testNotification: (recipient: string) =>
    request<{ sent: boolean }>('/api/notification-records/test', {
      method: 'POST',
      body: JSON.stringify({ recipient }),
    }),
  notification: (id: number) => request<NotificationRecord>(`/api/notification-records/${id}`),
  mailStatus: () => request<MailAuthStatus>('/api/mail/status'),
};

function query(params: Record<string, string | number | boolean | undefined>) {
  const values = new URLSearchParams();
  Object.entries(params).forEach(([key, value]) => {
    if (value !== undefined && value !== '') {
      values.set(key, String(value));
    }
  });
  return values.toString();
}
