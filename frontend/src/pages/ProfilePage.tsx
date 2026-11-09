import { useEffect, useState } from "react";
import { Button, Card, Form, Input, Space, Table, Tag, Typography, message } from "antd";
import dayjs from "dayjs";
import {
  changePassword,
  createApiToken,
  listApiTokens,
  listSessions,
  revokeApiToken,
  revokeSession,
} from "../api";
import type { ApiTokenInfo, SessionInfo } from "../types";

export default function ProfilePage() {
  const [sessions, setSessions] = useState<SessionInfo[]>([]);
  const [tokens, setTokens] = useState<ApiTokenInfo[]>([]);
  const [newToken, setNewToken] = useState<string | null>(null);
  const [form] = Form.useForm();

  function refresh() {
    listSessions().then((result) => setSessions(result.items));
    listApiTokens().then((result) => setTokens(result.items));
  }

  useEffect(refresh, []);

  async function submitPassword() {
    const values = await form.validateFields();
    try {
      await changePassword(values.current_password, values.new_password);
      message.success("密码已修改");
      form.resetFields();
      refresh();
    } catch (error) {
      message.error((error as Error).message);
    }
  }

  async function createToken() {
    const name = `token-${Date.now()}`;
    try {
      const result = await createApiToken(name);
      setNewToken(result.token);
      refresh();
    } catch (error) {
      message.error((error as Error).message);
    }
  }

  return (
    <div>
      <Typography.Title level={3}>个人设置</Typography.Title>
      <Card title="修改密码" style={{ marginBottom: 16 }}>
        <Form form={form} layout="vertical" style={{ maxWidth: 420 }}>
          <Form.Item name="current_password" label="当前密码" rules={[{ required: true }]}>
            <Input.Password autoComplete="current-password" />
          </Form.Item>
          <Form.Item name="new_password" label="新密码" rules={[{ required: true, min: 12 }]}>
            <Input.Password autoComplete="new-password" />
          </Form.Item>
          <Button type="primary" onClick={submitPassword}>保存</Button>
        </Form>
      </Card>
      <Card title="会话" style={{ marginBottom: 16 }}>
        <Table<SessionInfo>
          rowKey="id"
          dataSource={sessions}
          pagination={false}
          columns={[
            { title: "登录时间", dataIndex: "created_at", render: (value: string) => dayjs(value).format("YYYY-MM-DD HH:mm:ss") },
            { title: "最近活动", dataIndex: "last_seen_at", render: (value: string) => dayjs(value).format("YYYY-MM-DD HH:mm:ss") },
            { title: "过期", dataIndex: "expires_at", render: (value: string) => dayjs(value).format("YYYY-MM-DD HH:mm:ss") },
            { title: "IP", dataIndex: "ip_address", render: (value: string | null) => value || "-" },
            { title: "操作", render: (_, item) => <Button size="small" danger onClick={() => revokeSession(item.id).then(refresh)}>吊销</Button> },
          ]}
        />
      </Card>
      <Card title="API Token" extra={<Button onClick={createToken}>创建 Token</Button>}>
        {newToken && (
          <Typography.Paragraph copyable={{ text: newToken }}>
            <Tag color="green">仅显示一次</Tag> {newToken}
          </Typography.Paragraph>
        )}
        <Table<ApiTokenInfo>
          rowKey="id"
          dataSource={tokens}
          pagination={false}
          columns={[
            { title: "名称", dataIndex: "name" },
            { title: "前缀", dataIndex: "token_prefix" },
            { title: "最近使用", dataIndex: "last_used_at", render: (value: string | null) => value ? dayjs(value).format("YYYY-MM-DD HH:mm:ss") : "-" },
            { title: "状态", render: (_, item) => <Tag>{item.revoked_at ? "已吊销" : "有效"}</Tag> },
            { title: "操作", render: (_, item) => !item.revoked_at && <Button size="small" danger onClick={() => revokeApiToken(item.id).then(refresh)}>吊销</Button> },
          ]}
        />
      </Card>
    </div>
  );
}
