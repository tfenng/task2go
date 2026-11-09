import { useState } from "react";
import { Button, Card, Checkbox, Form, Input, Typography, message } from "antd";
import { login } from "../api";
import type { User } from "../types";

export default function LoginPage({ onLogin }: { onLogin: (user: User) => void }) {
  const [loading, setLoading] = useState(false);

  async function submit(values: { username: string; password: string; remember?: boolean }) {
    setLoading(true);
    try {
      const result = await login(values.username, values.password, values.remember ?? false);
      onLogin(result.user);
    } catch (error) {
      message.error((error as Error).message);
    } finally {
      setLoading(false);
    }
  }

  return (
    <div className="login-wrap">
      <Card className="login-card">
        <Typography.Title level={3}>任务中控台</Typography.Title>
        <Typography.Paragraph type="secondary">本地账号登录</Typography.Paragraph>
        <Form layout="vertical" onFinish={submit}>
          <Form.Item name="username" label="用户名" rules={[{ required: true }]}>
            <Input autoComplete="username" />
          </Form.Item>
          <Form.Item name="password" label="密码" rules={[{ required: true }]}>
            <Input.Password autoComplete="current-password" />
          </Form.Item>
          <Form.Item name="remember" valuePropName="checked" initialValue={false} style={{ marginBottom: 16 }}>
            <Checkbox>记住我（一周内免登录）</Checkbox>
          </Form.Item>
          <Button type="primary" htmlType="submit" block loading={loading}>
            登录
          </Button>
        </Form>
      </Card>
    </div>
  );
}
