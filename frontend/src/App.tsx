import { useEffect, useRef, useState, type ReactNode } from 'react';
import {
  Alert,
  Button,
  Card,
  Descriptions,
  Drawer,
  Grid,
  Form,
  Input,
  Layout,
  Modal,
  Popconfirm,
  Select,
  Space,
  Spin,
  Switch,
  Table,
  Tag,
  Tooltip,
  Transfer,
  Tabs,
  message,
} from 'antd';
import type { ColumnsType } from 'antd/es/table';
import type { FormInstance } from 'antd';
import { api } from './api/client';
import type {
  AuthUser,
  CheckRecord,
  ComponentItem,
  DashboardSummary,
  GlobalSubscriber,
  NotificationRecord,
  MailAuthStatus,
  Subscriber,
  SystemRun,
} from './types/domain';

type PageKey = 'dashboard' | 'components' | 'subscribers' | 'checks' | 'notifications';

export function App() {
  const [user, setUser] = useState<AuthUser | null>();
  const [page, setPage] = useState<PageKey>('dashboard');
  const [mobileNavOpen, setMobileNavOpen] = useState(false);
  const screens = Grid.useBreakpoint();
  const isMobile = !screens.md;

  useEffect(() => {
    if (!isMobile) {
      setMobileNavOpen(false);
    }
  }, [isMobile]);

  useEffect(() => {
    api.me().then(setUser).catch(() => setUser(null));
  }, []);

  async function logout() {
    try {
      await api.logout();
    } catch {
      // The local login state still needs to be cleared if the session is already expired.
    }
    setUser(null);
    setPage('dashboard');
  }

  if (user === undefined) {
    return <div className="boot">Loading...</div>;
  }

  if (!user) {
    return <Login onLogin={setUser} />;
  }

  const navItems: Array<[PageKey, string]> = [
    ['dashboard', '仪表盘'],
    ['components', '组件管理'],
    ['subscribers', '订阅人管理'],
    ['checks', '检查记录'],
    ['notifications', '通知记录'],
  ];

  const pageContent = (
    <>
      {page === 'dashboard' && <Dashboard isMobile={isMobile} />}
      {page === 'components' && <Components isMobile={isMobile} />}
      {page === 'subscribers' && <Subscribers isMobile={isMobile} />}
      {page === 'checks' && <Checks isMobile={isMobile} />}
      {page === 'notifications' && <Notifications isMobile={isMobile} />}
    </>
  );

  if (isMobile) {
    return (
      <div className="shell mobile-shell">
        <header className="mobile-topbar">
          <div className="brand mobile-brand">
            <span className="brand-mark">OR</span>
            <div>
              <strong>Release Watcher</strong>
              <small>开源组件版本感知</small>
            </div>
          </div>
          <div className="mobile-topbar-actions">
            <Button className="mobile-menu-button" onClick={() => setMobileNavOpen(true)}>☰</Button>
            <Button size="small" onClick={() => void logout()}>退出</Button>
          </div>
        </header>
        <Drawer
          className="mobile-nav-drawer"
          open={mobileNavOpen}
          placement="left"
          width={280}
          onClose={() => setMobileNavOpen(false)}
        >
          <div className="mobile-drawer-session">
            <strong>{user.username}</strong>
            <Button size="small" onClick={() => void logout()}>退出登录</Button>
          </div>
          <nav className="nav mobile-nav">
            {navItems.map(([key, label]) => (
              <button
                key={key}
                className={page === key ? 'active' : ''}
                onClick={() => {
                  setPage(key);
                  setMobileNavOpen(false);
                }}
              >
                {label}
              </button>
            ))}
          </nav>
        </Drawer>
        <main className="content mobile-content">
          {pageContent}
        </main>
      </div>
    );
  }

  return (
    <Layout className="shell">
      <Layout.Sider width={260} className="side">
        <div className="brand">
          <span className="brand-mark">OR</span>
          <div>
            <strong>Release Watcher</strong>
            <small>开源组件版本感知</small>
          </div>
        </div>
        <div className="session">
          <span>{user.username}</span>
          <Button size="small" onClick={() => void logout()}>退出</Button>
        </div>
        <nav className="nav">
          {navItems.map(([key, label]) => (
            <button key={key} className={page === key ? 'active' : ''} onClick={() => setPage(key as PageKey)}>
              {label}
            </button>
          ))}
        </nav>
      </Layout.Sider>
      <Layout.Content className="content">
        {pageContent}
      </Layout.Content>
    </Layout>
  );
}

function Login({ onLogin }: { onLogin: (user: AuthUser) => void }) {
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');

  async function submit(values: { username: string; password: string }) {
    setLoading(true);
    setError('');
    try {
      onLogin(await api.login(values.username, values.password));
    } catch (nextError) {
      setError(nextError instanceof Error ? nextError.message : String(nextError));
    } finally {
      setLoading(false);
    }
  }

  return (
    <main className="login-shell">
      <section className="login-panel">
        <div className="login-brand">
          <span className="brand-mark">OR</span>
          <div>
            <h1>Release Watcher</h1>
            <p>开源组件版本感知</p>
          </div>
        </div>
        {error && <Alert className="login-alert" type="error" message={error} showIcon />}
        <Form layout="vertical" initialValues={{ username: 'admin' }} onFinish={submit}>
          <Form.Item name="username" label="用户名" rules={[{ required: true }]}>
            <Input autoComplete="username" />
          </Form.Item>
          <Form.Item name="password" label="密码" rules={[{ required: true }]}>
            <Input.Password autoComplete="current-password" />
          </Form.Item>
          <Button type="primary" htmlType="submit" loading={loading} block>
            登录
          </Button>
        </Form>
      </section>
    </main>
  );
}

