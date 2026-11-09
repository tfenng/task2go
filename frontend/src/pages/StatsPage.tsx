import { useEffect, useState } from "react";
import { Card, Col, Row, Statistic, Table, Typography, message } from "antd";
import { getRunStats, listDefinitions } from "../api";
import type { Definition, RunStats } from "../types";

export default function StatsPage() {
  const [stats, setStats] = useState<RunStats | null>(null);
  const [definitions, setDefinitions] = useState<Definition[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    Promise.all([getRunStats("definition"), listDefinitions()])
      .then(([runStats, definitionPage]) => {
        setStats(runStats);
        setDefinitions(definitionPage.items);
      })
      .catch((error) => message.error((error as Error).message))
      .finally(() => setLoading(false));
  }, []);

  const definitionName = (id: string) =>
    definitions.find((item) => item.id === id)?.name || id.slice(0, 8);

  return (
    <div>
      <Typography.Title level={3}>统计</Typography.Title>
      <Typography.Paragraph type="secondary">
        耗时指标按任务定义分别统计，不对不同任务做跨任务平均；全局卡片只展示数量和成功率。
      </Typography.Paragraph>
      <Row gutter={[16, 16]}>
        <Col xs={12} md={6}><Card loading={loading}><Statistic title="运行总数" value={stats?.total ?? 0} /></Card></Col>
        <Col xs={12} md={6}><Card loading={loading}><Statistic title="全局成功率" value={stats?.success_rate ?? 0} precision={1} suffix="%" /></Card></Col>
        <Col xs={12} md={6}><Card loading={loading}><Statistic title="失败" value={stats?.failed ?? 0} valueStyle={{ color: "#cf1322" }} /></Card></Col>
        <Col xs={12} md={6}><Card loading={loading}><Statistic title="重试中" value={stats?.retrying ?? 0} /></Card></Col>
      </Row>
      <Card title="按任务定义的运行统计" style={{ marginTop: 16 }}>
        <Table
          rowKey="key"
          loading={loading}
          dataSource={stats?.buckets || []}
          pagination={false}
          columns={[
            { title: "任务定义", dataIndex: "key", render: (value: string) => definitionName(value) },
            { title: "总数", dataIndex: "total" },
            { title: "成功", dataIndex: "succeeded" },
            { title: "失败", dataIndex: "failed" },
            { title: "成功率", dataIndex: "success_rate", render: (value: number) => `${value.toFixed(1)}%` },
            {
              title: "平均耗时",
              dataIndex: "avg_duration_ms",
              render: (value: number | null) => value == null ? "-" : `${value.toFixed(0)} ms`,
            },
            {
              title: "P95 耗时",
              dataIndex: "p95_duration_ms",
              render: (value: number | null) => value == null ? "-" : `${value.toFixed(0)} ms`,
            },
          ]}
        />
      </Card>
    </div>
  );
}
