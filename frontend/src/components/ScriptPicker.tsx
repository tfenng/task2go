import { useEffect, useState } from "react";
import { Button, Modal, Table, Tag, message } from "antd";
import { FolderOutlined, FileOutlined } from "@ant-design/icons";
import { listScripts } from "../api";
import type { ScriptEntry, ScriptList } from "../types";

export default function ScriptPicker({
  open,
  onCancel,
  onSelect,
}: {
  open: boolean;
  onCancel: () => void;
  onSelect: (absolutePath: string, entry: ScriptEntry) => void;
}) {
  const [path, setPath] = useState("");
  const [data, setData] = useState<ScriptList | null>(null);
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    if (!open) return;
    setLoading(true);
    listScripts(path)
      .then(setData)
      .catch((error) => message.error((error as Error).message))
      .finally(() => setLoading(false));
  }, [open, path]);

  function choose(entry: ScriptEntry) {
    if (entry.type === "dir") {
      setPath(entry.path);
      return;
    }
    const root = data?.root.replace(/\/$/, "") || "";
    onSelect(`${root}/${entry.path}`, entry);
  }

  return (
    <Modal title="选择本机脚本" open={open} onCancel={onCancel} footer={null} width={720}>
      <div style={{ marginBottom: 12, display: "flex", gap: 8, flexWrap: "wrap" }}>
        <Button size="small" onClick={() => setPath("")} disabled={!path}>根目录</Button>
        {data?.parent !== null && data?.parent !== undefined && (
          <Button size="small" onClick={() => setPath(data.parent || "")}>上一级</Button>
        )}
        <Tag>{`/${data?.path || ""}`}</Tag>
      </div>
      <Table<ScriptEntry>
        rowKey="path"
        size="small"
        loading={loading}
        dataSource={data?.items || []}
        pagination={false}
        onRow={(entry) => ({ onClick: () => choose(entry), style: { cursor: "pointer" } })}
        columns={[
          {
            title: "名称",
            dataIndex: "name",
            render: (value: string, entry: ScriptEntry) => (
              <span>{entry.type === "dir" ? <FolderOutlined /> : <FileOutlined />} {value}</span>
            ),
          },
          { title: "类型", dataIndex: "type", width: 100, render: (value: string) => value === "dir" ? "目录" : "文件" },
          { title: "大小", dataIndex: "size", width: 120, render: (value: number | null) => value == null ? "-" : `${value} B` },
        ]}
      />
    </Modal>
  );
}