function Dashboard({ isMobile }: { isMobile: boolean }) {
  const [summary, setSummary] = useState<DashboardSummary>();
  const [runs, setRuns] = useState<SystemRun[]>([]);
  const [checkRecords, setCheckRecords] = useState<CheckRecord[]>([]);
  const [notifications, setNotifications] = useState<NotificationRecord[]>([]);
  const [mailStatus, setMailStatus] = useState<MailAuthStatus>();
  const [loading, setLoading] = useState(false);

  async function load() {
    setLoading(true);
    try {
      const [nextSummary, nextRuns, nextMailStatus, nextCheckRecords, nextNotifications] = await Promise.all([
        api.dashboard(),
        api.systemRuns(),
        api.mailStatus(),
        api.checkRecords({ page_size: 8, has_update: true }),
        api.notifications({ page_size: 8 }),
      ]);
      setSummary(nextSummary);
      setRuns(nextRuns.items);
      setMailStatus(nextMailStatus);
      setCheckRecords(nextCheckRecords.items);
      setNotifications(nextNotifications.items);
    } catch (error) {
      message.error(String(error));
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void load();
  }, []);

  const latestRun = runs[0];
  const recentUpdates = checkRecords.slice(0, 5);
  const recentNotificationRecords = notifications.slice(0, 5);
  const dashboardLoading = loading && !summary;
  const latestCheckAt = summary?.last_full_check_at ?? latestRun?.finished_at ?? latestRun?.started_at;
  const successRate = latestRun && latestRun.total_count > 0 ? latestRun.success_count / latestRun.total_count : null;
  const metricCards = [
    { label: '组件总数', value: summary?.component_total ?? 0, tone: 'neutral' as const },
    { label: '启用监控', value: summary?.enabled_component_total ?? 0, tone: 'neutral' as const },
    { label: '最近发现更新', value: summary?.components_with_update ?? 0, tone: 'warning' as const },
    { label: '检查异常', value: summary?.last_check_failed_total ?? 0, tone: 'danger' as const },
    { label: '通知异常', value: summary?.notification_failed_total ?? 0, tone: 'danger' as const },
  ];

  const healthRows: Array<{ label: string; value: string; extra?: string }> = [
    {
      label: '调度状态',
      value: !latestRun ? '待运行' : latestRun.status === 'running' ? '运行中' : latestRun.status === 'failed' ? '异常' : '正常',
      extra: latestRun ? `最近检查 ${formatClock(latestCheckAt)}` : '尚未执行过检查',
    },
    {
      label: 'Mail',
      value: mailStatus
        ? (mailStatus.configured ? (mailStatus.connected ? '正常' : '异常') : '未配置')
        : '待检测',
      extra: mailStatus?.message ?? '',
    },
    {
      label: '最近检查',
      value: formatClock(latestCheckAt),
      extra: latestRun ? `检查结果 ${latestRun.success_count}/${latestRun.total_count} 成功` : '暂无运行记录',
    },
  ];

  const alertRows: Array<{ label: string; value: string }> = [];
  if ((summary?.last_check_failed_total ?? 0) > 0) {
    alertRows.push({
      label: '检查异常',
      value: `${summary?.last_check_failed_total ?? 0} 条`,
    });
  }
  if ((summary?.notification_failed_total ?? 0) > 0) {
    alertRows.push({
      label: '通知异常',
      value: `${summary?.notification_failed_total ?? 0} 条`,
    });
  }

  const notificationByCheckRecordId = new Map<number, NotificationRecord[]>();
  notifications.forEach(item => {
    const bucket = notificationByCheckRecordId.get(item.check_record_id);
    if (bucket) {
      bucket.push(item);
    } else {
      notificationByCheckRecordId.set(item.check_record_id, [item]);
    }
  });

  const recentUpdateRows = recentUpdates.map(item => {
    const relatedNotifications = notificationByCheckRecordId.get(item.id) ?? [];
    const notificationMeta = relatedNotifications.some(notification => notification.status === 'sent')
      ? { label: '已通知', tone: 'success' as const }
      : relatedNotifications.some(notification => notification.status === 'failed')
        ? { label: '通知失败', tone: 'danger' as const }
        : { label: '待通知', tone: 'warning' as const };
    return {
      id: item.id,
      title: item.component_name,
      versionRange: `${item.previous_version || '-'} → ${item.latest_version || '-'}`,
      checkedAt: formatClock(item.checked_at),
      meta: notificationMeta,
    };
  });

  const recentNotificationRows = recentNotificationRecords.map(item => ({
    id: item.id,
    title: item.component_name,
    target: item.recipient_email,
    version: item.version || '-',
    sentAt: formatClock(item.sent_at ?? item.created_at),
    meta: item.status === 'sent'
      ? { label: '通知成功', tone: 'success' as const }
      : { label: '通知失败', tone: 'danger' as const },
  }));

  const statusPill = latestRun
    ? latestRun.status === 'failed'
      ? '最近失败'
      : latestRun.status === 'running'
        ? '运行中'
        : '调度正常'
    : '待运行';

  return (
    <section className="dashboard-page">
      <PageHeader
        title="仪表盘"
        description="查看开源组件监控整体状态。"
        action={(
          <div className="dashboard-header-actions">
            <div className="dashboard-status-strip">
              <Tag color={latestRun?.status === 'failed' ? 'red' : latestRun?.status === 'running' ? 'blue' : 'green'}>
                {statusPill}
              </Tag>
              <span>最近检查：{formatClock(latestCheckAt)}</span>
            </div>
          </div>
        )}
      />
      <div className="metric-grid dashboard-metric-grid">
        {metricCards.map(card => (
          <Card key={card.label} className={`metric dashboard-metric dashboard-metric-${card.tone}`} loading={dashboardLoading}>
            <small>{card.label}</small>
            <strong>{card.value}</strong>
          </Card>
        ))}
      </div>
      <div className="dashboard-split-grid">
        <Card
          className="dashboard-panel"
          title={(
            <div className="dashboard-panel-title">
              <span>系统概览</span>
            </div>
          )}
          loading={dashboardLoading}
        >
          <div className="dashboard-health-list">
            {healthRows.map(item => (
              <div key={item.label} className="dashboard-health-row">
                <div>
                  <strong>{item.label}</strong>
                  {item.extra ? <span>{item.extra}</span> : null}
                </div>
                <Tag color={dashboardTagColor(item.value)}>
                  {item.value}
                </Tag>
              </div>
            ))}
          </div>
          <div className="dashboard-trend-list dashboard-health-metrics">
            <div className="dashboard-trend-row">
              <div className="dashboard-trend-row-head">
                <span>检查结果</span>
                <strong>{latestRun ? `${latestRun.success_count}/${latestRun.total_count} 成功` : '-'}</strong>
              </div>
              <div className="dashboard-trend-bar"><span style={{ width: `${Math.round((successRate ?? 0) * 100)}%` }} /></div>
              <small>{latestRun ? `最近一次运行 ${formatPercent(successRate)}` : '暂无运行记录'}</small>
            </div>
          </div>
        </Card>
        <Card
          className="dashboard-panel"
          title={(
            <div className="dashboard-panel-title">
              <span>异常提醒</span>
            </div>
          )}
          loading={dashboardLoading}
        >
          {alertRows.length === 0 ? (
            <DashboardEmptyState
              title="暂无异常"
            />
          ) : (
            <div className="dashboard-alert-list">
              {alertRows.map(item => (
                <div key={item.label} className="dashboard-alert-row">
                  <div>
                    <strong>{item.label}</strong>
                    <span>最近一次检查结果</span>
                  </div>
                  <Tag color="red">{item.value}</Tag>
                </div>
              ))}
            </div>
          )}
        </Card>
      </div>
      <Card
        className="dashboard-panel"
        title={(
          <div className="dashboard-panel-title">
            <span>最近发现更新</span>
            <small>{recentUpdateRows.length} 条</small>
          </div>
        )}
        loading={dashboardLoading}
      >
        {recentUpdateRows.length === 0 ? (
          <DashboardEmptyState
            title="暂无发现更新"
          />
        ) : (
          <div className="dashboard-compact-list">
            {recentUpdateRows.map(item => (
              <div key={item.id} className="dashboard-compact-item">
                <div className="dashboard-compact-item-head">
                  <strong>{item.title}</strong>
                  <Tag color={dashboardCompactTagColor(item.meta.tone)}>{item.meta.label}</Tag>
                </div>
                <div className="dashboard-compact-item-version">{item.versionRange}</div>
                <div className="dashboard-compact-item-footer">检查于 {item.checkedAt}</div>
              </div>
            ))}
          </div>
        )}
      </Card>
      <Card
        className="dashboard-panel"
        title={(
          <div className="dashboard-panel-title">
            <span>最近通知记录</span>
            <small>{recentNotificationRows.length} 条</small>
          </div>
        )}
        loading={dashboardLoading}
      >
        {recentNotificationRows.length === 0 ? (
          <DashboardEmptyState
            title="暂无通知记录"
          />
        ) : (
          <div className="dashboard-compact-list">
            {recentNotificationRows.map(item => (
              <div key={item.id} className="dashboard-compact-item">
                <div className="dashboard-compact-item-head">
                  <strong>{item.title}</strong>
                  <Tag color={dashboardCompactTagColor(item.meta.tone)}>{item.meta.label}</Tag>
                </div>
                <div className="dashboard-compact-item-version">{item.version} · {item.target}</div>
                <div className="dashboard-compact-item-footer">发送于 {item.sentAt}</div>
              </div>
            ))}
          </div>
        )}
      </Card>
    </section>
  );
}

