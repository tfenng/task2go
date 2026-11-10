import { useEffect, useState } from "react";
import { Card, Col, Row, Statistic, Table, Tag, Tooltip, Typography } from "antd";
import { Link } from "react-router-dom";
import dayjs from "dayjs";
import { listDefinitions, listRuns } from "../api";
import { parameterPreview, parameterText } from "../lib/run";
import { STATUS_COLORS } from "../lib/status";
import type { Definition, Run } from "../types";

export default function DashboardPage() {
  const [runs, setRuns] = useState<Run[]>([]);
  const [definitions, setDefinitions] = useState<Definition[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    Promise.all([listRuns(), listDefinitions()])
      .then(([runPage, definitionPage]) => {
        setRuns(runPage.items);
        setDefinitions(definitionPage.items);
      })
      .finally(() => setLoading(false));
  }, []);

  const counts = {
    total: runs.length,
    running: runs.filter((item) => item.status === "running").length,
    queued: runs.filter((item) => item.status === "queued" || item.status === "retry_wait").length,
    failed: runs.filter((item) => item.status === "failed").length,
    succeeded: runs.filter((item) => item.status === "succeeded").length,
  };

  return (
    <div>
      <Typography.Title level={3}>仪表盘</Typography.Title>
      <Row gutter={[16, 16]}>
        <Col xs={12} md={6}><Card><Statistic title="运行总数" value={counts.total} /></Card></Col>
        <Col xs={12} md={6}><Card><Statistic title="执行中" value={counts.running} /></Card></Col>
        <Col xs={12} md={6}><Card><Statistic title="排队/重试" value={counts.queued} /></Card></Col>
        <Col xs={12} md={6}><Card><Statistic title="失败" value={counts.failed} valueStyle={{ color: "#cf1322" }} /></Card></Col>
      </Row>
      <Card style={{ marginTop: 16 }} title="最近运行">
        <Table<Run>
          rowKey="id"
          loading={loading}
          dataSource={runs.slice(0, 8)}
          pagination={false}
          columns={[
            { title: "运行", dataIndex: "id", render: (value: string) => <Link to={`/runs/${value}`}>{value.slice(0, 8)}</Link> },
            { title: "任务定义", dataIndex: "definition_id", render: (value: string) => definitions.find((item) => item.id === value)?.name || value.slice(0, 8) },
            {
              title: "参数",
              width: 240,
              render: (_, item) => {
                const full = parameterText(item);
                if (full === "-") return "-";
                const preview = parameterPreview(full);
                if (!preview.truncated) return <span>{preview.text}</span>;
                return (
                  <Tooltip
                    title={<div style={{ maxWidth: 420, whiteSpace: "pre-wrap", wordBreak: "break-all" }}>{full}</div>}
                  >
                    <span style={{ cursor: "help" }}>{preview.text}</span>
                  </Tooltip>
                );
              },
            },
            { title: "状态", dataIndex: "status", render: (value: string) => <Tag color={STATUS_COLORS[value]}>{value}</Tag> },
            { title: "触发", dataIndex: "trigger_type" },
            { title: "创建时间", dataIndex: "created_at", render: (value: string) => dayjs(value).format("YYYY-MM-DD HH:mm:ss") },
          ]}
        />
      </Card>
    </div>
  );
}
