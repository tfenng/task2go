import { useEffect, useState } from "react";
import { Button, Card, Descriptions, Modal, Table, Tag, Typography } from "antd";
import dayjs from "dayjs";
import { listAuditEvents, listUsers } from "../api";
import type { AuditEvent, User } from "../types";

const ACTION_LABELS: Record<string, string> = {
  "auth.login": "登录",
  "auth.logout": "退出",
  "auth.password_change": "修改密码",
  "auth.session_revoke": "吊销会话",
  "auth.token_create": "创建 API Token",
  "auth.token_revoke": "吊销 API Token",
  "user.create": "创建用户",
  "user.update": "更新用户",
  "user.reset_password": "重置用户密码",
  "definition.create": "创建任务",
  "definition.update": "更新任务",
  "definition.enable": "启用任务",
  "definition.disable": "禁用任务",
  "definition.archive": "归档任务",
  "definition.delete": "删除任务",
  "run.create": "提交运行",
  "run.retry": "重试运行",
  "run.cancel": "取消运行",
  "run.succeeded": "运行成功",
  "run.failed": "运行失败",
  "schedule.create": "创建调度",
  "schedule.update": "更新调度",
  "schedule.pause": "暂停调度",
  "schedule.resume": "恢复调度",
  "schedule.cancel": "取消调度",
  "target.create": "创建执行目标",
  "target.update": "更新执行目标",
  "target.delete": "删除执行目标",
  "credential.create": "创建凭据",
  "credential.update": "更新凭据",
  "credential.delete": "删除凭据",
};

const RESOURCE_LABELS: Record<string, string> = {
  user: "用户",
  task_definition: "任务",
  task_run: "运行",
  schedule: "调度",
  execution_target: "执行目标",
  credential: "凭据",
  session: "会话",
  api_token: "API Token",
};

export default function AuditPage() {
  const [items, setItems] = useState<AuditEvent[]>([]);
  const [users, setUsers] = useState<User[]>([]);
  const [loading, setLoading] = useState(true);
  const [detail, setDetail] = useState<AuditEvent | null>(null);

  useEffect(() => {
    Promise.all([listAuditEvents(), listUsers()])
      .then(([eventPage, userPage]) => {
        setItems(eventPage.items);
        setUsers(userPage.items);
      })
      .finally(() => setLoading(false));
  }, []);

  const username = (id: string | null | undefined) => users.find((item) => item.id === id)?.username;
  const resourceLabel = (event: AuditEvent) => {
    const label = RESOURCE_LABELS[event.resource_type] || event.resource_type;
    if (event.resource_type === "user") {
      return `${label}：${username(event.resource_id) || event.resource_id?.slice(0, 8) || "-"}`;
    }
    return `${label}：${event.resource_id?.slice(0, 8) || "-"}`;
  };

  return (
    <div>
      <Typography.Title level={3}>审计</Typography.Title>
      <Card>
        <Table<AuditEvent>
          rowKey="id"
          loading={loading}
          dataSource={items}
          columns={[
            { title: "时间", dataIndex: "created_at", render: (value: string) => dayjs(value).format("YYYY-MM-DD HH:mm:ss") },
            { title: "操作者", render: (_, item) => item.actor_username || username(item.actor_user_id) || "系统" },
            { title: "操作", dataIndex: "action", render: (value: string) => ACTION_LABELS[value] || value },
            { title: "对象", render: (_, item) => resourceLabel(item) },
            { title: "结果", dataIndex: "outcome", render: (value: string) => <Tag color={value === "success" ? "green" : value === "denied" ? "orange" : "red"}>{value === "success" ? "成功" : value === "denied" ? "拒绝" : "失败"}</Tag> },
            { title: "", render: (_, item) => <Button size="small" onClick={() => setDetail(item)}>详情</Button> },
          ]}
        />
      </Card>
      <Modal title="审计详情" open={detail !== null} onCancel={() => setDetail(null)} footer={null} width={640}>
        {detail && (
          <>
            <Typography.Paragraph type="secondary">
              追踪 ID 用于在服务端日志中检索本次操作；异步任务可同时使用运行 ID 检索 worker/scheduler 日志。
            </Typography.Paragraph>
            <Descriptions column={1} bordered size="small">
            <Descriptions.Item label="时间">{dayjs(detail.created_at).format("YYYY-MM-DD HH:mm:ss")}</Descriptions.Item>
            <Descriptions.Item label="操作者">{detail.actor_username || username(detail.actor_user_id) || "系统"}</Descriptions.Item>
            <Descriptions.Item label="操作">{ACTION_LABELS[detail.action] || detail.action}</Descriptions.Item>
            <Descriptions.Item label="对象">{resourceLabel(detail)}</Descriptions.Item>
            <Descriptions.Item label="结果">{detail.outcome}</Descriptions.Item>
            <Descriptions.Item label="追踪 ID"><Typography.Text copyable>{detail.correlation_id}</Typography.Text></Descriptions.Item>
            <Descriptions.Item label="资源 ID"><Typography.Text copyable>{detail.resource_id || "-"}</Typography.Text></Descriptions.Item>
            <Descriptions.Item label="详情"><pre>{JSON.stringify(detail.details, null, 2)}</pre></Descriptions.Item>
            </Descriptions>
          </>
        )}
      </Modal>
    </div>
  );
}
