import { useMemo, useState } from "react";
import { ArrowRight, Cable, Lock, Pencil, Plus, Trash2 } from "lucide-react";
import { api, type Certificate, type Stream } from "../api";
import { NoteLine, NotesSection, notesMatch } from "../components/NotesField";
import { ErrorBox, PageHeader, Toolbar, useSubmit } from "../components/common";
import { Button } from "../components/ui/button";
import { Badge, Card, EmptyState, Mono, TableSkeleton } from "../components/ui/display";
import { Field, FormSection, IdField, Input, NumberInput, Segmented, Select, SearchInput, Switch, SwitchRow, fieldErr } from "../components/ui/form";
import { ConfirmDialog, RowMenu, Sheet } from "../components/ui/overlay";
import { Table, THead, Td, Th, Tr } from "../components/ui/table";
import { useResource } from "../hooks/useResource";
import { useCan } from "../lib/rbac";
import { useI18n } from "../i18n";
import { relTime } from "../lib/utils";

type Form = Omit<Stream, "id" | "created_at" | "updated_at">;
type Proto = "tcp" | "udp" | "both";

const blank: Form = { incoming_port: NaN, tcp_forwarding: true, udp_forwarding: false, forward_host: "", forward_port: NaN, certificate_id: null, enabled: true, notes: "", proxy_protocol_in: false, proxy_protocol_out: false };

function toForm(s: Stream): Form {
  const { id: _i, created_at: _c, updated_at: _u, ...rest } = s;
  return { ...blank, ...rest };
}

const protoOf = (s: Pick<Stream, "tcp_forwarding" | "udp_forwarding">): Proto => (s.tcp_forwarding && s.udp_forwarding ? "both" : s.udp_forwarding ? "udp" : "tcp");
const protoLabel = (s: Pick<Stream, "tcp_forwarding" | "udp_forwarding">) => [s.tcp_forwarding && "TCP", s.udp_forwarding && "UDP"].filter(Boolean).join(" + ");

