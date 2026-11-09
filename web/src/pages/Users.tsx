import { useMemo, useState } from "react";
import { Copy, KeyRound, Lock, Pencil, Plus, ShieldOff, Trash2, UserCheck, UserX, Users as UsersIcon, Wand2 } from "lucide-react";
import { useNavigate } from "react-router-dom";
import { toast } from "sonner";
import { api, ApiError, type User } from "../api";
import { ErrorBox, PageHeader, Toolbar, useSubmit } from "../components/common";
import { Button } from "../components/ui/button";
import { Badge, Callout, Card, ComingSoon, EmptyState, TableSkeleton } from "../components/ui/display";
import { FormSection, IdField, Input, SearchInput, Select, fieldErr } from "../components/ui/form";
import { ConfirmDialog, Modal, RowMenu, Sheet } from "../components/ui/overlay";
import { Table, THead, Td, Th, Tr } from "../components/ui/table";
import { useResource } from "../hooks/useResource";
import { useI18n } from "../i18n";
import { useApp } from "../lib/app";
import { roleKey, useCan, type Role } from "../lib/rbac";
import { copyText, relTime } from "../lib/utils";

const ROLES: Role[] = ["admin", "operator", "viewer"];
const roleDesc = { admin: "roleAdminDesc", operator: "roleOperatorDesc", viewer: "roleViewerDesc" } as const;
const roleTone = { admin: "brand", operator: "info", viewer: "neutral" } as const;

export function genPassword(len = 20): string {
  const chars = "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz23456789!@#$%^&*-_";
  const buf = new Uint32Array(len);
  crypto.getRandomValues(buf);
  return Array.from(buf, (n) => chars[n % chars.length]).join("");
}

interface Form { email: string; name: string; role: Role; password: string }
const blank: Form = { email: "", name: "", role: "operator", password: "" };

