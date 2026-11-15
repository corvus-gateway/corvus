import type { K } from "../retention.i18n";

export const retentionStrings: Record<K, string> = {
  navRetention: "Data retention",
  retentionDesc: "Choose how long and how much traffic statistics, security events and request logs are kept. Data past the limits is cleaned up automatically every hour.",
  retentionHow: "Statistics and events survive restarts. Request logs are written to one file per day and earlier days are compressed. When the size limit is exceeded, the oldest day files are deleted first.",
  logMode: "Request log",
  logModeOff: "Off", logModeNotable: "Notable requests", logModeAll: "All requests",
  logModeDesc: "Notable: 4xx/5xx responses, blocks, security detections and access-queue waits. \"All requests\" can reach several GB per day on busy sites.",
  logDays: "Keep logs (days)", logMaxMb: "Log size limit (MB)", logMaxHint: "When exceeded, the oldest day files are deleted first.",
  statsDays: "Keep statistics (days)", eventsDays: "Keep events (days)",
  storageUsage: "Storage", dbSize: "Statistics & events database", logSize: "Request logs", logFilesN: "{n} files", logLimit: "max {mb} MB",
  droppedLines: "{n} log lines dropped because the disk could not keep up",
  archives: "Request log files", noArchives: "No log files", noArchivesDesc: "With the request log enabled, one file per day appears here.",
  fileName: "File", fileSize: "Size", fileModified: "Modified", retentionSaved: "Retention settings saved",
};
