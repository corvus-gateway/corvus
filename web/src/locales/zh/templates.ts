import type { K } from "../templates.i18n";

export const templatesStrings: Record<K, string> = {
  fromTemplate: "从模板添加",
  templatesTitle: "虚拟补丁模板",
  templatesDesc: "在完成修补前拦截已知 CVE 的规则。请先以“记录”方式应用并确认无误报，再切换为“拦截”。",
  tplSevCritical: "严重", tplSevHigh: "高",
  tplProducts: "影响范围", tplFalsePositives: "误报可能",
  tplExemptList: "豁免 IP 列表（管理员网络）", tplExemptNone: "不豁免",
  tplExemptHint: "此模板可以豁免管理员网络。不指定时规则适用于所有 IP。",
  tplApply: "创建规则", tplCreated: "已根据模板创建规则", tplSelect: "请选择模板",
  managedRules: "托管规则",
  managedRulesDesc: "请求体检测特征与虚拟补丁模板的规则包。仅应用签名有效的更新版本。",
  mrVersion: "版本", mrSource: "来源", mrBuiltin: "内置", mrInstalled: "已更新", mrSignatures: "特征", mrTemplates: "模板", mrChecked: "上次检查",
  mrUrl: "更新 URL", mrUrlHint: "签名从同一地址加上 .sig 后缀获取（仅限 https）。",
  mrAuto: "每天自动检查", mrCheckNow: "立即检查",
  mrUpload: "手动上传", mrUploadHint: "离线环境请上传 bundle.json 和 bundle.json.sig。",
  mrBundleFile: "规则包 (bundle.json)", mrSigFile: "签名 (bundle.json.sig)",
  mrUploaded: "已应用规则包", mrUpToDate: "已检查（有新版本时会自动应用）", mrSaved: "已保存更新设置",
};
