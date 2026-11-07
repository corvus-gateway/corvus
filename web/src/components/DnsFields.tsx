import { useEffect } from "react";
import type { DnsCredential } from "../api";
import type { DnsProvider } from "../hooks/useDnsProviders";
import { useResource } from "../hooks/useResource";
import { useI18n } from "../i18n";
import { IdField, Input, Select, Textarea } from "./ui/form";

/** A DNS-01 credential choice: a saved credential, or new credentials (saved with the certificate). */
export interface DnsChoice { credential?: number | null; provider: string; creds: string; name: string }

export const newDnsChoice = (p: DnsProvider): DnsChoice => ({ provider: p.id, creds: p.credentials_template, name: "" });

/** Request fields for a DNS-01 order (certificate_request or /api/certificates/acme). */
export function dnsRequest(c: DnsChoice): Record<string, unknown> {
  return c.credential ? { dns_credential_id: c.credential } : { dns_provider: c.provider, dns_credentials: c.creds, dns_credential_name: c.name.trim() };
}
export const dnsReady = (c: DnsChoice) => !!c.credential || c.creds.trim().length > 0;

export function useDnsCredentials() {
  return useResource<DnsCredential[]>("/api/dns-credentials");
}

/** Next free automatic name ("Cloudflare", "Cloudflare 2", ...), as the server picks it. */
export function autoCredName(list: DnsCredential[], providers: DnsProvider[], provider: string): string {
  const base = providers.find((p) => p.id === provider)?.name ?? provider;
  const used = new Set(list.map((c) => c.name));
  let name = base;
  for (let i = 2; used.has(name); i++) name = `${base} ${i}`;
  return name;
}

/** DNS provider select + credentials textarea prefilled with the provider template (NPM-style). */
export function DnsCredentialInputs({ providers, provider, creds, onChange, error }: {
  providers: DnsProvider[]; provider: string; creds: string; onChange: (p: { provider: string; creds: string }) => void; error?: string;
}) {
  const { t } = useI18n();
  return (
    <>
      <IdField label={t("dnsProvider")}>
        {(id) => (
          <Select id={id} value={provider} onChange={(e) => onChange({ provider: e.target.value, creds: providers.find((p) => p.id === e.target.value)?.credentials_template ?? "" })}>
            {providers.map((p) => <option key={p.id} value={p.id}>{p.name}</option>)}
          </Select>
        )}
      </IdField>
      <IdField label={t("dnsCredentials")} required hint={t("dnsCredentialsHint")} error={error}>
        {(id) => <Textarea id={id} rows={4} spellCheck={false} autoComplete="off" className="font-mono text-xs leading-5" invalid={!!error}
          value={creds} onChange={(e) => onChange({ provider, creds: e.target.value })} />}
      </IdField>
    </>
  );
}

/**
 * DNS-01 credentials for a certificate request: pick a saved credential or enter new ones.
 * Without saved credentials this is just the provider + credentials fields; new
 * credentials are saved (auto-named) so the next certificate can pick them.
 */
export function DnsFields({ providers, value, onChange, fieldError }: {
  providers: DnsProvider[]; value: DnsChoice; onChange: (c: DnsChoice) => void; fieldError: (field: string) => string | undefined;
}) {
  const { t } = useI18n();
  const saved = useDnsCredentials();
  const list = saved.data ?? [];
  // Default to the first saved credential once the list is known.
  useEffect(() => {
    if (value.credential === undefined && saved.data) onChange({ ...value, credential: saved.data[0]?.id ?? null });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [saved.data, value.credential]);
  const isNew = !value.credential;
  const providerName = (id: string) => providers.find((p) => p.id === id)?.name ?? id;
  const credErr = fieldError("dns_credential_id");
  const nameErr = fieldError("dns_credential_name");
  return (
    <>
      {list.length > 0 && (
        <IdField label={t("dnsCredSelect")} error={credErr}>
          {(id) => (
            <Select id={id} value={value.credential ?? ""} invalid={!!credErr} onChange={(e) => onChange({ ...value, credential: e.target.value ? Number(e.target.value) : null })}>
              {list.map((c) => <option key={c.id} value={c.id}>{c.name} · {providerName(c.provider)}</option>)}
              <option value="">{t("dnsCredNew")}</option>
            </Select>
          )}
        </IdField>
      )}
      {isNew && <>
        <DnsCredentialInputs providers={providers} provider={value.provider} creds={value.creds} error={fieldError("dns_credentials_ini") ?? fieldError("dns_credentials")}
          onChange={(x) => onChange({ ...value, ...x })} />
        {list.length > 0 && (
          <IdField label={t("dnsCredName")} hint={t("dnsCredNameHint")} error={nameErr}>
            {(id) => <Input id={id} value={value.name} invalid={!!nameErr} placeholder={autoCredName(list, providers, value.provider)} onChange={(e) => onChange({ ...value, name: e.target.value })} />}
          </IdField>
        )}
      </>}
    </>
  );
}
