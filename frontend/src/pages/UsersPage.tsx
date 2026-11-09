import { useEffect, useState } from "react";
import { Button, Card, Form, Input, Modal, Select, Space, Table, Tag, Typography, message } from "antd";
import { PlusOutlined } from "@ant-design/icons";
import { createUser, listUsers, resetUserPassword, updateUser } from "../api";
import type { User } from "../types";

export default function UsersPage() {
  const [items, setItems] = useState<User[]>([]);
  const [loading, setLoading] = useState(true);
  const [open, setOpen] = useState(false);
  const [saving, setSaving] = useState(false);
  const [form] = Form.useForm();

  function refresh() {
    setLoading(true);
    listUsers()
      .then((page) => setItems(page.items))
      .finally(() => setLoading(false));
  }

  useEffect(refresh, []);

  async function submit() {
    const values = await form.validateFields();
    setSaving(true);
    try {
      await createUser(values);
      message.success("用户已创建");
      setOpen(false);
      form.resetFields();
      refresh();
    } catch (error) {
      message.error((error as Error).message);
    } finally {
      setSaving(false);
    }
  }

  async function update(user: User, payload: Record<string, unknown>) {
    try {
      await updateUser(user.id, payload, user.version);
      message.success("用户已更新");
      refresh();
    } catch (error) {
      message.error((error as Error).message);
    }
  }

  async function reset(user: User) {
    try {
      const result = await resetUserPassword(user.id);
      Modal.info({
        title: "临时密码",
        content: <Typography.Text copyable>{result.temporary_password}</Typography.Text>,
      });
    } catch (error) {
      message.error((error as Error).message);
    }
  }

  return (
    <div>
      <div className="page-head">
        <Typography.Title level={3}>用户</Typography.Title>
        <Button type="primary" icon={<PlusOutlined />} onClick={() => setOpen(true)}>新建用户</Button>
      </div>
      <Card>
        <Table<User>
          rowKey="id"
          loading={loading}
          dataSource={items}
          columns={[
            { title: "用户名", dataIndex: "username" },
            { title: "显示名", dataIndex: "display_name" },
            { title: "角色", dataIndex: "role", render: (value: string) => <Tag>{value}</Tag> },
            { title: "状态", dataIndex: "is_active", render: (value: boolean) => <Tag color={value ? "green" : "default"}>{value ? "启用" : "禁用"}</Tag> },
            {
              title: "角色调整",
              render: (_, item) => (
                <Select
                  size="small"
                  value={item.role}
                  style={{ width: 110 }}
                  onChange={(role) => update(item, { role })}
                  options={[{ value: "admin" }, { value: "operator" }, { value: "viewer" }]}
                />
              ),
            },
            {
              title: "操作",
              render: (_, item) => (
                <Space>
                  <Button size="small" onClick={() => reset(item)}>重置密码</Button>
                  <Button size="small" danger={item.is_active} onClick={() => update(item, { is_active: !item.is_active })}>
                    {item.is_active ? "禁用" : "启用"}
                  </Button>
                </Space>
              ),
            },
          ]}
        />
      </Card>
      <Modal title="新建用户" open={open} onCancel={() => setOpen(false)} onOk={submit} confirmLoading={saving}>
        <Form form={form} layout="vertical" initialValues={{ role: "operator" }}>
          <Form.Item name="username" label="用户名" rules={[{ required: true }]}><Input /></Form.Item>
          <Form.Item name="display_name" label="显示名" rules={[{ required: true }]}><Input /></Form.Item>
          <Form.Item name="role" label="角色"><Select options={[{ value: "admin" }, { value: "operator" }, { value: "viewer" }]} /></Form.Item>
          <Form.Item name="password" label="初始密码" rules={[{ required: true, min: 12 }]}><Input.Password autoComplete="new-password" /></Form.Item>
        </Form>
      </Modal>
    </div>
  );
}
