import { useEffect, useState } from "react";
import { Button, Card, Form, Input, Modal, Select, Space, Table, Tag, Typography, message } from "antd";
import { PlusOutlined } from "@ant-design/icons";
import { createTarget, deleteTarget, listCredentials, listTargets, updateTarget } from "../api";
import type { Credential, Target } from "../types";

const CONFIG_TEMPLATES: Record<string, string> = {
  ssh: JSON.stringify(
    { host: "192.168.1.10", port: 22, user: "ops", known_hosts: "/etc/task-control/known_hosts" },
    null,
    2,
  ),
};

export default function TargetsPage() {
  const [items, setItems] = useState<Target[]>([]);
  const [credentials, setCredentials] = useState<Credential[]>([]);
  const [loading, setLoading] = useState(true);
  const [open, setOpen] = useState(false);
  const [saving, setSaving] = useState(false);
  const [form] = Form.useForm();

  function refresh() {
    setLoading(true);
    Promise.all([listTargets(), listCredentials()])
      .then(([targetPage, credentialPage]) => {
        setItems(targetPage.items);
        setCredentials(credentialPage.items.filter((item) => item.is_active));
      })
      .finally(() => setLoading(false));
  }

  useEffect(refresh, []);

  async function submit() {
    const values = await form.validateFields();
    setSaving(true);
    try {
      await createTarget({
        name: values.name,
        type: values.type,
        config: JSON.parse(values.config),
        credential_id: values.credential_id || null,
      });
      message.success("执行目标已创建");
      setOpen(false);
      form.resetFields();
      refresh();
    } catch (error) {
      message.error((error as Error).message);
    } finally {
      setSaving(false);
    }
  }

  async function toggle(target: Target) {
    try {
      await updateTarget(target.id, { is_active: !target.is_active }, target.version);
      message.success("已更新目标");
      refresh();
    } catch (error) {
      message.error((error as Error).message);
    }
  }

  function remove(target: Target) {
    Modal.confirm({
      title: `删除执行目标 ${target.name}？`,
      content: "仅未被任务定义或活动运行引用时可删除。",
      okText: "删除",
      okButtonProps: { danger: true },
      onOk: async () => {
        try {
          await deleteTarget(target.id, target.version);
          message.success("已删除执行目标");
          refresh();
        } catch (error) {
          message.error((error as Error).message);
        }
      },
    });
  }

  return (
    <div>
      <div className="page-head">
        <Typography.Title level={3}>执行目标</Typography.Title>
        <Button type="primary" icon={<PlusOutlined />} onClick={() => setOpen(true)}>新建目标</Button>
      </div>
      <Card>
        <Table<Target>
          rowKey="id"
          loading={loading}
          dataSource={items}
          columns={[
            { title: "名称", dataIndex: "name" },
            { title: "类型", dataIndex: "type", render: (value: string) => <Tag>{value}</Tag> },
            { title: "配置", dataIndex: "config", render: (value: Record<string, unknown>) => <code>{JSON.stringify(value)}</code> },
            { title: "状态", dataIndex: "is_active", render: (value: boolean) => <Tag color={value ? "green" : "default"}>{value ? "启用" : "禁用"}</Tag> },
            {
              title: "操作",
              render: (_, item) => (
                <Space>
                  <Button size="small" danger={item.is_active} onClick={() => toggle(item)}>{item.is_active ? "禁用" : "启用"}</Button>
                  <Button size="small" danger onClick={() => remove(item)}>删除</Button>
                </Space>
              ),
            },
          ]}
        />
      </Card>
      <Modal title="新建执行目标" open={open} onCancel={() => setOpen(false)} onOk={submit} confirmLoading={saving} width={680}>
        <Form form={form} layout="vertical" initialValues={{ type: "ssh", config: CONFIG_TEMPLATES.ssh }}>
          <Form.Item name="name" label="名称" rules={[{ required: true }]}><Input /></Form.Item>
          <Form.Item name="type" label="类型">
            <Select
              options={[{ value: "ssh", label: "SSH" }]}
              onChange={(value) => form.setFieldsValue({ config: CONFIG_TEMPLATES[value] })}
            />
          </Form.Item>
          <Form.Item name="config" label="配置 JSON" rules={[{ required: true }]}><Input.TextArea rows={6} className="code-area" /></Form.Item>
          <Form.Item name="credential_id" label="凭据"><Select allowClear options={credentials.map((item) => ({ value: item.id, label: `${item.name} (${item.kind})` }))} /></Form.Item>
        </Form>
      </Modal>
    </div>
  );
}
