import type { K } from "../backup.i18n";

export const backupStrings: Record<K, string> = {
  backupTitle: "备份与恢复",
  backupDesc: "将设置数据库、证书和密钥下载为一个归档文件，或从备份恢复。",
  backupSecretsTitle: "备份文件包含敏感信息",
  backupSecretsWarn: "其中包括证书私钥以及用于解密数据库中机密信息的密钥。请像对待密码一样保管：加密存储并限制访问。",
  backupPgNote: "此安装使用 PostgreSQL：备份仅包含证书和密钥。请使用 pg_dump 单独备份数据库。",
  backupExcluded: "不包含：生成的配置版本、流量历史、请求日志和国家数据库（会重新生成或重新下载）。",
  backupDownload: "下载备份", backupStarted: "正在创建备份，下载即将开始。",
  restoreTitle: "从备份恢复",
  restoreHow: "上传的归档会先经过检查并暂存。下次重启时将替换当前的设置、证书和密钥；原有文件保存在数据目录的 restore-backup-<时间>/ 中。",
  restoreChoose: "选择备份文件", restoreUpload: "上传并检查", restoreChecked: "备份已检查",
  restoreVersion: "Corvus 版本", restoreCreated: "创建时间", restoreEngine: "数据库", restoreFiles: "文件",
  restoreCerts: "证书", restoreSecrets: "密钥",
  restoreNoDb: "此备份不含数据库（PostgreSQL）：请使用 pg_restore 恢复数据库。",
  restoreVersionDiffers: "该备份由 Corvus {v} 创建（当前为 {cur}）。如有需要，启动时会迁移数据库。",
  restoreConfirm: "下次重启时恢复",
  restoreConfirmWarn: "当前所有设置、证书和密钥都将被备份内容替换。从现在到重启之间所做的更改将丢失。",
  restoreCancel: "取消恢复", restoreStaged: "已安排恢复", restoreDiscarded: "已取消恢复",
  restorePending: "恢复已就绪 — 重启容器即可完成",
  restorePendingDesc: "请在 docker-compose.yml 所在目录运行以下命令。在此之前 Corvus 继续使用当前配置运行。",
};
