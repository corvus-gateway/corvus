import { useState } from "react";
import { toast } from "sonner";
import { KeyRound, Pencil, Plus, RefreshCw, Trash2 } from "lucide-react";
import { api, ApiError, type DnsCredential } from "../api";
import { ErrorBox, useSubmit } from "./common";
import { Button } from "./ui/button";
import { Card, CardHeader } from "./ui/display";
import { FormSection, IdField, Input, fieldErr } from "./ui/form";
import { ConfirmDialog, RowMenu, Sheet } from "./ui/overlay";
import { Table, THead, Td, Th, Tr } from "./ui/table";
import type { Resource } from "../hooks/useResource";
import type { DnsProvider } from "../hooks/useDnsProviders";
import { DnsCredentialInputs, autoCredName } from "./DnsFields";
import { useCan } from "../lib/rbac";
import { useI18n } from "../i18n";

type Mode = "add" | "replace" | "rename";
interface Edit { mode: Mode; id?: number; name: string; provider: string; creds: string }

/** Saved DNS-01 credentials (certificates page): list, add, replace secret, rename, delete. */
export function DnsCredentialsCard({ list, providers }: { list: Resource<DnsCredential[]>; providers: DnsProvider[] }) {
  const { t } = useI18n();
  const { can } = useCan();
  const [edit, setEdit] = useState<Edit | null>(null);
  const [deleting, setDeleting] = useState<DnsCredential | null>(null);
  const sub = useSubmit();
  const row = useSubmit();
  const items = list.data ?? [];
  if (list.missing) return null;

  const template = (id: string) => providers.find((p) => p.id === id)?.credentials_template ?? "";
  const providerName = (id: string) => providers.find((p) => p.id === id)?.name ?? id;
  const open = (e: Edit) => { sub.setError(null); setEdit(e); };

  const save = async () => {
    if (!edit) return;
    const body: Record<string, unknown> = {};
    if (edit.mode !== "replace") body.name = edit.name.trim();
    if (edit.mode !== "rename") { body.provider = edit.provider; body.credentials = edit.creds; }
    const r = await sub.run(() => (edit.id ? api.put(`/api/dns-credentials/${edit.id}`, body) : api.post("/api/dns-credentials", body)), { success: t("dnsCredSaved"), deployed: false });
    if (r !== undefined) { setEdit(null); list.reload(); }
  };
  const remove = async () => {
    if (!deleting) return;
    // A refusal (credential still in use) is a validation error, which useSubmit doesn't toast.
    await row.run(() => api.del(`/api/dns-credentials/${deleting.id}`).catch((e) => { toast.error(e instanceof ApiError ? e.message : t("requestFailed")); throw e; }),
      { success: t("dnsCredDeleted"), deployed: false, silentError: true });
    list.reload();
  };

  const title = edit?.mode === "add" ? t("dnsCredAdd") : edit?.mode === "replace" ? t("dnsCredReplace") : t("dnsCredRename");
  const desc = edit?.mode === "add" ? t("dnsCredAddDesc") : edit?.mode === "replace" ? t("dnsCredReplaceDesc") : undefined;
  const nameErr = fieldErr(sub.error, "name");

  return (
    <>
      <Card className="mt-4">
        <CardHeader icon={KeyRound} title={t("dnsCredsTitle")} description={t("dnsCredsDesc")} actions={can && (
          <Button size="sm" onClick={() => open({ mode: "add", name: "", provider: providers[0].id, creds: providers[0].credentials_template })}><Plus className="h-4 w-4" />{t("dnsCredAdd")}</Button>
        )} />
        {!items.length ? <p className="px-4 pb-4 pt-1 text-xs text-fg-subtle">{list.loading ? "…" : t("dnsCredsEmpty")}</p> : (
          <Table>
            <THead><tr><Th>{t("name")}</Th><Th>{t("dnsProvider")}</Th><Th>{t("dnsCredUsedBy")}</Th><Th className="w-12"><span className="sr-only">{t("actions")}</span></Th></tr></THead>
            <tbody>
              {items.map((c) => (
                <Tr key={c.id}>
                  <Td className="font-medium">{c.name}</Td>
                  <Td className="text-fg-muted">{providerName(c.provider)}</Td>
                  <Td className="num text-fg-muted">{c.used_by}</Td>
                  <Td className="text-right"><RowMenu items={[
                    { label: t("dnsCredReplace"), icon: RefreshCw, onSelect: () => open({ mode: "replace", id: c.id, name: c.name, provider: c.provider, creds: template(c.provider) }) },
                    { label: t("dnsCredRename"), icon: Pencil, onSelect: () => open({ mode: "rename", id: c.id, name: c.name, provider: c.provider, creds: "" }) },
                    { label: t("delete"), icon: Trash2, danger: true, separatorBefore: true, onSelect: () => (c.used_by > 0 ? toast.error(t("dnsCredInUse")) : setDeleting(c)) },
                  ]} /></Td>
                </Tr>
              ))}
            </tbody>
          </Table>
        )}
      </Card>

      <Sheet open={!!edit} onOpenChange={(o) => !o && setEdit(null)} onSubmit={save} title={title} description={desc}
        footer={<><Button onClick={() => setEdit(null)}>{t("cancel")}</Button><Button type="submit" variant="primary" loading={sub.busy}>{t("save")}</Button></>}>
        {edit && <>
          {sub.error && !nameErr && !fieldErr(sub.error, "credentials") ? <div className="px-6 pt-5"><ErrorBox error={sub.error} /></div> : null}
          <FormSection title={edit.mode === "replace" ? edit.name : t("secBasic")}>
            {edit.mode !== "replace" && (
              <IdField label={t("name")} required={edit.mode === "rename"} error={nameErr}>
                {(id) => <Input id={id} value={edit.name} invalid={!!nameErr} placeholder={edit.mode === "add" ? autoCredName(items, providers, edit.provider) : undefined} onChange={(e) => setEdit({ ...edit, name: e.target.value })} />}
              </IdField>
            )}
            {edit.mode !== "rename" && (
              <DnsCredentialInputs providers={providers} provider={edit.provider} creds={edit.creds} error={fieldErr(sub.error, "credentials")} onChange={(x) => setEdit({ ...edit, ...x })} />
            )}
          </FormSection>
        </>}
      </Sheet>

      <ConfirmDialog open={!!deleting} onOpenChange={(o) => !o && setDeleting(null)} title={t("dnsCredDeleteTitle")} description={t("dnsCredDeleteDesc")} confirmLabel={t("delete")} onConfirm={remove}>
        {deleting && <div className="rounded-lg border border-line bg-surface-2 px-3 py-2 text-sm font-medium">{deleting.name}</div>}
      </ConfirmDialog>
    </>
  );
}