function formatDuration(seconds?: number | null) {
  if (seconds === null || seconds === undefined || !Number.isFinite(seconds)) return '-';
  if (seconds < 60) return `${seconds}s`;
  const minutes = Math.floor(seconds / 60);
  const remainder = seconds % 60;
  if (minutes < 60) return `${minutes}m${remainder ? ` ${remainder}s` : ''}`;
  const hours = Math.floor(minutes / 60);
  const minuteRemainder = minutes % 60;
  return `${hours}h${minuteRemainder ? ` ${minuteRemainder}m` : ''}`;
}

function formatInterval(seconds?: number) {
  if (!seconds || seconds <= 0) return '-';
  return formatDuration(seconds);
}

function DashboardEmptyState(props: { title: string }) {
  return (
    <div className="dashboard-empty">
      <strong>{props.title}</strong>
    </div>
  );
}

function Components({ isMobile }: { isMobile: boolean }) {
  const [items, setItems] = useState<ComponentItem[]>([]);
  const [loading, setLoading] = useState(false);
  const [editing, setEditing] = useState<ComponentItem | null>(null);
  const [form] = Form.useForm<Partial<ComponentItem>>();

  async function load() {
    setLoading(true);
    try {
      setItems((await api.components()).items);
    } catch (error) {
      message.error(String(error));
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void load();
  }, []);

  function openEditor(item?: ComponentItem) {
    const next = item ?? emptyComponent();
    setEditing(next);
    form.setFieldsValue(next);
  }

  async function saveComponent(values: Partial<ComponentItem>) {
    try {
      if (editing?.id) {
        await api.updateComponent(editing.id, values);
        message.success('组件已更新');
      } else {
        await api.createComponent(values);
        message.success('组件已创建');
      }
      setEditing(null);
      form.resetFields();
      await load();
    } catch (error) {
      message.error(String(error));
    }
  }

  async function toggleEnabled(row: ComponentItem, enabled: boolean) {
    try {
      await api.updateComponent(row.id, { ...row, enabled });
      await load();
    } catch (error) {
      message.error(String(error));
    }
  }

  async function remove(id: number) {
    try {
      await api.deleteComponent(id);
      message.success('组件已删除');
      await load();
    } catch (error) {
      message.error(String(error));
    }
  }

  async function check(id: number) {
    try {
      await api.checkComponent(id);
      message.success('检查完成');
      await load();
    } catch (error) {
      message.error(String(error));
    }
  }

  const columns: ColumnsType<ComponentItem> = [
    { title: '组件', dataIndex: 'name' },
    { title: '仓库', render: (_, row) => <a href={row.repo_url} target="_blank">{row.repo_url}</a> },
    { title: '当前版本', dataIndex: 'current_version' },
    { title: '最新版本', dataIndex: 'latest_version', render: value => value || '-' },
    { title: '启用', dataIndex: 'enabled', render: (_, row) => <Switch checked={row.enabled} onChange={checked => void toggleEnabled(row, checked)} /> },
    { title: '状态', render: (_, row) => <ComponentStatusLight status={row.last_check_status} /> },
    { title: '检查时间', dataIndex: 'last_checked_at', render: formatTime },
    {
      title: '操作',
      render: (_, row) => (
        <Space className="component-actions">
          <Tooltip title="检查">
            <Button aria-label="检查" className="icon-action" size="small" shape="circle" onClick={() => void check(row.id)}>↻</Button>
          </Tooltip>
          <Tooltip title="编辑">
            <Button aria-label="编辑" className="icon-action" size="small" shape="circle" onClick={() => openEditor(row)}>✎</Button>
          </Tooltip>
          <Popconfirm title="删除这个组件？" onConfirm={() => void remove(row.id)}>
            <Tooltip title="删除">
              <Button aria-label="删除" className="icon-action" size="small" shape="circle" danger>×</Button>
            </Tooltip>
          </Popconfirm>
        </Space>
      ),
    },
  ];

  return (
    <section>
      <PageHeader title="组件管理" description="维护开源组件清单和当前内部使用版本。" action={<Button type="primary" onClick={() => openEditor()}>新增组件</Button>} />
      {isMobile ? (
        <MobileComponentList
          loading={loading}
          items={items}
          onCheck={check}
          onEdit={openEditor}
          onRemove={remove}
          onToggle={toggleEnabled}
        />
      ) : (
        <Table rowKey="id" loading={loading} columns={columns} dataSource={items} pagination={{ pageSize: 10 }} scroll={{ x: 1100 }} size="middle" />
      )}
      <ComponentModal
        form={form}
        open={editing !== null}
        title={editing?.id ? '编辑组件' : '新增组件'}
        isMobile={isMobile}
        onCancel={() => setEditing(null)}
        onFinish={saveComponent}
      />
    </section>
  );
}

