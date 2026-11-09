import { useEffect, useState } from "react";
import { Button, Card, Form, Input, Modal, Select, Space, Table, Tag, Typography, message } from "antd";
import { PlusOutlined, ReloadOutlined } from "@ant-design/icons";
import { Link } from "react-router-dom";
import dayjs from "dayjs";
import { createRun, listDefinitions, listRuns } from "../api";
import { parseShellArgs } from "../lib/args";
import type { Definition, Run, User } from "../types";

export default function RunsPage({ user }: { user: User }) {
  const [runs, setRuns] = useState<Run[]>([]);
  const [definitions, setDefinitions] = useState<Definition[]>([]);
  const [loading, setLoading] = useState(true);
  const [open, setOpen] = useState(false);
  const [saving, setSaving] = useState(false);
  const [selected, setSelected] = useState<string | undefined>();
  const [form] = Form.useForm();

  function refresh() {
    setLoading(true);
    Promise.all([listRuns(), listDefinitions()])
      .then(([runPage, definitionPage]) => {
        setRuns(runPage.items);
        setDefinitions(definitionPage.items.filter((item) => !item.is_archived));
      })
      .finally(() => setLoading(false));
  }

  useEffect(refresh, []);
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
          <Button icon={<ReloadOutlined />} onClick={refresh} loading={loading}>
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
            { title: "状态", dataIndex: "status", render: (value: string) => <Tag>{value}</Tag> },
            { title: "触发", render: (_, item) => item.schedule_id ? `调度/${item.trigger_type}` : item.trigger_type },
            { title: "尝试", render: (_, item) => `${item.attempt_count}/${item.max_attempts}` },
            { title: "创建时间", dataIndex: "created_at", render: (value: string) => dayjs(value).format("YYYY-MM-DD HH:mm:ss") },
            { title: "错误", dataIndex: "error_code", render: (value: string | null) => value || "-" },
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
        </Form>
      </Modal>
    </div>
  );
}