export default function Streams() {
  const { t, lang } = useI18n();
  const { can } = useCan();
  const streams = useResource<Stream[]>("/api/streams", { poll: 15000 });
  const certs = useResource<Certificate[]>("/api/certificates");

  const [q, setQ] = useState("");
  const [editing, setEditing] = useState<{ id: number | null; form: Form } | null>(null);
  const [deleting, setDeleting] = useState<Stream | null>(null);
  const { busy, error, setError, run } = useSubmit();
  const rowAction = useSubmit({ toastAll: true });

  const cert = (id: number | null) => certs.data?.find((c) => c.id === id);

  const filtered = useMemo(() => {
    const s = q.trim().toLowerCase();
    if (!s) return streams.data ?? [];
    return (streams.data ?? []).filter((x) => String(x.incoming_port).includes(s) || x.forward_host.toLowerCase().includes(s) || String(x.forward_port).includes(s) || notesMatch(x.notes, s));
  }, [streams.data, q]);

  const open = (s?: Stream) => { setError(null); setEditing({ id: s?.id ?? null, form: s ? toForm(s) : { ...blank } }); };
  const f = editing?.form;
  const set = (patch: Partial<Form>) => setEditing((e) => (e ? { ...e, form: { ...e.form, ...patch } } : e));
  const setProto = (p: Proto) => set({ tcp_forwarding: p !== "udp", udp_forwarding: p !== "tcp", ...(p === "udp" ? { certificate_id: null, proxy_protocol_in: false, proxy_protocol_out: false } : {}) });
  const incomplete = !f || !Number.isFinite(f.incoming_port) || !Number.isFinite(f.forward_port) || !f.forward_host.trim();
  const pending = !!f?.certificate_id && cert(f.certificate_id)?.status === "pending";

  const save = async () => {
    if (!editing) return;
    const { id, form } = editing;
    const r = await run(() => (id ? api.put(`/api/streams/${id}`, form) : api.post("/api/streams", form)), { success: t("savedDeployed") });
    if (r !== undefined) { setEditing(null); streams.reload(); window.dispatchEvent(new Event("corvus:refresh")); }
  };

  const toggle = async (s: Stream, enabled: boolean) => {
    const r = await rowAction.run(() => api.put(`/api/streams/${s.id}`, { ...toForm(s), enabled }), { success: enabled ? t("hostEnabled") : t("hostDisabled") });
    if (r !== undefined) { streams.reload(); window.dispatchEvent(new Event("corvus:refresh")); }
  };

  const remove = async () => {
    if (!deleting) return;
    const r = await rowAction.run(() => api.del(`/api/streams/${deleting.id}`), { success: t("deletedDeployed") });
    if (r !== undefined) { streams.reload(); window.dispatchEvent(new Event("corvus:refresh")); }
  };

  return (
    <>
      <PageHeader title={t("navStreams")} description={t("streamsDesc")} actions={can && <Button variant="primary" onClick={() => open()}><Plus className="h-4 w-4" />{t("addStream")}</Button>} />
      <Card>
        <Toolbar>
          <SearchInput className="w-full sm:w-72" value={q} onChange={setQ} placeholder={t("searchStreams")} />
          <span className="ml-auto text-xs text-fg-subtle num">{t("nItems", { n: filtered.length })}</span>
        </Toolbar>
        {streams.loading ? <TableSkeleton cols={4} /> : streams.error && !streams.data ? <div className="p-4"><ErrorBox error={streams.error} /></div> :
          !streams.data?.length ? (
            <EmptyState icon={Cable} title={t("noStreams")} description={t("noStreamsDesc")} action={can ? <Button variant="primary" onClick={() => open()}><Plus className="h-4 w-4" />{t("addStream")}</Button> : undefined} />
          ) : !filtered.length ? (
            <EmptyState icon={Cable} title={t("noResults")} description={t("noResultsDesc")} />
          ) : (
            <Table>
              <THead><tr>
                <Th>{t("incomingPort")}</Th><Th className="hidden md:table-cell">{t("forwardTo")}</Th><Th className="hidden sm:table-cell">{t("sslCol")}</Th>
                <Th>{t("status")}</Th><Th className="w-12"><span className="sr-only">{t("actions")}</span></Th>
              </tr></THead>
              <tbody>
                {filtered.map((s) => {
                  const c = cert(s.certificate_id);
                  return (
                    <Tr key={s.id}>
                      <Td>
                        <div className="flex items-center gap-2"><Mono className="font-medium text-fg">{s.incoming_port}</Mono><Badge>{protoLabel(s)}</Badge></div>
                        <div className="mt-0.5 text-xs text-fg-subtle">{relTime(s.updated_at, lang)}</div>
                        <NoteLine notes={s.notes} />
                      </Td>
                      <Td className="hidden md:table-cell">
                        <Mono className="inline-flex items-center gap-1.5 text-fg-muted"><ArrowRight className="h-3 w-3 text-fg-subtle" aria-hidden />{s.forward_host}:{s.forward_port}</Mono>
                      </Td>
                      <Td className="hidden sm:table-cell">
                        {c ? <Badge tone={c.status === "pending" ? "info" : "ok"} icon={Lock}>{c.name}</Badge> : <span className="text-xs text-fg-subtle">{t("sslNone")}</span>}
                      </Td>
                      <Td>
                        <div className="flex items-center gap-2">
                          <Switch checked={s.enabled} onCheckedChange={(v) => toggle(s, v)} label={`${s.incoming_port} ${t("enabled")}`} disabled={rowAction.busy || !can} />
                          <span className="hidden text-xs text-fg-muted xl:inline">{s.enabled ? t("enabled") : t("disabled")}</span>
                        </div>
                      </Td>
                      <Td className="text-right">
                        <RowMenu items={[
                          { label: t("edit"), icon: Pencil, onSelect: () => open(s) },
                          ...(can ? [{ label: t("delete"), icon: Trash2, danger: true, separatorBefore: true, onSelect: () => setDeleting(s) }] : []),
                        ]} />
                      </Td>
                    </Tr>
                  );
                })}
              </tbody>
            </Table>
          )}
      </Card>

      <Sheet open={!!editing} onOpenChange={(o) => !o && setEditing(null)} onSubmit={save} width="max-w-[34rem]"
        title={editing?.id ? t("editStream") : t("addStream")} description={t("streamSheetDesc")}
        footer={<><Button onClick={() => setEditing(null)}>{t("cancel")}</Button><Button type="submit" variant="primary" loading={busy} disabled={incomplete || !can}>{t("saveAndDeploy")}</Button></>}>
        {f && (<>
          {error && <div className="px-6 pt-5"><ErrorBox error={error} /></div>}
          <FormSection title={t("secStreamSource")}>
            <IdField label={t("incomingPort")} required hint={t("incomingPortHint")} error={fieldErr(error, "incoming_port")}>
              {(id) => <NumberInput id={id} min={1} max={65535} value={f.incoming_port} onChange={(v) => set({ incoming_port: v })} placeholder="5432" />}
            </IdField>
            <Field label={t("streamProtocol")} hint={t("streamProtocolHint")} error={fieldErr(error, "tcp_forwarding")}>
              <Segmented label={t("streamProtocol")} value={protoOf(f)} onChange={setProto}
                options={[{ value: "tcp", label: "TCP" }, { value: "udp", label: "UDP" }, { value: "both", label: "TCP + UDP" }]} />
            </Field>
          </FormSection>

          <FormSection title={t("secStreamForward")}>
            <div className="grid gap-4 sm:grid-cols-[2fr_1fr]">
              <IdField label={t("forwardHost")} required error={fieldErr(error, "forward_host")}>
                {(id) => <Input id={id} className="font-mono" value={f.forward_host} onChange={(e) => set({ forward_host: e.target.value })} placeholder="db" invalid={!!fieldErr(error, "forward_host")} />}
              </IdField>
              <IdField label={t("forwardPort")} required error={fieldErr(error, "forward_port")}>
                {(id) => <NumberInput id={id} min={1} max={65535} value={f.forward_port} onChange={(v) => set({ forward_port: v })} placeholder="5432" />}
              </IdField>
            </div>
          </FormSection>

          <FormSection title={t("secTls")}>
            <IdField label={t("certificate")} hint={pending ? t("streamTlsPending") : t("streamTlsHint")} error={fieldErr(error, "certificate_id")}>
              {(id) => (
                <Select id={id} value={f.certificate_id ?? ""} disabled={!f.tcp_forwarding} onChange={(e) => set({ certificate_id: e.target.value ? Number(e.target.value) : null })}>
                  <option value="">{t("certNone")}</option>
                  {certs.data?.map((c) => <option key={c.id} value={c.id}>{c.name} · {c.domains.slice(0, 2).join(", ")}{c.status === "pending" ? ` (${t("certStatusPending")})` : c.status === "error" ? ` (${t("certError")})` : ""}</option>)}
                </Select>
              )}
            </IdField>
          </FormSection>

          {f.tcp_forwarding && (
            <FormSection title={t("secStreamPP")} description={f.udp_forwarding ? t("streamPPBoth") : undefined}>
              <div className="divide-y divide-line">
                <SwitchRow label={t("streamPPIn")} description={t("streamPPInDesc")} checked={!!f.proxy_protocol_in} onChange={(v) => set({ proxy_protocol_in: v })} />
                <SwitchRow label={t("streamPPOut")} description={t("streamPPOutDesc")} checked={!!f.proxy_protocol_out} onChange={(v) => set({ proxy_protocol_out: v })} />
              </div>
              {(fieldErr(error, "proxy_protocol_in") || fieldErr(error, "proxy_protocol_out")) && (
                <p role="alert" className="text-xs text-danger">{fieldErr(error, "proxy_protocol_in") || fieldErr(error, "proxy_protocol_out")}</p>
              )}
            </FormSection>
          )}

          <NotesSection value={f.notes ?? ""} onChange={(v) => set({ notes: v })} error={error} disabled={!can} />

          <FormSection title={t("status")}>
            <SwitchRow label={t("enabled")} description={t("streamEnabledDesc")} checked={f.enabled} onChange={(v) => set({ enabled: v })} />
          </FormSection>
        </>)}
      </Sheet>

      <ConfirmDialog open={!!deleting} onOpenChange={(o) => !o && setDeleting(null)} title={t("deleteStreamTitle")} confirmLabel={t("delete")}
        description={t("confirmDelete")} onConfirm={remove}>
        {deleting && <div className="rounded-lg border border-line bg-surface-2 px-3 py-2 font-mono text-xs">{deleting.incoming_port} {protoLabel(deleting)} → {deleting.forward_host}:{deleting.forward_port}</div>}
      </ConfirmDialog>
    </>
  );
}