function Subscribers({ isMobile }: { isMobile: boolean }) {
  const [items, setItems] = useState<GlobalSubscriber[]>([]);
  const [components, setComponents] = useState<ComponentItem[]>([]);
  const [editor, setEditor] = useState<{ subscriber: GlobalSubscriber | null; activeTab: 'basic' | 'modules' | 'notifications' } | null>(null);
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [recentNotifications, setRecentNotifications] = useState<NotificationRecord[]>([]);
  const [notificationLoading, setNotificationLoading] = useState(false);
  const [lastActiveTab, setLastActiveTab] = useState<'basic' | 'modules' | 'notifications'>('basic');

  useEffect(() => {
    void load();
  }, []);

  async function load() {
    setLoading(true);
    try {
      const [nextItems, nextComponents] = await Promise.all([api.globalSubscribers(), api.components()]);
      setItems(nextItems);
      setComponents(nextComponents.items);
    } catch (error) {
      message.error(String(error));
    } finally {
      setLoading(false);
    }
  }

  function openEditor(item?: GlobalSubscriber, activeTab?: 'basic' | 'modules' | 'notifications') {
    if (!item) {
      setRecentNotifications([]);
    }
    setEditor({ subscriber: item ?? null, activeTab: activeTab ?? (item ? lastActiveTab : 'basic') });
  }

  async function save(values: Partial<GlobalSubscriber>) {
    setSaving(true);
    try {
      if (editor?.subscriber?.id) {
        const next = await api.updateGlobalSubscriber(editor.subscriber.id, values);
        message.success('订阅人已更新');
        setEditor(prev => prev ? { ...prev, subscriber: next } : prev);
      } else {
        const created = await api.createGlobalSubscriber(values);
        message.success('订阅人已创建');
        setEditor({ subscriber: created, activeTab: 'modules' });
      }
      await load();
    } catch (error) {
      message.error(String(error));
    } finally {
      setSaving(false);
    }
  }

  async function remove(id: number) {
    try {
      await api.deleteGlobalSubscriber(id);
      message.success('订阅人已删除');
      if (editor?.subscriber?.id === id) {
        setEditor(null);
      }
      await load();
    } catch (error) {
      message.error(String(error));
    }
  }

  async function toggle(row: GlobalSubscriber, enabled: boolean) {
    try {
      await api.updateGlobalSubscriber(row.id, { enabled, name: row.name, email: row.email });
      await load();
    } catch (error) {
      message.error(String(error));
    }
  }

  async function saveModules(payload: { component_ids: number[] }) {
    if (!editor?.subscriber) {
      return;
    }
    setSaving(true);
    try {
      const next = await api.updateGlobalSubscriberComponents(editor.subscriber.id, {
        all_components: false,
        component_ids: payload.component_ids,
      });
      setEditor(prev => prev ? { ...prev, subscriber: next } : prev);
      message.success('订阅模块已更新');
      await load();
    } catch (error) {
      message.error(String(error));
    } finally {
      setSaving(false);
    }
  }

  async function loadNotifications(email: string) {
    setNotificationLoading(true);
    try {
      const page = await api.notifications({ recipient_email: email, page_size: 10 });
      setRecentNotifications(page.items);
    } catch (error) {
      message.error(String(error));
    } finally {
      setNotificationLoading(false);
    }
  }

  useEffect(() => {
    if (editor?.subscriber && editor.activeTab === 'notifications') {
      void loadNotifications(editor.subscriber.email);
    }
  }, [editor?.subscriber?.email, editor?.activeTab]);

  return (
    <>
      <div className="table-actions">
        <Button type="primary" onClick={() => openEditor()}>新增订阅人</Button>
      </div>
      {isMobile ? (
        <MobileSubscriberList
          loading={loading}
          items={items}
          components={components}
          onEdit={openEditor}
          onRemove={remove}
          onToggle={toggle}
        />
      ) : (
        <Table
          rowKey="id"
          loading={loading}
          dataSource={items}
          pagination={false}
          scroll={{ x: 960 }}
          size="middle"
          columns={[
            { title: '名称', dataIndex: 'name' },
            { title: '邮箱', dataIndex: 'email' },
            {
              title: '订阅模块',
              render: (_, row) => (
                row.all_components ? (
                  <Tag color="green">全部组件</Tag>
                ) : (
                  <Tooltip
                    title={
                      (row.component_ids ?? []).length
                        ? (row.component_ids ?? []).map(id => components.find(item => item.id === id)?.name ?? `#${id}`).join('、')
                        : '未选择任何组件'
                    }
                  >
                    <Space size={[4, 4]} wrap>
                      {(row.component_ids ?? []).slice(0, 3).map(id => {
                        const component = components.find(item => item.id === id);
                        return <Tag key={id}>{component?.name ?? `#${id}`}</Tag>;
                      })}
                      {(row.component_ids?.length ?? 0) > 3 && <Tag>+{(row.component_ids?.length ?? 0) - 3}</Tag>}
                      {(row.component_ids?.length ?? 0) === 0 && <Tag color="orange">未选择</Tag>}
                    </Space>
                  </Tooltip>
                )
              ),
            },
            { title: '启用', dataIndex: 'enabled', render: (_, row) => <Switch checked={row.enabled} onChange={checked => void toggle(row, checked)} /> },
            { title: '创建时间', dataIndex: 'created_at', render: formatTime },
            {
              title: '操作',
              render: (_, row) => (
                <Space className="subscriber-actions">
                  <Tooltip title="编辑">
                    <Button aria-label="编辑" className="icon-action" size="small" shape="circle" onClick={() => openEditor(row)}>✎</Button>
                  </Tooltip>
                  <Popconfirm title="删除这个订阅人？" onConfirm={() => void remove(row.id)}>
                    <Tooltip title="删除">
                      <Button aria-label="删除" className="icon-action" size="small" shape="circle" danger>×</Button>
                    </Tooltip>
                  </Popconfirm>
                </Space>
              ),
            },
          ]}
        />
      )}
      <SubscriberDetailDrawer
        open={editor !== null}
        subscriber={editor ? editor.subscriber : null}
        components={components}
        loading={saving}
        activeTab={editor?.activeTab ?? 'basic'}
        isMobile={isMobile}
        onClose={() => setEditor(null)}
        onChangeTab={tab => {
          setLastActiveTab(tab);
          setEditor(prev => prev ? { ...prev, activeTab: tab } : prev);
        }}
        onSaveBasic={save}
        onSave={saveModules}
        notifications={recentNotifications}
        notificationsLoading={notificationLoading}
        onRefreshNotifications={() => {
          if (editor?.subscriber) {
            void loadNotifications(editor.subscriber.email);
          }
        }}
      />
    </>
  );
}

