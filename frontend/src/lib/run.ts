import type { Run } from "../types";

export const PARAM_PREVIEW_CHARS = 64;

export function parameterText(item: Run): string {
  if (item.arguments && item.arguments.length > 0) {
    return item.arguments.join(" ");
  }
  const entries = Object.entries(item.parameters || {});
  if (entries.length === 0) return "-";
  return entries
    .map(([key, value]) => `${key}=${typeof value === "string" ? value : JSON.stringify(value)}`)
    .join(", ");
}

export function parameterPreview(full: string): { text: string; truncated: boolean } {
  const chars = Array.from(full);
  if (chars.length <= PARAM_PREVIEW_CHARS) return { text: full, truncated: false };
  return { text: `${chars.slice(0, PARAM_PREVIEW_CHARS).join("")}…`, truncated: true };
}
