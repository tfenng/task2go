import { useEffect, useState } from "react";
import { Button, Card, Form, Input, InputNumber, Modal, Select, Space, Switch, Table, Tabs, Tag, Typography, message } from "antd";
import { DeleteOutlined, PlusOutlined } from "@ant-design/icons";
import { Link } from "react-router-dom";
import { createDefinition, listDefinitions, listTargets } from "../api";
import DeleteDefinitionModal from "../components/DeleteDefinitionModal";
import ScriptPicker from "../components/ScriptPicker";
import { scriptBaseName } from "../lib/args";
import type { Definition, Target, User } from "../types";

export default function DefinitionsPage({ user }: { user: User }) {
  const [items, setItems] = useState<Definition[]>([]);
  const [loading, setLoading] = useState(true);
  const [open, setOpen] = useState(false);
  const [saving, setSaving] = useState(false);
  const [executorType, setExecutorType] = useState<"local" | "ssh">("local");
  const [targets, setTargets] = useState<Target[]>([]);
  const [scriptPath, setScriptPath] = useState("");
  const [interpreter, setInterpreter] = useState("");
  const [pickerOpen, setPickerOpen] = useState(false);
  const [showArchived, setShowArchived] = useState(false);
  const [deleteTarget, setDeleteTarget] = useState<Definition | null>(null);
  const [form] = Form.useForm();

  function refresh() {
    setLoading(true);
    Promise.all([listDefinitions(showArchived ? undefined : false), listTargets().catch(() => ({ items: [] }))])
      .then(([page, targetPage]) => {
        setItems(page.items);
        setTargets(targetPage.items.filter((item) => item.is_active));
      })
      .finally(() => setLoading(false));
  }

  useEffect(refresh, [showArchived]);

  function resetForm() {
    form.resetFields();
    setExecutorType("local");
    setScriptPath("");
    setInterpreter("");
  }

  async function submit() {
    const values = await form.validateFields();
    if (executorType === "local" && !scriptPath) {
      message.error("请先选择本机脚本");
      return;
    }
    setSaving(true);
    try {
      const executorConfig: Record<string, unknown> = {};
      if (executorType === "local") {
        executorConfig.argv = [interpreter, scriptPath].filter(Boolean);
      } else {
        executorConfig.target_id = values.target_id;
        executorConfig.command = values.command;
      }
      await createDefinition({
        key: values.key,
        name: values.name,
        description: values.description || null,
        version: {
          executor_type: executorType,
          executor_config: executorConfig,
          timeout_seconds: values.timeout_seconds,
          retry_policy: { max_attempts: values.max_attempts },
          max_concurrency: values.max_concurrency,
        },
      });
      message.success("任务定义已创建");
      setOpen(false);
      resetForm();
      refresh();
    } catch (error) {
      message.error((error as Error).message);
    } finally {
      setSaving(false);
    }
  }

  const localPanel = (
    <>
      <div className="form-row">
        <Form.Item label="解释器" style={{ flex: 1 }}>
          <Select
            value={interpreter}
            onChange={setInterpreter}
            options={[
              { value: "", label: "自动（脚本头部 / 默认 bash）" },
              { value: "python3" },
              { value: "bash" },
              { value: "sh" },
            ]}
          />
        </Form.Item>
        <Form.Item label="脚本文件" style={{ flex: 3 }}>
          <Input
            readOnly
            value={scriptPath}
            placeholder="点击右侧按钮选择脚本"
            addonAfter={<Button type="link" onClick={() => setPickerOpen(true)}>选择</Button>}
          />
        </Form.Item>
      </div>
    </>
  );

  const sshPanel = (
    <>
      <Form.Item name="target_id" label="执行目标" rules={[{ required: true }]}>
        <Select options={targets.filter((item) => item.type === "ssh").map((item) => ({ value: item.id, label: item.name }))} />
      </Form.Item>
      <Form.Item name="command" label="远程命令" rules={[{ required: true }]}><Input /></Form.Item>
    </>
  );

  return (
    <div>
      <div className="page-head">
        <Typography.Title level={3}>任务定义</Typography.Title>
        <Space>
          <Space size={4}>
            <Typography.Text type="secondary">显示已归档</Typography.Text>
            <Switch size="small" checked={showArchived} onChange={setShowArchived} />
          </Space>
          {user.role !== "viewer" && (
            <Button type="primary" icon={<PlusOutlined />} onClick={() => setOpen(true)}>
              新建定义
            </Button>
          )}
        </Space>
      </div>
      <Card>
        <Table<Definition>
          rowKey="id"
          loading={loading}
          dataSource={items}
          columns={[
            { title: "业务键", dataIndex: "key" },
            { title: "名称", dataIndex: "name", render: (value: string, item: Definition) => <Link to={`/definitions/${item.id}`}>{value}</Link> },
            { title: "版本", render: (_, item) => item.active_version?.version_no ?? "-" },
            { title: "执行器", render: (_, item) => item.active_version?.executor_type ?? "-" },
            { title: "状态", render: (_, item) => <Tag color={item.is_archived ? "default" : "green"}>{item.is_archived ? "已归档" : "启用"}</Tag> },
            { title: "更新时间", dataIndex: "updated_at" },
            ...(user.role !== "viewer"
              ? [
                  {
                    title: "操作",
                    render: (_: unknown, item: Definition) => (
                      <Button size="small" danger icon={<DeleteOutlined />} onClick={() => setDeleteTarget(item)}>
                        删除
                      </Button>
                    ),
                  },
                ]
              : []),
          ]}
        />
      </Card>
      <Modal title="新建任务定义" open={open} onCancel={() => setOpen(false)} onOk={submit} confirmLoading={saving} width={760}>
        <Form form={form} layout="vertical" initialValues={{ timeout_seconds: 21600, max_attempts: 1, max_concurrency: 1 }}>
          <Form.Item name="key" label="业务键" rules={[{ required: true, pattern: /^[a-z][a-z0-9_-]{1,99}$/, message: "小写字母开头，可含数字、_、-" }]}>
            <Input placeholder="demo-hello" />
          </Form.Item>
          <Form.Item name="name" label="任务名称" rules={[{ required: true }]}><Input /></Form.Item>
          <Form.Item name="description" label="说明"><Input.TextArea rows={2} /></Form.Item>
          <Tabs
            activeKey={executorType}
            onChange={(value) => setExecutorType(value as "local" | "ssh")}
            items={[
              { key: "local", label: "本地脚本", children: localPanel },
              { key: "ssh", label: "SSH 命令", children: sshPanel },
            ]}
          />
          <div className="form-row">
            <Form.Item name="timeout_seconds" label="超时（秒）" extra="默认 6 小时；长任务按实际耗时调大，最大 24 小时。">
              <InputNumber min={1} max={86400} style={{ width: "100%" }} />
            </Form.Item>
            <Form.Item name="max_attempts" label="最大尝试次数"><InputNumber min={1} max={10} /></Form.Item>
            <Form.Item name="max_concurrency" label="并发上限"><InputNumber min={1} max={100} /></Form.Item>
          </div>
        </Form>
      </Modal>
      <ScriptPicker
        open={pickerOpen}
        onCancel={() => setPickerOpen(false)}
        onSelect={(absolutePath) => {
          setScriptPath(absolutePath);
          setInterpreter("");
          form.setFieldValue("name", scriptBaseName(absolutePath));
          setPickerOpen(false);
        }}
      />
      <DeleteDefinitionModal
        definition={deleteTarget}
        onCancel={() => setDeleteTarget(null)}
        onDeleted={() => {
          setDeleteTarget(null);
          refresh();
        }}
      />
    </div>
  );
}