function SubscriberDetailDrawer(props: {
  open: boolean;
  subscriber: GlobalSubscriber | null;
  components: ComponentItem[];
  loading: boolean;
  activeTab: 'basic' | 'modules' | 'notifications';
  isMobile: boolean;
  notifications: NotificationRecord[];
  notificationsLoading: boolean;
  onClose: () => void;
  onChangeTab: (tab: 'basic' | 'modules' | 'notifications') => void;
  onSaveBasic: (values: Partial<GlobalSubscriber>) => void | Promise<void>;
  onSave: (payload: { component_ids: number[] }) => void | Promise<void>;
  onRefreshNotifications: () => void | Promise<void>;
}) {
  const [basicForm] = Form.useForm<Partial<GlobalSubscriber>>();
  const [moduleForm] = Form.useForm<{ component_ids: number[] }>();

  useEffect(() => {
    if (!props.subscriber) {
      basicForm.resetFields();
      moduleForm.resetFields();
      basicForm.setFieldsValue({ enabled: true });
      moduleForm.setFieldsValue({ component_ids: [] });
      return;
    }
    const selectedComponentIds = props.subscriber.all_components
      ? props.components.map(item => item.id)
      : props.subscriber.component_ids ?? [];
    basicForm.setFieldsValue({
      name: props.subscriber.name,
      email: props.subscriber.email,
      enabled: props.subscriber.enabled,
    });
    moduleForm.setFieldsValue({
      component_ids: selectedComponentIds,
    });
  }, [basicForm, moduleForm, props.components, props.subscriber]);

  function getSelectedIds() {
    return (moduleForm.getFieldValue('component_ids') ?? []) as number[];
  }

  return (
    <Drawer
      width={props.isMobile ? '100vw' : 'min(780px, 100vw)'}
      title={props.subscriber ? `${props.subscriber.name} 的订阅详情` : '新增订阅人'}
      open={props.open}
      onClose={props.onClose}
      destroyOnHidden
      extra={props.activeTab === 'basic'
        ? <Button type="primary" loading={props.loading} onClick={() => basicForm.submit()}>保存</Button>
        : props.activeTab === 'modules'
          ? <Button type="primary" loading={props.loading} onClick={() => moduleForm.submit()}>保存</Button>
          : <Button onClick={() => void props.onRefreshNotifications()}>刷新记录</Button>}
    >
      <Tabs
        activeKey={props.activeTab}
        onChange={key => props.onChangeTab(key as 'basic' | 'modules' | 'notifications')}
        items={[
          {
            key: 'basic',
            label: '基础信息',
            children: (
              <Form
                form={basicForm}
                layout="vertical"
                onFinish={props.onSaveBasic}
                initialValues={{ enabled: true }}
              >
                <Form.Item name="name" label="名称" rules={[{ required: true }]}>
                  <Input />
                </Form.Item>
                <Form.Item name="email" label="邮箱" rules={[{ required: true, type: 'email' }]}>
                  <Input />
                </Form.Item>
                <Form.Item name="enabled" label="启用" valuePropName="checked">
                  <Switch />
                </Form.Item>
              </Form>
            ),
          },
          {
            key: 'modules',
            label: '订阅模块',
            children: (
              <Form
                form={moduleForm}
                layout="vertical"
                onFinish={values => props.onSave({ component_ids: values.component_ids ?? [] })}
                initialValues={{ component_ids: [] }}
              >
                <Form.Item shouldUpdate noStyle>
                  {({ getFieldValue }) => (
                    <Form.Item label="选择订阅组件">
                      <Transfer
                        dataSource={props.components.map(item => ({
                          key: String(item.id),
                          title: item.name,
                        }))}
                        titles={['全部组件', '已选组件']}
                        targetKeys={(getFieldValue('component_ids') ?? []).map((id: number) => String(id))}
                        onChange={keys => moduleForm.setFieldValue('component_ids', keys.map(key => Number(key)))}
                        render={item => item.title}
                        showSearch
                        listStyle={props.isMobile ? { width: '100%', height: 280 } : { width: 340, height: 360 }}
                        locale={{ itemUnit: '项', itemsUnit: '项', searchPlaceholder: '搜索组件' }}
                      />
                    </Form.Item>
                  )}
                </Form.Item>
                <Form.Item
                  hidden
                  name="component_ids"
                  rules={[{
                    validator: async (_, value) => {
                      if (Array.isArray(value) && value.length > 0) {
                        return;
                      }
                      throw new Error('请选择至少一个组件');
                    },
                  }]}
                />
              </Form>
            ),
          },
          {
            key: 'notifications',
            label: '最近通知',
            children: (
              <Table
                rowKey="id"
                loading={props.notificationsLoading}
                dataSource={props.notifications}
                pagination={false}
                size={props.isMobile ? 'small' : 'middle'}
                columns={[
                  { title: '组件', dataIndex: 'component_name' },
                  { title: '版本', dataIndex: 'version' },
                  { title: '状态', dataIndex: 'status', render: value => <StatusTag status={value} /> },
                  { title: '发送时间', dataIndex: 'sent_at', render: formatTime },
                  { title: '创建时间', dataIndex: 'created_at', render: formatTime },
                ]}
              />
            ),
          },
        ]}
      />
    </Drawer>
  );
}

function Checks({ isMobile }: { isMobile: boolean }) {
  const [items, setItems] = useState<CheckRecord[]>([]);
  const [components, setComponents] = useState<ComponentItem[]>([]);
  const [filters, setFilters] = useState<Record<string, string | number | boolean | undefined>>({});
  const [detail, setDetail] = useState<CheckRecord | null>(null);
  const [loading, setLoading] = useState(false);

  async function load(nextFilters = filters) {
    setLoading(true);
    try {
      const [records, componentPage] = await Promise.all([api.checkRecords(nextFilters), api.components()]);
      setItems(records.items);
      setComponents(componentPage.items);
    } catch (error) {
      message.error(String(error));
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void load();
  }, []);

  async function showDetail(id: number) {
    try {
      setDetail(await api.checkRecord(id));
    } catch (error) {
      message.error(String(error));
    }
  }

  return (
    <section>
      <PageHeader title="检查记录" description="查看每次 GitHub Release/Tag 检查结果。" />
      <FilterBar components={components} filters={filters} onChange={next => { setFilters(next); void load(next); }} includeUpdate />
      {isMobile ? (
        <MobileCheckList items={items} loading={loading} onOpenDetail={showDetail} />
      ) : (
        <Table
          rowKey="id"
          loading={loading}
          dataSource={items}
          size="middle"
          columns={[
            { title: '组件', dataIndex: 'component_name' },
            { title: '来源', dataIndex: 'source' },
            { title: '版本变化', render: (_, row) => `${row.previous_version || '-'} -> ${row.latest_version || '-'}` },
            { title: '是否更新', dataIndex: 'has_update', render: value => value ? <Tag color="orange">有更新</Tag> : <Tag>无</Tag> },
            { title: '状态', dataIndex: 'status', render: value => <StatusTag status={value} /> },
            { title: '失败原因', dataIndex: 'error_message', render: value => value || '-' },
            { title: '检查时间', dataIndex: 'checked_at', render: formatTime },
            { title: '操作', render: (_, row) => <Button size="small" onClick={() => void showDetail(row.id)}>详情</Button> },
          ]}
          scroll={{ x: 1000 }}
        />
      )}
      <Drawer title="检查详情" width={isMobile ? '100vw' : 'min(720px, 100vw)'} open={detail !== null} onClose={() => setDetail(null)}>
        {detail && (
          <Descriptions column={1} bordered size="small">
            <Descriptions.Item label="组件">{detail.component_name}</Descriptions.Item>
            <Descriptions.Item label="来源">{detail.source || '-'}</Descriptions.Item>
            <Descriptions.Item label="版本变化">{detail.previous_version || '-'} -&gt; {detail.latest_version || '-'}</Descriptions.Item>
            <Descriptions.Item label="状态"><StatusTag status={detail.status} /></Descriptions.Item>
            <Descriptions.Item label="发布时间">{formatTime(detail.release_published_at)}</Descriptions.Item>
            <Descriptions.Item label="链接">{detail.release_url ? <a href={detail.release_url} target="_blank">{detail.release_url}</a> : '-'}</Descriptions.Item>
            <Descriptions.Item label="失败原因">{detail.error_message || '-'}</Descriptions.Item>
            <Descriptions.Item label="摘要"><pre className="plain-pre">{detail.release_note_summary || '-'}</pre></Descriptions.Item>
            <Descriptions.Item label="Release Note"><pre className="plain-pre">{detail.release_note || '-'}</pre></Descriptions.Item>
          </Descriptions>
        )}
      </Drawer>
    </section>
  );
}

