import { useEffect, useMemo, useRef, useState, type ReactNode } from 'react';
import {
  Alert,
  Button,
  Card,
  Descriptions,
  Drawer,
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
  MailAuthStatus,
  NotificationRecord,
  Subscriber,
  SystemRun,
} from './types/domain';

type PageKey = 'dashboard' | 'components' | 'subscribers' | 'checks' | 'notifications';

export function App() {
  const [user, setUser] = useState<AuthUser | null>();
  const [page, setPage] = useState<PageKey>('dashboard');

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
          {[
            ['dashboard', '仪表盘'],
            ['components', '组件管理'],
            ['subscribers', '订阅人管理'],
            ['checks', '检查记录'],
            ['notifications', '通知记录'],
          ].map(([key, label]) => (
            <button key={key} className={page === key ? 'active' : ''} onClick={() => setPage(key as PageKey)}>
              {label}
            </button>
          ))}
        </nav>
      </Layout.Sider>
      <Layout.Content className="content">
        {page === 'dashboard' && <Dashboard />}
        {page === 'components' && <Components />}
        {page === 'subscribers' && <Subscribers />}
        {page === 'checks' && <Checks />}
        {page === 'notifications' && <Notifications />}
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

function Dashboard() {
  const [summary, setSummary] = useState<DashboardSummary>();
  const [runs, setRuns] = useState<SystemRun[]>([]);
  const [loading, setLoading] = useState(false);

  async function load() {
    setLoading(true);
    try {
      const [nextSummary, nextRuns] = await Promise.all([api.dashboard(), api.systemRuns()]);
      setSummary(nextSummary);
      setRuns(nextRuns.items);
    } catch (error) {
      message.error(String(error));
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void load();
  }, []);

  async function runChecks() {
    setLoading(true);
    try {
      await api.runChecks();
      message.success('全量检查已完成');
      await load();
    } catch (error) {
      message.error(String(error));
    } finally {
      setLoading(false);
    }
  }

  const cards = [
    ['组件总数', summary?.component_total ?? 0],
    ['启用检查', summary?.enabled_component_total ?? 0],
    ['存在更新', summary?.components_with_update ?? 0],
    ['检查失败', summary?.last_check_failed_total ?? 0],
    ['通知失败', summary?.notification_failed_total ?? 0],
  ];

  return (
    <section>
      <PageHeader title="仪表盘" description="查看开源组件监控整体状态。" action={<Button type="primary" loading={loading} onClick={runChecks}>手动全量检查</Button>} />
      <div className="metric-grid">
        {cards.map(([label, value]) => (
          <Card key={label} className="metric">
            <small>{label}</small>
            <strong>{value}</strong>
          </Card>
        ))}
      </div>
      <Card title="最近全量检查">
        <Table
          rowKey="id"
          size="small"
          pagination={false}
          dataSource={runs}
          scroll={{ x: 780 }}
          columns={[
            { title: '触发方式', dataIndex: 'trigger_type' },
            { title: '状态', dataIndex: 'status', render: value => <StatusTag status={value} /> },
            { title: '总数', dataIndex: 'total_count' },
            { title: '成功', dataIndex: 'success_count' },
            { title: '失败', dataIndex: 'failed_count' },
            { title: '开始时间', dataIndex: 'started_at', render: formatTime },
            { title: '结束时间', dataIndex: 'finished_at', render: formatTime },
          ]}
        />
      </Card>
    </section>
  );
}

function Components() {
  const [items, setItems] = useState<ComponentItem[]>([]);
  const [loading, setLoading] = useState(false);
  const [editing, setEditing] = useState<ComponentItem | null>(null);
  const [subscribersFor, setSubscribersFor] = useState<ComponentItem | null>(null);
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
          <Tooltip title="订阅人">
            <Button aria-label="订阅人" className="icon-action" size="small" shape="circle" onClick={() => setSubscribersFor(row)}>@</Button>
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
      <Table rowKey="id" loading={loading} columns={columns} dataSource={items} pagination={{ pageSize: 10 }} scroll={{ x: 1100 }} />
      <ComponentModal form={form} open={editing !== null} title={editing?.id ? '编辑组件' : '新增组件'} onCancel={() => setEditing(null)} onFinish={saveComponent} />
      {subscribersFor && <SubscriberDrawer component={subscribersFor} onClose={() => setSubscribersFor(null)} />}
    </section>
  );
}

function Subscribers() {
  const [components, setComponents] = useState<ComponentItem[]>([]);
  const [componentId, setComponentId] = useState<number>();
  const selected = useMemo(() => components.find(item => item.id === componentId), [components, componentId]);

  useEffect(() => {
    api.components().then(data => {
      setComponents(data.items);
      setComponentId(data.items[0]?.id);
    }).catch(error => message.error(String(error)));
  }, []);

  return (
    <section>
      <PageHeader title="订阅人管理" description="维护组件版本更新的邮件订阅人。" />
      <Card className="toolbar-card">
        <Select
          showSearch
          className="component-select"
          placeholder="选择组件"
          value={componentId}
          optionFilterProp="label"
          onChange={setComponentId}
          options={components.map(item => ({ label: `${item.name} (${item.repo_url})`, value: item.id }))}
        />
      </Card>
      {selected && <SubscriberManager component={selected} />}
    </section>
  );
}

