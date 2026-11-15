import type { K } from "../templates.i18n";

export const templatesStrings: Record<K, string> = {
  fromTemplate: "Add from template",
  templatesTitle: "Virtual-patch templates",
  templatesDesc: "Rules that block known CVEs until you can patch. Apply them as Log first, check for false positives, then switch to Block.",
  tplSevCritical: "Critical", tplSevHigh: "High",
  tplProducts: "Affects", tplFalsePositives: "False positives",
  tplExemptList: "Exempt IP list (admin network)", tplExemptNone: "No exemption",
  tplExemptHint: "This template can exempt your admin network. Without it the rule applies to every IP.",
  tplApply: "Create rule", tplCreated: "Rule created from template", tplSelect: "Select a template",
  managedRules: "Managed rules",
  managedRulesDesc: "Bundle of body-inspection signatures and virtual-patch templates. Only newer, validly signed versions are applied.",
  mrVersion: "Version", mrSource: "Source", mrBuiltin: "Built-in", mrInstalled: "Updated", mrSignatures: "Signatures", mrTemplates: "Templates", mrChecked: "Last check",
  mrUrl: "Update URL", mrUrlHint: "The signature is fetched from the same address with .sig appended (https only).",
  mrAuto: "Check daily", mrCheckNow: "Check now",
  mrUpload: "Upload", mrUploadHint: "For offline installations, upload bundle.json and bundle.json.sig.",
  mrBundleFile: "Bundle (bundle.json)", mrSigFile: "Signature (bundle.json.sig)",
  mrUploaded: "Rule bundle applied", mrUpToDate: "Checked (a newer version is applied when available)", mrSaved: "Update settings saved",
};
