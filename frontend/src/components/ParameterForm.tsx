import { Form, Input, InputNumber, Switch } from "antd";
import type { ParameterSchema } from "../types";

export default function ParameterForm({ schema }: { schema?: ParameterSchema | null }) {
  const properties = schema?.properties || {};
  const required = new Set(schema?.required || []);
  if (!Object.keys(properties).length) {
    return <Form.Item name="parametersJson" label="参数（JSON）"><Input.TextArea rows={4} placeholder='{"name": "alice"}' /></Form.Item>;
  }
  return (
    <>
      {Object.entries(properties).map(([name, definition]) => {
        const type = definition.type;
        const label = definition.title || name;
        const rules = required.has(name) ? [{ required: true, message: `请填写 ${label}` }] : [];
        if (type === "boolean") {
          return (
            <Form.Item key={name} name={["parameters", name]} label={label} valuePropName="checked" initialValue={definition.default ?? false}>
              <Switch />
            </Form.Item>
          );
        }
        if (type === "integer" || type === "number") {
          return (
            <Form.Item key={name} name={["parameters", name]} label={label} rules={rules} initialValue={definition.default}>
              <InputNumber style={{ width: "100%" }} />
            </Form.Item>
          );
        }
        return (
          <Form.Item key={name} name={["parameters", name]} label={label} rules={rules} initialValue={definition.default}>
            {(definition as Record<string, unknown>)["x-secret"] ? <Input.Password autoComplete="new-password" /> : <Input />}
          </Form.Item>
        );
      })}
    </>
  );
}
