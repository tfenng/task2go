import { useEffect, useState } from "react";
import { Button, Card, Collapse, DatePicker, Form, Input, InputNumber, Modal, Popconfirm, Select, Space, Table, Tag, Typography, message } from "antd";
import { DeleteOutlined, PlusOutlined, ReloadOutlined } from "@ant-design/icons";
import dayjs, { Dayjs } from "dayjs";
import { createSchedule, deleteAllSchedules, deleteSchedule, listDefinitions, listSchedules, scheduleAction } from "../api";
import { parseShellArgs } from "../lib/args";
import type { Definition, Schedule, User } from "../types";

const TIMEZONES = [
  "UTC",
  "Asia/Shanghai",
  "Asia/Hong_Kong",
  "Asia/Tokyo",
  "Asia/Seoul",
  "Asia/Singapore",
  "Asia/Bangkok",
  "Asia/Kolkata",
  "Asia/Dubai",
  "Europe/London",
  "Europe/Paris",
  "Europe/Berlin",
  "Europe/Madrid",
  "Europe/Moscow",
  "Africa/Cairo",
  "Africa/Johannesburg",
  "America/New_York",
  "America/Chicago",
  "America/Denver",
  "America/Los_Angeles",
  "America/Mexico_City",
  "America/Sao_Paulo",
  "Australia/Perth",
  "Australia/Sydney",
  "Pacific/Auckland",
];

function zoneLabel(zone: string): string {
  if (zone === "UTC") return "UTC (UTC+00:00)";
  try {
    const parts = new Intl.DateTimeFormat("en-US", {
      timeZone: zone,
      timeZoneName: "shortOffset",
    }).formatToParts(new Date());
    const offset = parts.find((part) => part.type === "timeZoneName")?.value;
    return offset ? `${zone} (${offset})` : zone;
  } catch {
    return zone;
  }
}

const TIMEZONE_OPTIONS = TIMEZONES.map((zone) => ({ value: zone, label: zoneLabel(zone) }));

const TRIGGER_LABELS: Record<string, string> = {
  once: "一次性",
  cron: "Cron",
  interval: "固定间隔",
};