function SubscriberDrawer(props: { component: ComponentItem; onClose: () => void }) {
  return (
    <Drawer width="min(760px, 100vw)" title={`${props.component.name} 订阅人`} open onClose={props.onClose}>
      <SubscriberManager component={props.component} />
    </Drawer>
  );
}

function SubscriberManager({ component }: { component: ComponentItem }) {
  const [items, setItems] = useState<Subscriber[]>([]);
  const [editing, setEditing] = useState<Subscriber | null>(null);
  const [loading, setLoading] = useState(false);
  const [form] = Form.useForm();

  async function load() {
    setLoading(true);
    try {
      setItems(await api.subscribers(component.id));
    } catch (error) {
      message.error(String(error));
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void load();
  }, [component.id]);

  function openEditor(item?: Subscriber) {
    const value = item ?? { name: '', email: '', enabled: true };
    setEditing(value as Subscriber);
    form.setFieldsValue(value);
  }

  async function save(values: Partial<Subscriber>) {
    try {
      if (editing?.id) {
        await api.updateSubscriber(editing.id, values);
        message.success('订阅人已更新');
      } else {
        await api.createSubscriber(component.id, values);
        message.success('订阅人已创建');
      }
      setEditing(null);
      form.resetFields();
      await load();
    } catch (error) {
      message.error(String(error));
    }
  }

  async function remove(id: number) {
    try {
      await api.deleteSubscriber(id);
      message.success('订阅人已删除');
      await load();
    } catch (error) {
      message.error(String(error));
    }
  }

  async function toggle(row: Subscriber, enabled: boolean) {
    try {
      await api.updateSubscriber(row.id, { ...row, enabled });
      await load();
    } catch (error) {
      message.error(String(error));
    }
  }

  return (
    <>
      <div className="table-actions">
        <Button type="primary" onClick={() => openEditor()}>新增订阅人</Button>
      </div>
      <Table
        rowKey="id"
        loading={loading}
        dataSource={items}
        pagination={false}
        scroll={{ x: 720 }}
        columns={[
          { title: '名称', dataIndex: 'name' },
          { title: '邮箱', dataIndex: 'email' },
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
      <Modal title={editing?.id ? '编辑订阅人' : '新增订阅人'} open={editing !== null} onCancel={() => setEditing(null)} onOk={() => form.submit()} destroyOnHidden>
        <Form form={form} layout="vertical" onFinish={save} initialValues={{ enabled: true }}>
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
      </Modal>
    </>
  );
}

function Checks() {
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
      <Table
        rowKey="id"
        loading={loading}
        dataSource={items}
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
      <Drawer title="检查详情" width="min(720px, 100vw)" open={detail !== null} onClose={() => setDetail(null)}>
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

function Notifications() {
  const [items, setItems] = useState<NotificationRecord[]>([]);
  const [components, setComponents] = useState<ComponentItem[]>([]);
  const [mailStatus, setMailStatus] = useState<MailAuthStatus | null>(null);
  const [filters, setFilters] = useState<Record<string, string | number | boolean | undefined>>({});
  const [detail, setDetail] = useState<NotificationRecord | null>(null);
  const [testOpen, setTestOpen] = useState(false);
  const [testSending, setTestSending] = useState(false);
  const [loading, setLoading] = useState(false);
  const [testForm] = Form.useForm<{ recipient: string }>();

  async function load(nextFilters = filters) {
    setLoading(true);
    try {
      const [records, componentPage, status] = await Promise.all([
        api.notifications(nextFilters),
        api.components(),
        api.mailStatus(),
      ]);
      setItems(records.items);
      setComponents(componentPage.items);
      setMailStatus(status);
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
      {mailStatus && (
        <Alert
          className="mail-status"
          type={mailStatus.connected ? 'success' : 'warning'}
          showIcon
          message={mailStatus.connected ? '邮件 Token 已配置' : '邮件 Token 未配置'}
          description={mailStatus.message || '测试邮件和更新通知会使用 .env 中的 Outlook token 发送。'}
        />
      )}
      <FilterBar components={components} filters={filters} onChange={next => { setFilters(next); void load(next); }} />
      <Table
        rowKey="id"
        loading={loading}
        dataSource={items}
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
      <Drawer title="邮件正文" width="min(720px, 100vw)" open={detail !== null} onClose={() => setDetail(null)}>
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
    <Modal title={props.title} open={props.open} onCancel={props.onCancel} onOk={() => props.form.submit()} destroyOnHidden>
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
  function patch(key: string, value: string | number | boolean | undefined) {
    props.onChange({ ...props.filters, [key]: value });
  }

  return (
    <Card className="toolbar-card">
      <Space wrap>
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

function StatusTag({ status }: { status?: string }) {
  if (status === 'success' || status === 'sent') return <Tag color="green">{status}</Tag>;
  if (status === 'failed') return <Tag color="red">{status}</Tag>;
  if (status === 'running') return <Tag color="blue">{status}</Tag>;
  if (status === 'skipped') return <Tag>{status}</Tag>;
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
