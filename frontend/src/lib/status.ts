export const TERMINAL_STATUSES = ["succeeded", "failed", "cancelled", "skipped", "missed"];

export const STATUS_COLORS: Record<string, string> = {
  succeeded: "success",
  failed: "error",
  running: "processing",
  queued: "default",
  retry_wait: "warning",
  cancelled: "default",
  skipped: "warning",
  missed: "warning",
  scheduled: "blue",
};
