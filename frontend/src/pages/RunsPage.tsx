import { useEffect, useState } from "react";
import { Button, Card, Form, Input, Modal, Popconfirm, Select, Space, Table, Tag, Tooltip, Typography, message } from "antd";
import { PlusOutlined, ReloadOutlined } from "@ant-design/icons";
import { Link } from "react-router-dom";
import dayjs from "dayjs";
import { createRun, deleteRun, listDefinitions, listRuns } from "../api";
import { parseShellArgs } from "../lib/args";
import { parameterPreview, parameterText } from "../lib/run";
import { STATUS_COLORS, TERMINAL_STATUSES } from "../lib/status";
import type { Definition, Run, User } from "../types";

const ACTIVE_STATUSES = ["scheduled", "queued", "running", "retry_wait"];

function formatDuration(ms: number): string {
  if (ms < 1000) return `${ms}ms`;
  const seconds = Math.floor(ms / 1000);
  if (seconds < 60) return `${seconds}s`;
  const minutes = Math.floor(seconds / 60);
  const restSeconds = seconds % 60;
  if (minutes < 60) return restSeconds ? `${minutes}m ${restSeconds}s` : `${minutes}m`;
  const hours = Math.floor(minutes / 60);
  const restMinutes = minutes % 60;
  return restMinutes ? `${hours}h ${restMinutes}m` : `${hours}h`;
}