function Notifications({ isMobile }: { isMobile: boolean }) {
  const [items, setItems] = useState<NotificationRecord[]>([]);
  const [components, setComponents] = useState<ComponentItem[]>([]);
  const [filters, setFilters] = useState<Record<string, string | number | boolean | undefined>>({});
  const [detail, setDetail] = useState<NotificationRecord | null>(null);
  const [testOpen, setTestOpen] = useState(false);
  const [testSending, setTestSending] = useState(false);
  const [loading, setLoading] = useState(false);
  const [testForm] = Form.useForm<{ recipient: string }>();

  async function load(nextFilters = filters) {
    setLoading(true);
    try {
      const [records, componentPage] = await Promise.all([
        api.notifications(nextFilters),
        api.components(),
      ]);
      setItems(records.items);
      setComponents(componentPage.items);
    } catch (error) {
      message.error(String(error));
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void load();
  }, []);

  async function showDetail(id: number) {
    try {
      setDetail(await api.notification(id));
    } catch (error) {
      message.error(String(error));
    }
  }

  async function sendTestMail(values: { recipient: string }) {
    setTestSending(true);
    try {
      await api.testNotification(values.recipient);
      message.success('测试邮件已发送');
      setTestOpen(false);
      testForm.resetFields();
    } catch (error) {
      message.error(String(error));
    } finally {
      setTestSending(false);
    }
  }

  return (
    <section>
      <PageHeader
        title="通知记录"
        description="查看邮件通知发送结果。"
        action={<Button type="primary" onClick={() => setTestOpen(true)}>测试邮件</Button>}
      />
      <FilterBar components={components} filters={filters} onChange={next => { setFilters(next); void load(next); }} />
      {isMobile ? (
        <MobileNotificationList items={items} loading={loading} onOpenDetail={showDetail} />
      ) : (
        <Table
          rowKey="id"
          loading={loading}
          dataSource={items}
          size="middle"
          columns={[
            { title: '组件', dataIndex: 'component_name' },
            { title: '版本', dataIndex: 'version' },
            { title: '收件人', dataIndex: 'recipient_email' },
            { title: '标题', dataIndex: 'subject' },
            { title: '状态', dataIndex: 'status', render: value => <StatusTag status={value} /> },
            { title: '失败原因', dataIndex: 'error_message', render: value => value || '-' },
            { title: '发送时间', dataIndex: 'sent_at', render: formatTime },
            { title: '操作', render: (_, row) => <Button size="small" onClick={() => void showDetail(row.id)}>正文</Button> },
          ]}
          scroll={{ x: 1100 }}
        />
      )}
      <Drawer title="邮件正文" width={isMobile ? '100vw' : 'min(720px, 100vw)'} open={detail !== null} onClose={() => setDetail(null)}>
        {detail && (
          <Descriptions column={1} bordered size="small">
            <Descriptions.Item label="组件">{detail.component_name}</Descriptions.Item>
            <Descriptions.Item label="版本">{detail.version}</Descriptions.Item>
            <Descriptions.Item label="收件人">{detail.recipient_email}</Descriptions.Item>
            <Descriptions.Item label="标题">{detail.subject}</Descriptions.Item>
            <Descriptions.Item label="状态"><StatusTag status={detail.status} /></Descriptions.Item>
            <Descriptions.Item label="失败原因">{detail.error_message || '-'}</Descriptions.Item>
            <Descriptions.Item label="正文"><pre className="plain-pre">{detail.body || '-'}</pre></Descriptions.Item>
          </Descriptions>
        )}
      </Drawer>
      <Modal
        title="发送测试邮件"
        open={testOpen}
        width={isMobile ? 'calc(100vw - 24px)' : 520}
        confirmLoading={testSending}
        onCancel={() => setTestOpen(false)}
        onOk={() => testForm.submit()}
        destroyOnHidden
      >
        <Form form={testForm} layout="vertical" onFinish={sendTestMail}>
          <Form.Item name="recipient" label="收件邮箱" rules={[{ required: true, type: 'email' }]}>
            <Input placeholder="name@example.com" />
          </Form.Item>
        </Form>
      </Modal>
    </section>
  );
}

function ComponentModal(props: {
  form: FormInstance<Partial<ComponentItem>>;
  open: boolean;
  title: string;
  isMobile: boolean;
  onCancel: () => void;
  onFinish: (values: Partial<ComponentItem>) => void | Promise<void>;
}) {
  const autoFilledNameRef = useRef('');
  const autoFilledVersionRef = useRef('');
  const latestVersionRequestRef = useRef(0);
  const [latestVersionLoading, setLatestVersionLoading] = useState(false);

  useEffect(() => {
    if (!props.open) {
      autoFilledNameRef.current = '';
      autoFilledVersionRef.current = '';
      latestVersionRequestRef.current += 1;
      setLatestVersionLoading(false);
    }
  }, [props.open]);

  function handleValuesChange(changed: Partial<ComponentItem>, values: Partial<ComponentItem>) {
    if (changed.name !== undefined && changed.name !== autoFilledNameRef.current) {
      autoFilledNameRef.current = '';
    }
    if (changed.current_version !== undefined && changed.current_version !== autoFilledVersionRef.current) {
      autoFilledVersionRef.current = '';
    }

    if (changed.repo_url !== undefined) {
      const parsedName = parseGitHubRepoName(changed.repo_url);
      const currentName = values.name?.trim() ?? '';
      if (parsedName && (!currentName || currentName === autoFilledNameRef.current)) {
        autoFilledNameRef.current = parsedName;
        props.form.setFieldValue('name', parsedName);
      }
    }

    if (changed.repo_url !== undefined || changed.check_strategy !== undefined) {
      void fetchLatestVersion(values);
    }
  }

  async function fetchLatestVersion(values: Partial<ComponentItem>) {
    const repoURL = values.repo_url?.trim();
    if (!repoURL || !parseGitHubRepoName(repoURL)) return;

    const requestID = latestVersionRequestRef.current + 1;
    latestVersionRequestRef.current = requestID;
    setLatestVersionLoading(true);
    try {
      const info = await api.latestComponentVersion({
        repo_url: repoURL,
        check_strategy: values.check_strategy ?? 'release_first',
      });
      if (latestVersionRequestRef.current !== requestID) return;

      const currentVersion = props.form.getFieldValue('current_version')?.trim() ?? '';
      if (info.version && (!currentVersion || currentVersion === autoFilledVersionRef.current)) {
        autoFilledVersionRef.current = info.version;
        props.form.setFieldValue('current_version', info.version);
      }
    } catch {
      // Users can still enter the current version manually when GitHub lookup is unavailable.
    } finally {
      if (latestVersionRequestRef.current === requestID) {
        setLatestVersionLoading(false);
      }
    }
  }

  return (
    <Modal
      title={props.title}
      open={props.open}
      width={props.isMobile ? 'calc(100vw - 24px)' : 720}
      onCancel={props.onCancel}
      onOk={() => props.form.submit()}
      destroyOnHidden
    >
      <Form form={props.form} layout="vertical" onFinish={props.onFinish} onValuesChange={handleValuesChange} initialValues={{ check_strategy: 'release_first', enabled: true }}>
        <Form.Item name="name" label="组件名称" rules={[{ required: true }]}>
          <Input placeholder="protobuf" />
        </Form.Item>
        <Form.Item name="repo_url" label="GitHub 仓库" rules={[{ required: true }]}>
          <Input placeholder="https://github.com/protocolbuffers/protobuf" />
        </Form.Item>
        <Form.Item name="current_version" label="当前版本" rules={[{ required: true }]}>
          <Input placeholder="3.20.1" suffix={latestVersionLoading ? <Spin size="small" /> : undefined} />
        </Form.Item>
        <Form.Item name="check_strategy" label="检查策略">
          <Select options={[{ label: 'Release 优先', value: 'release_first' }, { label: '仅 Tag', value: 'tag_only' }]} />
        </Form.Item>
        <Form.Item name="enabled" label="启用检查" valuePropName="checked">
          <Switch />
        </Form.Item>
        <Form.Item name="notes" label="备注">
          <Input.TextArea rows={3} />
        </Form.Item>
      </Form>
    </Modal>
  );
}

