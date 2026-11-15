import type { DnsCredsKey } from "../dnscreds.i18n";

export const dnsCredsStrings: Record<DnsCredsKey, string> = {
  dnsCredsTitle: "DNS credentials",
  dnsCredsDesc: "DNS provider API tokens for DNS-01 certificates. Saved once, they are shared by certificates and their renewals. Secrets are stored encrypted and never shown again.",
  dnsCredsEmpty: "No saved credentials yet. Credentials entered when requesting a DNS-01 certificate are saved here.",
  dnsCredAdd: "Add credential",
  dnsCredAddDesc: "Save a DNS provider and API token once, then just pick it when requesting DNS-01 certificates.",
  dnsCredUsedBy: "Certificates",
  dnsCredSelect: "DNS credential",
  dnsCredNew: "Enter new credentials",
  dnsCredName: "Credential name",
  dnsCredNameHint: "Saved under this name so the next certificate can pick it. Leave empty for an automatic name.",
  dnsCredReplace: "Replace secret",
  dnsCredReplaceDesc: "Enter the new credentials. Certificates using this credential use them from their next renewal; nothing is re-issued now.",
  dnsCredRename: "Rename",
  dnsCredSaved: "Credential saved",
  dnsCredDeleted: "Credential deleted",
  dnsCredDeleteTitle: "Delete DNS credential?",
  dnsCredDeleteDesc: "The saved secret is removed. This can't be undone.",
  dnsCredInUse: "Used by certificates, so it can't be deleted",
};