export default function SchedulesPage({ user }: { user: User }) {
  const [items, setItems] = useState<Schedule[]>([]);
  const [definitions, setDefinitions] = useState<Definition[]>([]);
  const [loading, setLoading] = useState(true);
  const [open, setOpen] = useState(false);
  const [saving, setSaving] = useState(false);
  const [selected, setSelected] = useState<string | undefined>();
  const [triggerType, setTriggerType] = useState<"interval" | "cron" | "once">("once");
  const [form] = Form.useForm();

  function refresh() {
    setLoading(true);
    Promise.all([listSchedules(), listDefinitions()])
      .then(([schedulePage, definitionPage]) => {
        setItems(schedulePage.items);
        setDefinitions(definitionPage.items.filter((item) => !item.is_archived));
      })
      .finally(() => setLoading(false));
  }

  useEffect(refresh, []);
  const definition = definitions.find((item) => item.id === selected);

  function openCreate() {
    form.resetFields();
    setSelected(undefined);
    setTriggerType("once");
    setOpen(true);
  }

  function selectDefinition(value: string) {
    setSelected(value);
    const target = definitions.find((item) => item.id === value);
    if (target) {
      form.setFieldValue("name", `${target.name}-${dayjs().format("YYMMDD-HHmmss")}`);
    }
  }

  async function submit() {
    const values = await form.validateFields();
    if (!definition?.active_version) {
      message.error("请选择任务定义");
      return;
    }
    setSaving(true);
    try {
      const trigger: Record<string, unknown> = { type: triggerType };
      if (triggerType === "interval") trigger.interval_seconds = values.interval_seconds;
      if (triggerType === "cron") trigger.cron_expr = values.cron_expr;
      if (triggerType === "once" && values.fire_at) trigger.fire_at = (values.fire_at as Dayjs).toISOString();
      await createSchedule({
        definition_id: definition.id,
        definition_version_id: definition.active_version.id,
        name: values.name,
        trigger,
        timezone: values.timezone || "UTC",
        arguments: parseShellArgs(values.arguments || ""),
        overlap_policy: values.overlap_policy || "skip",
        misfire_grace_seconds: values.misfire_grace_seconds ?? 3600,
      });
      message.success("调度已创建");
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

  async function action(schedule: Schedule, actionName: "pause" | "resume" | "cancel") {
    try {
      await scheduleAction(schedule.id, actionName, schedule.version);
      message.success("已更新调度");
      refresh();
    } catch (error) {
      message.error((error as Error).message);
    }
  }

  async function remove(schedule: Schedule) {
    try {
      await deleteSchedule(schedule.id);
      message.success("调度已删除");
      refresh();
    } catch (error) {
      message.error((error as Error).message);
    }
  }

  async function removeAll() {
    try {
      const result = await deleteAllSchedules();
      message.success(`已删除 ${result.deleted} 个调度`);
      refresh();
    } catch (error) {
      message.error((error as Error).message);
    }
  }

  return (
    <div>
      <div className="page-head">
        <Typography.Title level={3}>调度</Typography.Title>
        <Space>
          <Button icon={<ReloadOutlined />} onClick={refresh} loading={loading}>
            刷新
          </Button>
          {user.role === "admin" && (
            <Popconfirm
              title="删除全部调度？"
              description="删除后不可恢复。运行记录会保留，但不再关联调度；已排队或运行中的任务不会被取消。"
              okText="删除全部"
              cancelText="取消"
              okButtonProps={{ danger: true }}
              onConfirm={removeAll}
              disabled={items.length === 0}
            >
              <Button danger icon={<DeleteOutlined />} disabled={items.length === 0}>
                删除全部
              </Button>
            </Popconfirm>
          )}
          {user.role !== "viewer" && (
            <Button type="primary" icon={<PlusOutlined />} onClick={openCreate}>
              新建调度
            </Button>
          )}
        </Space>
      </div>
      <Card>
        <Table<Schedule>
          rowKey="id"
          loading={loading}
          dataSource={items}
          columns={[
            { title: "名称", dataIndex: "name" },
            {
              title: "触发",
              render: (_, item) => {
                const trigger = TRIGGER_LABELS[item.trigger_type] || item.trigger_type;
                const detail = item.cron_expr || (item.interval_seconds ? `${item.interval_seconds} 秒` : "");
                return detail ? `${trigger} / ${detail}` : trigger;
              },
            },
            { title: "时区", dataIndex: "timezone" },
            { title: "状态", dataIndex: "status", render: (value: string) => <Tag>{value}</Tag> },
            { title: "下一次触发", dataIndex: "next_fire_at", render: (value: string | null) => value ? dayjs(value).format("YYYY-MM-DD HH:mm:ss") : "-" },
            {
              title: "操作",
              render: (_, item) => (
                <Space>
                  {item.status === "active" && <Button size="small" onClick={() => action(item, "pause")}>暂停</Button>}
                  {item.status === "paused" && <Button size="small" onClick={() => action(item, "resume")}>恢复</Button>}
                  {["active", "paused"].includes(item.status) && <Button size="small" danger onClick={() => action(item, "cancel")}>取消</Button>}
                  {user.role !== "viewer" && (
                    <Popconfirm
                      title="删除这个调度？"
                      description="运行记录会保留，但不再关联此调度；已排队或运行中的任务不会被取消。"
                      okText="删除"
                      cancelText="取消"
                      okButtonProps={{ danger: true }}
                      onConfirm={() => remove(item)}
                    >
                      <Button size="small" danger type="link">删除</Button>
                    </Popconfirm>
                  )}
                </Space>
              ),
            },
          ]}
        />
      </Card>
      <Modal title="新建调度" open={open} onCancel={() => setOpen(false)} onOk={submit} confirmLoading={saving} width={680}>
        <Form
          form={form}
          layout="vertical"
          initialValues={{
            interval_seconds: 60,
            cron_expr: "*/5 * * * *",
            timezone: "UTC",
            overlap_policy: "skip",
            misfire_grace_seconds: 3600,
          }}
        >
          <Form.Item label="任务定义" required>
            <Select
              value={selected}
              onChange={selectDefinition}
              placeholder="选择要调度的任务定义"
              options={definitions.map((item) => ({ value: item.id, label: `${item.name} (${item.key})` }))}
            />
          </Form.Item>
          <Form.Item name="name" label="调度名称" rules={[{ required: true }]}>
            <Input placeholder="选择任务定义后自动生成，可修改" />
          </Form.Item>
          <Form.Item label="触发方式" extra={triggerType === "once" ? "一次性任务不选时间时，保存后立即执行。" : undefined}>
            <Select
              value={triggerType}
              onChange={(value) => setTriggerType(value as "interval" | "cron" | "once")}
              options={[
                { value: "once", label: "一次性" },
                { value: "interval", label: "固定间隔" },
                { value: "cron", label: "Cron" },
              ]}
            />
          </Form.Item>
          {triggerType === "interval" && (
            <Form.Item name="interval_seconds" label="间隔秒数" rules={[{ required: true }]}>
              <InputNumber min={10} style={{ width: "100%" }} />
            </Form.Item>
          )}
          {triggerType === "cron" && (
            <Form.Item name="cron_expr" label="Cron 表达式" rules={[{ required: true }]}>
              <Input />
            </Form.Item>
          )}
          {triggerType === "once" && (
            <Form.Item name="fire_at" label="触发时间（可选）" extra="留空表示保存后立即执行。">
              <DatePicker showTime style={{ width: "100%" }} />
            </Form.Item>
          )}
          {definition && (
            <Form.Item
              name="arguments"
              label="调用参数"
              extra="按 shell 格式输入，空格分隔；支持单引号和双引号。"
            >
              <Input placeholder="例如: alice 3 'hello world'" />
            </Form.Item>
          )}
          <Collapse
            ghost
            items={[
              {
                key: "advanced",
                label: "高级选项（时区 / 重叠策略 / 错过宽限）",
                forceRender: true,
                children: (
                  <>
                    <Form.Item
                      name="timezone"
                      label="时区"
                      extra="Cron 和一次性触发时间按此时区解释；默认 UTC。"
                    >
                      <Select showSearch options={TIMEZONE_OPTIONS} />
                    </Form.Item>
                    <Form.Item
                      name="overlap_policy"
                      label="重叠策略"
                      extra="上一次还没结束时，本次触发怎么处理：跳过 = 直接标记为跳过，不补跑；排队 = 等上一次结束后再执行。"
                    >
                      <Select
                        options={[
                          { value: "skip", label: "跳过（推荐）" },
                          { value: "queue", label: "排队" },
                        ]}
                      />
                    </Form.Item>
                    <Form.Item
                      name="misfire_grace_seconds"
                      label="错过宽限（秒）"
                      extra="系统停机或繁忙错过触发时间后，在宽限时间内会补跑一次；超过宽限则标记为错过，不再执行。3600 = 1 小时，0 = 不补跑。"
                    >
                      <InputNumber min={0} style={{ width: "100%" }} />
                    </Form.Item>
                  </>
                ),
              },
            ]}
          />
        </Form>
      </Modal>
    </div>
  );
}
