import type { K } from "../retention.i18n";

export const retentionStrings: Record<K, string> = {
  navRetention: "数据保留",
  retentionDesc: "设置流量统计、安全事件和请求日志的保留时长与容量。超出限制的数据每小时自动清理。",
  retentionHow: "统计和事件在重启后保留。请求日志按天写入文件，过往日期的文件会被压缩。超出容量上限时，从最早日期的文件开始删除。",
  logMode: "请求日志",
  logModeOff: "不保存", logModeNotable: "仅重要请求", logModeAll: "全部请求",
  logModeDesc: "重要请求：4xx/5xx 响应、拦截、安全检测和访问排队等待。流量较大时“全部请求”每天可能达到数 GB。",
  logDays: "日志保留（天）", logMaxMb: "日志容量上限 (MB)", logMaxHint: "超出时从最早日期的文件开始删除。",
  statsDays: "统计保留（天）", eventsDays: "事件保留（天）",
  storageUsage: "存储空间", dbSize: "统计与事件数据库", logSize: "请求日志", logFilesN: "{n} 个文件", logLimit: "上限 {mb} MB",
  droppedLines: "因磁盘写入跟不上而丢弃 {n} 行日志",
  archives: "请求日志文件", noArchives: "暂无日志文件", noArchivesDesc: "开启请求日志后，每天的日志文件会显示在这里。",
  fileName: "文件", fileSize: "大小", fileModified: "修改时间", retentionSaved: "保留设置已保存",
};
