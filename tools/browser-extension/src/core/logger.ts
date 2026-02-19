type LogLevel = "debug" | "info" | "warn" | "error";

const PREFIX = "[PT Tools Helper]";

function log(level: LogLevel, message: string, payload?: unknown): void {
  if (payload === undefined) {
    console[level](`${PREFIX} ${message}`);
    return;
  }

  console[level](`${PREFIX} ${message}`, payload);
}

export const logger = {
  debug: (message: string, payload?: unknown): void => log("debug", message, payload),
  info: (message: string, payload?: unknown): void => log("info", message, payload),
  warn: (message: string, payload?: unknown): void => log("warn", message, payload),
  error: (message: string, payload?: unknown): void => log("error", message, payload),
};
