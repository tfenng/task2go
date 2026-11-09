import { useEffect, useState } from "react";
import { Button, Card, Descriptions, Form, Input, InputNumber, Modal, Select, Space, Tabs, Tag, Typography, message } from "antd";
import { ArrowLeftOutlined, DeleteOutlined, EditOutlined } from "@ant-design/icons";
import { useNavigate, useParams } from "react-router-dom";
import { getDefinition, listTargets, updateDefinition } from "../api";
import DeleteDefinitionModal from "../components/DeleteDefinitionModal";
import ScriptPicker from "../components/ScriptPicker";
import type { Definition, Target, User } from "../types";

export default function DefinitionDetailPage({ user }: { user: User }) {
  const { id } = useParams();
  const navigate = useNavigate();
  const [definition, setDefinition] = useState<Definition | null>(null);
  const [targets, setTargets] = useState<Target[]>([]);
  const [loading, setLoading] = useState(true);
  const [open, setOpen] = useState(false);
  const [saving, setSaving] = useState(false);
  const [executorType, setExecutorType] = useState<"local" | "ssh">("local");
  const [scriptPath, setScriptPath] = useState("");
  const [interpreter, setInterpreter] = useState("");
  const [pickerOpen, setPickerOpen] = useState(false);
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [form] = Form.useForm();

  function refresh() {
    if (!id) return;
    setLoading(true);
    Promise.all([getDefinition(id), listTargets().catch(() => ({ items: [] }))])
      .then(([item, targetPage]) => {
        setDefinition(item);
        setTargets(targetPage.items.filter((target) => target.is_active));
        setExecutorType(item.active_version?.executor_type === "ssh" ? "ssh" : "local");
      })
      .catch((error) => message.error((error as Error).message))
      .finally(() => setLoading(false));
  }

  useEffect(refresh, [id]);

  function openEdit() {
    const version = definition?.active_version;
    if (!definition || !version) return;
    const argv = version.executor_config.argv || [];
    const script = argv.find((item) => /\.(py|sh|bash|js|rb|pl)$/i.test(item) || item.includes("/")) || "";
    const interpreterValue = argv[0] && argv[0] !== script ? argv[0] : "";
    setScriptPath(script);
    setInterpreter(interpreterValue);
    form.setFieldsValue({
      name: definition.name,
      description: definition.description,
      target_id: version.executor_config.target_id,
      command: version.executor_config.command,
      timeout_seconds: version.timeout_seconds,
      max_attempts: version.retry_policy.max_attempts,
      max_concurrency: version.max_concurrency,
    });
    setOpen(true);
  }

  async function submit() {
    if (!definition) return;
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
      await updateDefinition(
        definition.id,
        {
          name: values.name,
          description: values.description || null,
          version: {
            executor_type: executorType,
            executor_config: executorConfig,
            timeout_seconds: values.timeout_seconds,
            retry_policy: { max_attempts: values.max_attempts },
            max_concurrency: values.max_concurrency,
          },
        },
        definition.version,
      );
      message.success("已创建新版本");
      setOpen(false);
      refresh();
    } catch (error) {
      message.error((error as Error).message);
    } finally {
      setSaving(false);
    }
  }

  if (!definition) return <Card loading={loading} />;
  const version = definition.active_version;
  const config = version?.executor_config || {};
  const command = version?.executor_type === "local"
    ? (config.argv || []).join(" ")
    : config.command || "";

  const localPanel = (
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
          addonAfter={<Button type="link" onClick={() => setPickerOpen(true)}>选择</Button>}
        />
      </Form.Item>
    </div>
  );

  const sshPanel = (
    <>
      <Form.Item name="target_id" label="执行目标" rules={[{ required: true }]}>
        <Select options={targets.filter((target) => target.type === "ssh").map((target) => ({ value: target.id, label: target.name }))} />
      </Form.Item>
      <Form.Item name="command" label="远程命令" rules={[{ required: true }]}><Input /></Form.Item>
    </>
  );

  return (
    <div>
      <div className="page-head">
        <Space>
          <Button icon={<ArrowLeftOutlined />} onClick={() => navigate("/definitions")}>返回</Button>
          <Typography.Title level={3}>{definition.name}</Typography.Title>
        </Space>
        {user.role !== "viewer" && (
          <Space>
            <Button danger icon={<DeleteOutlined />} onClick={() => setDeleteOpen(true)}>删除</Button>
            <Button type="primary" icon={<EditOutlined />} onClick={openEdit}>编辑（生成新版本）</Button>
          </Space>
        )}
      </div>
      <Card loading={loading}>
        <Descriptions column={2} bordered size="small">
          <Descriptions.Item label="业务键">{definition.key}</Descriptions.Item>
          <Descriptions.Item label="当前版本">{version?.version_no ?? "-"}</Descriptions.Item>
          <Descriptions.Item label="执行入口">{version?.executor_type ?? "-"}</Descriptions.Item>
          <Descriptions.Item label="状态"><Tag color={definition.is_archived ? "default" : "green"}>{definition.is_archived ? "已归档" : "启用"}</Tag></Descriptions.Item>
          <Descriptions.Item label="超时">{version?.timeout_seconds ?? "-"} 秒</Descriptions.Item>
          <Descriptions.Item label="最大尝试">{version?.retry_policy.max_attempts ?? "-"}</Descriptions.Item>
          <Descriptions.Item label="并发上限">{version?.max_concurrency ?? "-"}</Descriptions.Item>
          <Descriptions.Item label="说明" span={2}>{definition.description || "-"}</Descriptions.Item>
          <Descriptions.Item label="调用命令" span={2}><pre>{command || "-"}</pre></Descriptions.Item>
        </Descriptions>
      </Card>
      <Modal title="编辑任务定义（生成新版本）" open={open} onCancel={() => setOpen(false)} onOk={submit} confirmLoading={saving} width={760}>
        <Form form={form} layout="vertical">
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
          setPickerOpen(false);
        }}
      />
      <DeleteDefinitionModal
        definition={deleteOpen ? definition : null}
        onCancel={() => setDeleteOpen(false)}
        onDeleted={() => {
          setDeleteOpen(false);
          navigate("/definitions");
        }}
      />
    </div>
  );
}