export default function Users() {
  const { t, lang } = useI18n();
  const { isAdmin } = useCan();
  const { user: me } = useApp();
  const navigate = useNavigate();
  const list = useResource<User[]>(isAdmin ? "/api/users" : null, { poll: 20000 });
  const [q, setQ] = useState("");
  const [editing, setEditing] = useState<{ id: number | null; form: Form } | null>(null);
  const [pw, setPw] = useState<{ user: User; value: string } | null>(null);
  const [mfaFor, setMfaFor] = useState<User | null>(null);
  const [deleting, setDeleting] = useState<User | null>(null);
  const [disabling, setDisabling] = useState<User | null>(null);
  const { busy, error, setError, run } = useSubmit();
  const row = useSubmit();
  const pwSub = useSubmit();

  const rows = useMemo(() => {
    const s = q.trim().toLowerCase();
    return (list.data ?? []).filter((u) => !s || `${u.name} ${u.email}`.toLowerCase().includes(s));
  }, [list.data, q]);

  if (!isAdmin) {
    return (<><PageHeader title={t("navUsers")} description={t("usersDesc")} />
      <Card><EmptyState icon={Lock} title={t("noPermissionTitle")} description={t("noPermissionDesc")} /></Card></>);
  }

  const done = () => list.reload();
  const open = (u?: User) => { setError(null); setEditing({ id: u?.id ?? null, form: u ? { email: u.email, name: u.name, role: u.role as Role, password: "" } : { ...blank } }); };
  const save = async () => {
    if (!editing) return;
    const { id, form } = editing;
    const body = id ? { name: form.name, role: form.role, disabled: !!list.data?.find((u) => u.id === id)?.disabled } : { ...form };
    const r = await run(() => (id ? api.put(`/api/users/${id}`, body) : api.post("/api/users", body)), { deployed: false, success: t("userSaved") });
    if (r !== undefined) { setEditing(null); done(); }
  };
  const setDisabled = async (u: User, disabled: boolean) => {
    const r = await row.run(() => api.put(`/api/users/${u.id}`, { name: u.name, role: u.role, disabled }), { deployed: false, success: disabled ? t("userDisabledToast") : t("userEnabledToast") });
    if (r !== undefined) done();
  };
  const resetPw = async () => {
    if (!pw) return;
    const r = await pwSub.run(() => api.put(`/api/users/${pw.user.id}`, { name: pw.user.name, role: pw.user.role, disabled: !!pw.user.disabled, password: pw.value }), { deployed: false, success: t("passwordResetDone") });
    if (r !== undefined) { setPw(null); done(); }
  };
  const resetMfa = async () => {
    if (!mfaFor) return;
    const r = await row.run(() => api.post(`/api/users/${mfaFor.id}/reset-mfa`), { deployed: false, success: t("mfaResetDone") });
    if (r !== undefined) done();
  };
  const remove = async () => {
    if (!deleting) return;
    const r = await row.run(() => api.del(`/api/users/${deleting.id}`), { deployed: false, success: t("userDeletedToast") });
    if (r !== undefined) done();
  };

  const f = editing?.form;
  const set = (p: Partial<Form>) => setEditing((e) => (e ? { ...e, form: { ...e.form, ...p } } : e));
  const isSelfEdit = !!editing?.id && editing.id === me.id;
  const conflict = error instanceof ApiError && error.status === 409 ? error : null;

  return (
    <>
      <PageHeader title={t("navUsers")} description={t("usersDesc")} actions={!list.missing && <Button variant="primary" onClick={() => open()}><Plus className="h-4 w-4" />{t("addUser")}</Button>} />
      <Card>
        {list.missing ? <ComingSoon icon={UsersIcon} /> : (<>
          <Toolbar><SearchInput className="w-full sm:w-72" value={q} onChange={setQ} placeholder={t("searchUsers")} /><span className="ml-auto text-xs text-fg-subtle num">{t("nItems", { n: rows.length })}</span></Toolbar>
          {list.loading ? <TableSkeleton cols={6} /> : list.error && !list.data ? <div className="p-4"><ErrorBox error={list.error} /></div> : !list.data?.length ? (
            <EmptyState icon={UsersIcon} title={t("noUsers")} description={t("noUsersDesc")} action={<Button variant="primary" onClick={() => open()}><Plus className="h-4 w-4" />{t("addUser")}</Button>} />
          ) : !rows.length ? <EmptyState icon={UsersIcon} title={t("noResults")} description={t("noResultsDesc")} /> : (
            <Table>
              <THead><tr><Th>{t("name")}</Th><Th className="hidden md:table-cell">{t("email")}</Th><Th>{t("role")}</Th><Th className="hidden sm:table-cell">{t("mfaState")}</Th><Th>{t("status")}</Th><Th className="hidden lg:table-cell">{t("lastLogin")}</Th><Th className="w-12"><span className="sr-only">{t("actions")}</span></Th></tr></THead>
              <tbody>
                {rows.map((u) => {
                  const self = u.id === me.id;
                  return (
                    <Tr key={u.id}>
                      <Td><div className="font-medium">{u.name || u.email}{self && <span className="ml-1.5 text-xs font-normal text-fg-subtle">{t("meMark")}</span>}</div><div className="text-xs text-fg-subtle md:hidden">{u.email}</div></Td>
                      <Td className="hidden text-fg-muted md:table-cell">{u.email}</Td>
                      <Td><Badge tone={roleTone[u.role as Role] ?? "neutral"}>{roleKey[u.role] ? t(roleKey[u.role]) : u.role}</Badge></Td>
                      <Td className="hidden sm:table-cell">{u.mfa_enabled ? <Badge tone="ok" dot>{t("on")}</Badge> : <span className="text-xs text-fg-subtle">{t("off")}</span>}</Td>
                      <Td>{u.disabled ? <Badge tone="warn" dot>{t("userInactive")}</Badge> : <Badge tone="ok" dot>{t("userActive")}</Badge>}</Td>
                      <Td className="hidden text-fg-muted lg:table-cell">{u.last_login_at ? relTime(u.last_login_at, lang) : "–"}</Td>
                      <Td className="text-right"><RowMenu items={[
                        { label: t("edit"), icon: Pencil, onSelect: () => open(u) },
                        self
                          ? { label: t("changePassword"), icon: KeyRound, onSelect: () => navigate("/account") }
                          : { label: t("resetPassword"), icon: KeyRound, onSelect: () => { pwSub.setError(null); setPw({ user: u, value: "" }); } },
                        { label: t("resetMfa"), icon: ShieldOff, hidden: !u.mfa_enabled, onSelect: () => setMfaFor(u) },
                        { label: u.disabled ? t("enableUser") : t("disableUser"), icon: u.disabled ? UserCheck : UserX, disabled: self, separatorBefore: true, onSelect: () => (u.disabled ? setDisabled(u, false) : setDisabling(u)) },
                        { label: t("deleteUser"), icon: Trash2, danger: true, disabled: self, onSelect: () => setDeleting(u) },
                      ]} /></Td>
                    </Tr>
                  );
                })}
              </tbody>
            </Table>
          )}
        </>)}
      </Card>

      <Sheet open={!!editing} onOpenChange={(o) => !o && setEditing(null)} onSubmit={save} title={editing?.id ? t("editUser") : t("addUser")} description={t("userSheetDesc")}
        footer={<><Button onClick={() => setEditing(null)}>{t("cancel")}</Button><Button type="submit" variant="primary" loading={busy}>{t("save")}</Button></>}>
        {f && (<>
          {error && <div className="px-6 pt-5">{conflict ? <Callout tone="danger" title={t("conflictTitle")}>{conflict.message}</Callout> : <ErrorBox error={error} />}</div>}
          <FormSection title={t("secBasic")}>
            <IdField label={t("email")} required error={fieldErr(error, "email")}>{(id) => <Input id={id} type="email" autoComplete="off" disabled={!!editing?.id} value={f.email} onChange={(e) => set({ email: e.target.value })} invalid={!!fieldErr(error, "email")} />}</IdField>
            <IdField label={t("name")} error={fieldErr(error, "name")}>{(id) => <Input id={id} value={f.name} onChange={(e) => set({ name: e.target.value })} invalid={!!fieldErr(error, "name")} />}</IdField>
          </FormSection>
          <FormSection title={t("role")}>
            <IdField label={t("role")} error={fieldErr(error, "role")} hint={isSelfEdit ? t("roleSelf") : t(roleDesc[f.role])}>
              {(id) => (
                <Select id={id} value={f.role} disabled={isSelfEdit} onChange={(e) => set({ role: e.target.value as Role })}>
                  {ROLES.map((r) => <option key={r} value={r}>{t(roleKey[r])}</option>)}
                </Select>
              )}
            </IdField>
            <ul className="space-y-1 text-xs text-fg-subtle">{ROLES.map((r) => <li key={r}><span className="font-semibold text-fg-muted">{t(roleKey[r])}</span> — {t(roleDesc[r])}</li>)}</ul>
          </FormSection>
          {!editing?.id && (
            <FormSection title={t("initialPassword")}>
              <PasswordField value={f.password} onChange={(v) => set({ password: v })} error={fieldErr(error, "password")} hint={t("initialPasswordHint")} />
            </FormSection>
          )}
        </>)}
      </Sheet>

      <Modal open={!!pw} onOpenChange={(o) => !o && setPw(null)} title={t("resetPassword")} description={t("resetPasswordDesc")}
        footer={<><Button onClick={() => setPw(null)}>{t("cancel")}</Button><Button variant="primary" loading={pwSub.busy} disabled={!pw || pw.value.length < 10} onClick={resetPw}>{t("resetPassword")}</Button></>}>
        {pw && (<div className="space-y-3">
          <div className="rounded border border-line bg-surface-2 px-3 py-2 text-sm font-medium">{pw.user.name || pw.user.email} <span className="text-fg-subtle">· {pw.user.email}</span></div>
          <ErrorBox error={pwSub.error && !fieldErr(pwSub.error, "password") ? pwSub.error : null} />
          <PasswordField value={pw.value} onChange={(v) => setPw({ ...pw, value: v })} error={fieldErr(pwSub.error, "password")} hint={t("initialPasswordHint")} />
        </div>)}
      </Modal>

      <ConfirmDialog open={!!mfaFor} onOpenChange={(o) => !o && setMfaFor(null)} title={t("resetMfaTitle")} description={t("resetMfaDesc")} confirmLabel={t("resetMfa")} onConfirm={resetMfa}>
        {mfaFor && <div className="rounded border border-line bg-surface-2 px-3 py-2 text-sm font-medium">{mfaFor.name || mfaFor.email}</div>}
      </ConfirmDialog>
      <ConfirmDialog open={!!disabling} onOpenChange={(o) => !o && setDisabling(null)} title={disabling ? t("disableUserTitle", { name: disabling.name || disabling.email }) : ""}
        description={t("disableUserDesc")} confirmLabel={t("disableUser")} onConfirm={async () => { if (disabling) await setDisabled(disabling, true); }} />
      <ConfirmDialog open={!!deleting} onOpenChange={(o) => !o && setDeleting(null)} title={t("deleteUserTitle")} description={t("deleteUserDesc")} confirmLabel={t("deleteUser")} onConfirm={remove}>
        {deleting && <div className="rounded border border-line bg-surface-2 px-3 py-2 text-sm font-medium">{deleting.name || deleting.email}</div>}
      </ConfirmDialog>
    </>
  );
}

function PasswordField({ value, onChange, error, hint }: { value: string; onChange: (v: string) => void; error?: string; hint: string }) {
  const { t } = useI18n();
  return (
    <IdField label={t("password")} required hint={hint} error={error}>
      {(id) => (
        <div className="flex gap-2">
          <Input id={id} type="text" autoComplete="new-password" spellCheck={false} className="font-mono" value={value} onChange={(e) => onChange(e.target.value)} invalid={!!error} />
          <Button className="shrink-0" onClick={() => onChange(genPassword())}><Wand2 className="h-3.5 w-3.5" />{t("generatePassword")}</Button>
          <Button className="shrink-0" disabled={!value} onClick={async () => { if (await copyText(value)) toast.success(t("copied")); }} aria-label={t("copyPassword")}><Copy className="h-3.5 w-3.5" /></Button>
        </div>
      )}
    </IdField>
  );
}