export default function RunsPage({ user }: { user: User }) {
  const [runs, setRuns] = useState<Run[]>([]);
  const [definitions, setDefinitions] = useState<Definition[]>([]);
  const [loading, setLoading] = useState(true);
  const [open, setOpen] = useState(false);
  const [saving, setSaving] = useState(false);
  const [selected, setSelected] = useState<string | undefined>();
  const [now, setNow] = useState(() => Date.now());
  const [form] = Form.useForm();

  function refresh(showLoading = true) {
    if (showLoading) setLoading(true);
    Promise.all([listRuns(), listDefinitions()])
      .then(([runPage, definitionPage]) => {
        setRuns(runPage.items);
        setDefinitions(definitionPage.items.filter((item) => !item.is_archived));
      })
      .finally(() => {
        if (showLoading) setLoading(false);
      });
  }

  useEffect(() => {
    refresh();
  }, []);

  const hasActiveRun = runs.some((item) => ACTIVE_STATUSES.includes(item.status));

  useEffect(() => {
    if (!hasActiveRun) return;
    const timer = setInterval(() => refresh(false), 2000);
    return () => clearInterval(timer);
  }, [hasActiveRun]);

  useEffect(() => {
    const timer = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(timer);
  }, []);

  function runDuration(item: Run): string {
    if (!item.started_at) return "-";
    const end = item.finished_at ? dayjs(item.finished_at).valueOf() : now;
    return formatDuration(Math.max(0, end - dayjs(item.started_at).valueOf()));
  }

  async function remove(id: string) {
    try {
      await deleteRun(id);
      message.success("运行已删除");
      refresh();
    } catch (error) {
      message.error((error as Error).message);
    }
  }
  const definition = definitions.find((item) => item.id === selected);
  const definitionName = (id: string) => definitions.find((item) => item.id === id)?.name || id.slice(0, 8);

  async function submit() {
    const values = await form.validateFields();
    if (!definition?.active_version) return;
    setSaving(true);
    try {
      await createRun({
        definition_id: definition.id,
        definition_version_id: definition.active_version.id,
        arguments: parseShellArgs(values.arguments || ""),
        timeout_seconds: values.timeout_seconds || undefined,
        trigger: { type: "immediate" },
      });
      message.success("运行已提交");
      setOpen(false);
      form.resetFields();
      setSelected(undefined);
      refresh();
    } catch (error) {
      message.error((error as Error).message);
    } finally {
      setSaving(false);
    }
  }

  return (
    <div>
      <div className="page-head">
        <Typography.Title level={3}>运行</Typography.Title>
        <Space>
          <Button icon={<ReloadOutlined />} onClick={() => refresh()} loading={loading}>
            刷新
          </Button>
          {user.role !== "viewer" && (
            <Button type="primary" icon={<PlusOutlined />} onClick={() => setOpen(true)}>
              提交运行
            </Button>
          )}
        </Space>
      </div>
      <Card>
        <Table<Run>
          rowKey="id"
          loading={loading}
          dataSource={runs}
          columns={[
            { title: "运行", dataIndex: "id", render: (value: string) => <Link to={`/runs/${value}`}>{value.slice(0, 8)}</Link> },
            { title: "任务定义", dataIndex: "definition_id", render: (value: string) => definitionName(value) },
            {
              title: "参数",
              width: 240,
              render: (_, item) => {
                const full = parameterText(item);
                if (full === "-") return "-";
                const preview = parameterPreview(full);
                if (!preview.truncated) return <span>{preview.text}</span>;
                return (
                  <Tooltip
                    title={<div style={{ maxWidth: 420, whiteSpace: "pre-wrap", wordBreak: "break-all" }}>{full}</div>}
                  >
                    <span style={{ cursor: "help" }}>{preview.text}</span>
                  </Tooltip>
                );
              },
            },
            { title: "状态", dataIndex: "status", render: (value: string) => <Tag color={STATUS_COLORS[value]}>{value}</Tag> },
            { title: "总用时", render: (_, item) => runDuration(item) },
            { title: "触发", render: (_, item) => item.schedule_id ? `调度/${item.trigger_type}` : item.trigger_type },
            { title: "尝试", render: (_, item) => `${item.attempt_count}/${item.max_attempts}` },
            { title: "创建时间", dataIndex: "created_at", render: (value: string) => dayjs(value).format("YYYY-MM-DD HH:mm:ss") },
            { title: "错误", dataIndex: "error_code", render: (value: string | null) => value || "-" },
            {
              title: "操作",
              width: 80,
              render: (_, item) => TERMINAL_STATUSES.includes(item.status) && user.role !== "viewer" ? (
                <Popconfirm
                  title="删除这次运行及执行记录？"
                  okText="删除"
                  cancelText="取消"
                  onConfirm={() => remove(item.id)}
                >
                  <Button type="link" size="small" danger>删除</Button>
                </Popconfirm>
              ) : null,
            },
          ]}
        />
      </Card>
      <Modal title="提交运行" open={open} onCancel={() => setOpen(false)} onOk={submit} confirmLoading={saving} width={680}>
        <Form form={form} layout="vertical">
          <Form.Item label="任务定义" required>
            <Select
              value={selected}
              onChange={setSelected}
              options={definitions.map((item) => ({ value: item.id, label: `${item.name} (${item.key})` }))}
            />
          </Form.Item>
          {definition && (
            <Form.Item
              name="arguments"
              label="调用参数"
              extra="按 shell 格式输入，空格分隔；支持单引号和双引号。参数会逐个追加到命令末尾。"
            >
              <Input placeholder="例如: alice 3 'hello world'" />
            </Form.Item>
          )}
          {definition && (
            <Form.Item
              name="timeout_seconds"
              label="本次超时"
              extra={`默认跟随定义：${definition.active_version?.timeout_seconds ?? 21600} 秒；仅对本次运行生效。`}
            >
              <Select
                allowClear
                placeholder="跟随定义默认值"
                options={[
                  { value: 60, label: "1 分钟" },
                  { value: 300, label: "5 分钟" },
                  { value: 1800, label: "30 分钟" },
                  { value: 3600, label: "1 小时" },
                  { value: 21600, label: "6 小时" },
                  { value: 86400, label: "24 小时" },
                ]}
              />
            </Form.Item>
          )}
        </Form>
      </Modal>
    </div>
  );
}
