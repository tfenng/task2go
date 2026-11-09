import { useEffect, useState } from "react";
import { Checkbox, Modal, Typography, message } from "antd";
import { deleteDefinition } from "../api";
import type { Definition } from "../types";

export default function DeleteDefinitionModal({
  definition,
  onCancel,
  onDeleted,
}: {
  definition: Definition | null;
  onCancel: () => void;
  onDeleted: () => void;
}) {
  const [deleteRuns, setDeleteRuns] = useState(false);
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    if (definition) setDeleteRuns(false);
  }, [definition]);

  async function submit() {
    if (!definition) return;
    setSaving(true);
    try {
      const result = await deleteDefinition(definition.id, deleteRuns);
      message.success(
        result.deleted_runs > 0
          ? `任务已删除，同时删除 ${result.deleted_runs} 条运行记录`
          : "任务已删除",
      );
      onDeleted();
    } catch (error) {
      message.error((error as Error).message);
    } finally {
      setSaving(false);
    }
  }

  return (
    <Modal
      title={definition ? `删除任务：${definition.name}` : "删除任务"}
      open={definition !== null}
      onCancel={onCancel}
      onOk={submit}
      okText="删除"
      okButtonProps={{ danger: true }}
      cancelText="取消"
      confirmLoading={saving}
    >
      <Typography.Paragraph type="danger">
        删除后任务会从列表隐藏，且不能恢复。
      </Typography.Paragraph>
      <Typography.Paragraph>
        关联的调度会被取消。正在运行的任务不会被删除，请先在运行页取消。
      </Typography.Paragraph>
      <Checkbox checked={deleteRuns} onChange={(event) => setDeleteRuns(event.target.checked)}>
        同时删除已结束的运行记录（不可恢复）
      </Checkbox>
    </Modal>
  );
}