function parseGitHubRepoName(value?: string) {
  const input = value?.trim();
  if (!input) return '';

  const sshMatch = input.match(/^git@github\.com:([^/]+)\/([^/]+?)(?:\.git)?\/?$/i);
  if (sshMatch?.[2]) return sshMatch[2];

  const normalized = input.match(/^https?:\/\//i) ? input : `https://${input}`;
  try {
    const url = new URL(normalized);
    if (url.hostname.toLowerCase() !== 'github.com') return '';

    const [, , repo] = url.pathname.replace(/\/+$/, '').split('/');
    return repo?.replace(/\.git$/i, '') ?? '';
  } catch {
    const shorthandMatch = input.match(/^[^/\s]+\/([^/\s]+?)(?:\.git)?\/?$/);
    return shorthandMatch?.[1] ?? '';
  }
}

function FilterBar(props: {
  components: ComponentItem[];
  filters: Record<string, string | number | boolean | undefined>;
  includeUpdate?: boolean;
  onChange: (filters: Record<string, string | number | boolean | undefined>) => void;
}) {
  const screens = Grid.useBreakpoint();
  const isMobile = !screens.md;
  const [expanded, setExpanded] = useState(!isMobile);

  useEffect(() => {
    setExpanded(!isMobile);
  }, [isMobile]);

  function patch(key: string, value: string | number | boolean | undefined) {
    props.onChange({ ...props.filters, [key]: value });
  }

  function clearFilters() {
    props.onChange({});
  }

  const activeCount = Object.values(props.filters).filter(value => value !== undefined && value !== '').length;

  return (
    <Card className="toolbar-card">
      <div className="filter-bar-head">
        <div>
          <strong>筛选条件</strong>
          <span>{activeCount ? `已选择 ${activeCount} 项` : '按条件缩小范围'}</span>
        </div>
        <Space size={8}>
          {activeCount > 0 && <Button size="small" onClick={clearFilters}>清空</Button>}
          {isMobile && (
            <Button size="small" type="primary" onClick={() => setExpanded(value => !value)}>
              {expanded ? '收起' : '展开'}
            </Button>
          )}
        </Space>
      </div>
      {(!isMobile || expanded) && (
        <Space className="filter-space" wrap>
          <Select
            allowClear
            showSearch
            className="filter-select"
            placeholder="组件"
            value={props.filters.component_id}
            optionFilterProp="label"
            onChange={value => patch('component_id', value)}
            options={props.components.map(item => ({ label: item.name, value: item.id }))}
          />
          <Select
            allowClear
            className="filter-select"
            placeholder="状态"
            value={props.filters.status}
            onChange={value => patch('status', value)}
            options={[
              { label: 'success', value: 'success' },
              { label: 'failed', value: 'failed' },
              { label: 'sent', value: 'sent' },
            ]}
          />
          {props.includeUpdate && (
            <Select
              allowClear
              className="filter-select"
              placeholder="是否更新"
              value={props.filters.has_update}
              onChange={value => patch('has_update', value)}
              options={[
                { label: '有更新', value: true },
                { label: '无更新', value: false },
              ]}
            />
          )}
        </Space>
      )}
    </Card>
  );
}

function PageHeader(props: { title: string; description: string; action?: ReactNode }) {
  return (
    <header className="page-header">
      <div>
        <h1>{props.title}</h1>
        <p>{props.description}</p>
      </div>
      {props.action}
    </header>
  );
}

function MobileRunList(props: { runs: SystemRun[] }) {
  return (
    <div className="mobile-list">
      {props.runs.length === 0 ? (
        <Card className="mobile-empty">暂无检查记录</Card>
      ) : props.runs.map(run => (
        <Card key={run.id} className="mobile-item-card">
          <div className="mobile-item-head">
            <div>
              <strong>{run.trigger_type === 'manual' ? '手动触发' : '定时任务'}</strong>
              <div className="mobile-item-meta">{formatTime(run.started_at)} - {formatTime(run.finished_at)}</div>
            </div>
            <StatusTag status={run.status} />
          </div>
          <div className="mobile-item-grid">
            <div><span>总数</span><strong>{run.total_count}</strong></div>
            <div><span>成功</span><strong>{run.success_count}</strong></div>
            <div><span>失败</span><strong>{run.failed_count}</strong></div>
          </div>
          {run.error_message && <div className="mobile-item-note">{run.error_message}</div>}
        </Card>
      ))}
    </div>
  );
}

function MobileComponentList(props: {
  items: ComponentItem[];
  loading: boolean;
  onCheck: (id: number) => void | Promise<void>;
  onEdit: (item: ComponentItem) => void;
  onRemove: (id: number) => void | Promise<void>;
  onToggle: (row: ComponentItem, enabled: boolean) => void | Promise<void>;
}) {
  return (
    <div className="mobile-list">
      {props.loading ? (
        <Card className="mobile-empty">加载中...</Card>
      ) : props.items.length === 0 ? (
        <Card className="mobile-empty">暂无组件</Card>
      ) : props.items.map(item => (
        <Card key={item.id} className="mobile-item-card">
          <div className="mobile-item-head">
            <div>
              <strong>{item.name}</strong>
              <div className="mobile-item-meta"><a href={item.repo_url} target="_blank">{item.repo_url}</a></div>
            </div>
            <ComponentStatusLight status={item.last_check_status} />
          </div>
          <div className="mobile-item-grid">
            <div><span>当前版本</span><strong>{item.current_version || '-'}</strong></div>
            <div><span>最新版本</span><strong>{item.latest_version || '-'}</strong></div>
            <div><span>检查时间</span><strong>{formatTime(item.last_checked_at)}</strong></div>
          </div>
          {item.last_check_error && <div className="mobile-item-note">{item.last_check_error}</div>}
          <div className="mobile-item-footer">
            <Space wrap>
              <Switch checked={item.enabled} onChange={checked => void props.onToggle(item, checked)} />
              <Button size="small" onClick={() => void props.onCheck(item.id)}>检查</Button>
              <Button size="small" onClick={() => props.onEdit(item)}>编辑</Button>
              <Popconfirm title="删除这个组件？" onConfirm={() => void props.onRemove(item.id)}>
                <Button size="small" danger>删除</Button>
              </Popconfirm>
            </Space>
          </div>
        </Card>
      ))}
    </div>
  );
}

function MobileSubscriberList(props: {
  items: GlobalSubscriber[];
  components: ComponentItem[];
  loading: boolean;
  onEdit: (item: GlobalSubscriber) => void;
  onRemove: (id: number) => void | Promise<void>;
  onToggle: (row: GlobalSubscriber, enabled: boolean) => void | Promise<void>;
}) {
  return (
    <div className="mobile-list">
      {props.loading ? (
        <Card className="mobile-empty">加载中...</Card>
      ) : props.items.length === 0 ? (
        <Card className="mobile-empty">暂无订阅人</Card>
      ) : props.items.map(item => (
        <Card key={item.id} className="mobile-item-card">
          <div className="mobile-item-head">
            <div>
              <strong>{item.name}</strong>
              <div className="mobile-item-meta">{item.email}</div>
            </div>
            <Tag color={item.all_components ? 'green' : undefined}>{item.all_components ? '全部组件' : '部分组件'}</Tag>
          </div>
          <div className="mobile-tags">
            {item.all_components ? (
              <Tag color="green">全部组件</Tag>
            ) : (item.component_ids ?? []).length ? (
              (item.component_ids ?? []).slice(0, 4).map(id => {
                const component = props.components.find(entry => entry.id === id);
                return <Tag key={id}>{component?.name ?? `#${id}`}</Tag>;
              })
            ) : (
              <Tag color="orange">未选择组件</Tag>
            )}
            {!item.all_components && (item.component_ids?.length ?? 0) > 4 && <Tag>+{(item.component_ids?.length ?? 0) - 4}</Tag>}
          </div>
          <div className="mobile-item-grid">
            <div><span>创建时间</span><strong>{formatTime(item.created_at)}</strong></div>
            <div><span>状态</span><strong>{item.enabled ? '启用' : '停用'}</strong></div>
          </div>
          <div className="mobile-item-footer">
            <Space wrap>
              <Switch checked={item.enabled} onChange={checked => void props.onToggle(item, checked)} />
              <Button size="small" onClick={() => props.onEdit(item)}>编辑</Button>
              <Popconfirm title="删除这个订阅人？" onConfirm={() => void props.onRemove(item.id)}>
                <Button size="small" danger>删除</Button>
              </Popconfirm>
            </Space>
          </div>
        </Card>
      ))}
    </div>
  );
}

function MobileCheckList(props: {
  items: CheckRecord[];
  loading: boolean;
  onOpenDetail: (id: number) => void | Promise<void>;
}) {
  return (
    <div className="mobile-list">
      {props.loading ? (
        <Card className="mobile-empty">加载中...</Card>
      ) : props.items.length === 0 ? (
        <Card className="mobile-empty">暂无检查记录</Card>
      ) : props.items.map(item => (
        <Card key={item.id} className="mobile-item-card">
          <div className="mobile-item-head">
            <div>
              <strong>{item.component_name}</strong>
              <div className="mobile-item-meta">{item.source || '-'} · {formatTime(item.checked_at)}</div>
            </div>
            <StatusTag status={item.status} />
          </div>
          <div className="mobile-item-grid">
            <div><span>版本变化</span><strong>{item.previous_version || '-'} → {item.latest_version || '-'}</strong></div>
            <div><span>是否更新</span><strong>{item.has_update ? '有更新' : '无更新'}</strong></div>
          </div>
          {item.error_message && <div className="mobile-item-note">{item.error_message}</div>}
          <div className="mobile-item-footer">
            <Button size="small" onClick={() => void props.onOpenDetail(item.id)}>查看详情</Button>
          </div>
        </Card>
      ))}
    </div>
  );
}

function MobileNotificationList(props: {
  items: NotificationRecord[];
  loading: boolean;
  onOpenDetail: (id: number) => void | Promise<void>;
}) {
  return (
    <div className="mobile-list">
      {props.loading ? (
        <Card className="mobile-empty">加载中...</Card>
      ) : props.items.length === 0 ? (
        <Card className="mobile-empty">暂无通知记录</Card>
      ) : props.items.map(item => (
        <Card key={item.id} className="mobile-item-card">
          <div className="mobile-item-head">
            <div>
              <strong>{item.component_name}</strong>
              <div className="mobile-item-meta">{item.recipient_email}</div>
            </div>
            <StatusTag status={item.status} />
          </div>
          <div className="mobile-item-grid">
            <div><span>版本</span><strong>{item.version || '-'}</strong></div>
            <div><span>标题</span><strong>{item.subject || '-'}</strong></div>
          </div>
          {item.error_message && <div className="mobile-item-note">{item.error_message}</div>}
          <div className="mobile-item-footer">
            <Button size="small" onClick={() => void props.onOpenDetail(item.id)}>查看正文</Button>
          </div>
        </Card>
      ))}
    </div>
  );
}

function StatusTag({ status }: { status?: string }) {
  if (status === 'success' || status === 'sent' || status === 'normal' || status === 'connected') return <Tag color="green">{status}</Tag>;
  if (status === 'failed' || status === 'degraded') return <Tag color="red">{status}</Tag>;
  if (status === 'running' || status === 'warning') return <Tag color="blue">{status}</Tag>;
  if (status === 'skipped' || status === 'unknown' || status === 'not configured') return <Tag>{status}</Tag>;
  return <Tag>未检查</Tag>;
}

function ComponentStatusLight({ status }: { status?: string }) {
  const meta = componentStatusMeta(status);
  return (
    <span className={`status-light status-light-${meta.type}`}>
      <span className="status-light-dot" />
      <span>{meta.label}</span>
    </span>
  );
}

function componentStatusMeta(status?: string) {
  if (status === 'success') return { type: 'success', label: '成功' };
  if (status === 'failed') return { type: 'failed', label: '失败' };
  if (status === 'running') return { type: 'running', label: '检查中' };
  if (status === 'skipped') return { type: 'neutral', label: '已跳过' };
  return { type: 'neutral', label: '未检查' };
}

function formatTime(value?: string) {
  if (!value) return '-';
  return new Date(value).toLocaleString();
}

function formatClock(value?: string) {
  if (!value) return '-';
  return new Date(value).toLocaleTimeString('zh-CN', {
    hour: '2-digit',
    minute: '2-digit',
    hour12: false,
  });
}

function formatPercent(value: number | null) {
  if (value === null || !Number.isFinite(value)) return '-';
  return `${Math.round(value * 100)}%`;
}

function dashboardTagColor(value: string) {
  if (value === '正常' || value === '已完成' || value === 'success' || value === 'sent' || value === 'connected') return 'green';
  if (value === '异常' || value === '失败' || value === 'degraded' || value === 'failed') return 'red';
  if (value === '运行中' || value === '待运行' || value === '待检测' || value === 'warning' || value === 'running') return 'blue';
  return undefined;
}

function dashboardCompactTagColor(tone: 'success' | 'warning' | 'danger') {
  if (tone === 'success') return 'green';
  if (tone === 'warning') return 'orange';
  return 'red';
}

function emptyComponent(): ComponentItem {
  return {
    id: 0,
    name: '',
    repo_url: '',
    current_version: '',
    latest_version: '',
    check_strategy: 'release_first',
    enabled: true,
    last_check_status: '',
    last_check_error: '',
    notes: '',
    created_at: '',
    updated_at: '',
  };
}
