import { useEffect, useState } from "react";
import { Button, Card, Form, Input, Modal, Select, Space, Table, Tag, Typography, message } from "antd";
import { PlusOutlined } from "@ant-design/icons";
import { createCredential, deleteCredential, listCredentials, updateCredential } from "../api";
import type { Credential } from "../types";

export default function CredentialsPage() {
  const [items, setItems] = useState<Credential[]>([]);
  const [loading, setLoading] = useState(true);
  const [open, setOpen] = useState(false);
  const [saving, setSaving] = useState(false);
  const [form] = Form.useForm();

  function refresh() {
    setLoading(true);
    listCredentials()
      .then((page) => setItems(page.items))
      .finally(() => setLoading(false));
  }

  useEffect(refresh, []);

  async function submit() {
    const values = await form.validateFields();
    setSaving(true);
    try {
      await createCredential({
        name: values.name,
        kind: values.kind,
        secret: values.secret,
        metadata: values.metadata ? JSON.parse(values.metadata) : {},
      });
      message.success("凭据已创建");
      setOpen(false);
      form.resetFields();
      refresh();
    } catch (error) {
      message.error((error as Error).message);
    } finally {
      setSaving(false);
    }
  }

  async function toggle(credential: Credential) {
    try {
      await updateCredential(credential.id, { is_active: !credential.is_active }, credential.version);
      message.success("已更新凭据");
      refresh();
    } catch (error) {
      message.error((error as Error).message);
    }
  }

  function remove(credential: Credential) {
    Modal.confirm({
      title: `删除凭据 ${credential.name}？`,
      content: "仅未被执行目标或活动运行引用时可删除。",
      okText: "删除",
      okButtonProps: { danger: true },
      onOk: async () => {
        try {
          await deleteCredential(credential.id, credential.version);
          message.success("已删除凭据");
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
        <Typography.Title level={3}>凭据</Typography.Title>
        <Button type="primary" icon={<PlusOutlined />} onClick={() => setOpen(true)}>新建凭据</Button>
      </div>
      <Card>
        <Table<Credential>
          rowKey="id"
          loading={loading}
          dataSource={items}
          columns={[
            { title: "名称", dataIndex: "name" },
            { title: "类型", dataIndex: "kind", render: (value: string) => <Tag>{value}</Tag> },
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
      <Modal title="新建凭据" open={open} onCancel={() => setOpen(false)} onOk={submit} confirmLoading={saving} width={620}>
        <Form form={form} layout="vertical" initialValues={{ kind: "http_bearer" }}>
          <Form.Item name="name" label="名称" rules={[{ required: true }]}><Input /></Form.Item>
          <Form.Item name="kind" label="类型">
            <Select options={[
              { value: "http_bearer", label: "HTTP Bearer Token" },
              { value: "http_basic", label: "HTTP Basic" },
              { value: "ssh_key", label: "SSH 私钥" },
              { value: "password", label: "密码" },
            ]} />
          </Form.Item>
          <Form.Item name="secret" label="密钥值（只写，不回显）" rules={[{ required: true }]}>
            <Input.Password autoComplete="new-password" />
          </Form.Item>
          <Form.Item name="metadata" label="元数据 JSON（如 username/passphrase）"><Input.TextArea rows={3} className="code-area" /></Form.Item>
        </Form>
      </Modal>
    </div>
  );
}
