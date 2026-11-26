import { useMemo, useState } from "react";
import { Copy, LogIn, LogOut, Pencil, Plus, ShieldOff, Trash2, UsersRound, Wand2 } from "lucide-react";
import { toast } from "sonner";
import { api, type SigninGroup, type SigninUser } from "../api";
import { ErrorBox, PageHeader, Toolbar, useSubmit } from "../components/common";
import { Button } from "../components/ui/button";
import { Badge, Card, CardHeader, ComingSoon, EmptyState, TableSkeleton } from "../components/ui/display";
import { FormSection, IdField, Input, SearchInput, SwitchRow, fieldErr } from "../components/ui/form";
import { MultiPicker } from "../components/ui/picker";
import { ConfirmDialog, RowMenu, Sheet, SheetCancel, useDirty } from "../components/ui/overlay";
import { Table, THead, Td, Th, Tr } from "../components/ui/table";
import { useResource } from "../hooks/useResource";
import { useI18n } from "../i18n";
import { useCan } from "../lib/rbac";
import { copyText, fmtDateTime, genPassword, relTime } from "../lib/utils";

interface UserForm { username: string; password: string; totp_required: boolean; enabled: boolean; note: string; group_ids: number[] }
const blankUser: UserForm = { username: "", password: "", totp_required: false, enabled: true, note: "", group_ids: [] };
interface GroupForm { name: string; user_ids: number[] }

/** Sign-in users and their groups: the people a proxy host's login page lets in (#294). */
export default function SigninUsers() {
  const { t, lang } = useI18n();
  const { can } = useCan();
  const users = useResource<SigninUser[]>("/api/signin-users", { poll: 30000 });
  const groups = useResource<SigninGroup[]>("/api/signin-groups", { poll: 30000 });
  const [q, setQ] = useState("");
  const [editing, setEditing] = useState<{ id: number | null; form: UserForm } | null>(null);
  const [editingGroup, setEditingGroup] = useState<{ id: number | null; form: GroupForm } | null>(null);
  const dirty = useDirty(editing);
  const groupDirty = useDirty(editingGroup);
  const [ending, setEnding] = useState<SigninUser | null>(null);
  const [resetting, setResetting] = useState<SigninUser | null>(null);
  const [deleting, setDeleting] = useState<SigninUser | null>(null);
  const [deletingGroup, setDeletingGroup] = useState<SigninGroup | null>(null);
  const userSub = useSubmit();
  const groupSub = useSubmit();
  const row = useSubmit({ toastAll: true });

  const groupName = useMemo(() => new Map((groups.data ?? []).map((g) => [g.id, g.name])), [groups.data]);
  const userName = useMemo(() => new Map((users.data ?? []).map((u) => [u.id, u.username])), [users.data]);
  const groupOptions = useMemo(() => (groups.data ?? []).map((g) => ({ value: String(g.id), label: g.name })), [groups.data]);
  const userOptions = useMemo(() => (users.data ?? []).map((u) => ({ value: String(u.id), label: u.username })), [users.data]);
  const rows = useMemo(() => {
    const s = q.trim().toLowerCase();
    return (users.data ?? []).filter((u) => !s || `${u.username} ${u.note}`.toLowerCase().includes(s));
  }, [users.data, q]);

  // memberships live on both sides: reload both after any change
  const reload = () => { users.reload(); groups.reload(); };

  const openUser = (u?: SigninUser) => {
    userSub.setError(null);
    setEditing({ id: u?.id ?? null, form: u ? { username: u.username, password: "", totp_required: u.totp_required, enabled: u.enabled, note: u.note, group_ids: u.group_ids } : { ...blankUser } });
  };
  const saveUser = async () => {
    if (!editing) return;
    const { id, form } = editing;
    const body = { ...form, username: form.username.trim() };
    const r = await userSub.run(() => (id ? api.put(`/api/signin-users/${id}`, body) : api.post("/api/signin-users", body)), { deployed: false, success: t("siUserSaved") });
    if (r !== undefined) { setEditing(null); reload(); }
  };
  const setU = (p: Partial<UserForm>) => setEditing((e) => (e ? { ...e, form: { ...e.form, ...p } } : e));

  const openGroup = (g?: SigninGroup) => {
    groupSub.setError(null);
    setEditingGroup({ id: g?.id ?? null, form: g ? { name: g.name, user_ids: g.user_ids } : { name: "", user_ids: [] } });
  };
  const saveGroup = async () => {
    if (!editingGroup) return;
    const { id, form } = editingGroup;
    const body = { ...form, name: form.name.trim() };
    const r = await groupSub.run(() => (id ? api.put(`/api/signin-groups/${id}`, body) : api.post("/api/signin-groups", body)), { deployed: false, success: t("siGroupSaved") });
    if (r !== undefined) { setEditingGroup(null); reload(); }
  };
  const setG = (p: Partial<GroupForm>) => setEditingGroup((e) => (e ? { ...e, form: { ...e.form, ...p } } : e));

  const action = async (path: string, success: string, del = false) => {
    const r = await row.run(() => (del ? api.del(path) : api.post(path)), { deployed: false, success });
    if (r !== undefined) reload();
  };

  const f = editing?.form;
  const gf = editingGroup?.form;
  const addUser = can && <Button variant="primary" onClick={() => openUser()}><Plus className="h-4 w-4" />{t("siAddUser")}</Button>;
  const addGroup = can && <Button onClick={() => openGroup()}><Plus className="h-4 w-4" />{t("siAddGroup")}</Button>;

  return (
    <>
      <PageHeader title={t("navSigninUsers")} description={t("siDesc")} actions={!users.missing && addUser} />
      <Card>
        {users.missing ? <ComingSoon icon={LogIn} /> : (<>
          <Toolbar><SearchInput className="w-full sm:w-72" value={q} onChange={setQ} placeholder={t("siSearch")} /><span className="ml-auto text-xs text-fg-subtle num">{t("nItems", { n: rows.length })}</span></Toolbar>
          {users.loading ? <TableSkeleton cols={6} /> : users.error && !users.data ? <div className="p-4"><ErrorBox error={users.error} /></div> : !users.data?.length ? (
            <EmptyState icon={LogIn} title={t("siNoUsers")} description={t("siNoUsersDesc")} action={addUser} />
          ) : !rows.length ? <EmptyState icon={LogIn} title={t("noResults")} description={t("noResultsDesc")} /> : (
            <Table>
              <THead><tr>
                <Th>{t("username")}</Th><Th className="hidden md:table-cell">{t("siGroupsCol")}</Th><Th className="hidden sm:table-cell">{t("siTwoStep")}</Th>
                <Th className="hidden lg:table-cell">{t("lastLogin")}</Th><Th>{t("status")}</Th><Th className="w-12"><span className="sr-only">{t("actions")}</span></Th>
              </tr></THead>
              <tbody>
                {rows.map((u) => (
                  <Tr key={u.id}>
                    <Td><div className="font-medium">{u.username}</div>{u.note && <div className="truncate text-xs text-fg-subtle">{u.note}</div>}</Td>
                    <Td className="hidden md:table-cell">
                      <div className="flex flex-wrap gap-1">{u.group_ids.length ? u.group_ids.map((g) => <Badge key={g}>{groupName.get(g) ?? `#${g}`}</Badge>) : <span className="text-xs text-fg-subtle">–</span>}</div>
                    </Td>
                    <Td className="hidden sm:table-cell">
                      {!u.totp_required ? <span className="text-xs text-fg-subtle">{t("off")}</span> : u.totp_enrolled ? <Badge tone="ok" dot>{t("siTwoStepOn")}</Badge> : <Badge tone="warn" dot>{t("siTwoStepPending")}</Badge>}
                    </Td>
                    <Td className="hidden text-fg-muted lg:table-cell">{u.last_login_at ? <time dateTime={u.last_login_at} title={fmtDateTime(u.last_login_at, lang)}>{relTime(u.last_login_at, lang)}</time> : "–"}</Td>
                    <Td>{u.enabled ? <Badge tone="ok" dot>{t("enabled")}</Badge> : <Badge tone="warn" dot>{t("disabled")}</Badge>}</Td>
                    <Td className="text-right"><RowMenu items={[
                      { label: t("edit"), icon: Pencil, onSelect: () => openUser(u) },
                      { label: t("siEndSessions"), icon: LogOut, onSelect: () => setEnding(u) },
                      { label: t("siResetTwoStep"), icon: ShieldOff, hidden: !u.totp_enrolled, onSelect: () => setResetting(u) },
                      { label: t("siDeleteUser"), icon: Trash2, danger: true, separatorBefore: true, onSelect: () => setDeleting(u) },
                    ]} /></Td>
                  </Tr>
                ))}
              </tbody>
            </Table>
          )}
        </>)}
      </Card>

      {!groups.missing && (
        <Card className="mt-4">
          <CardHeader icon={UsersRound} title={t("siGroupsTitle")} description={t("siGroupsDesc")} actions={groups.data?.length ? addGroup : undefined} />
          {groups.loading ? <TableSkeleton cols={3} rows={3} /> : groups.error && !groups.data ? <div className="p-4"><ErrorBox error={groups.error} /></div> : !groups.data?.length ? (
            <EmptyState icon={UsersRound} title={t("siNoGroups")} description={t("siNoGroupsDesc")} action={addGroup} />
          ) : (
            <Table>
              <THead><tr><Th>{t("siGroupName")}</Th><Th>{t("siMembers")}</Th><Th className="hidden sm:table-cell">{t("navHosts")}</Th><Th className="w-12"><span className="sr-only">{t("actions")}</span></Th></tr></THead>
              <tbody>
                {groups.data.map((g) => (
                  <Tr key={g.id}>
                    <Td className="font-medium">{g.name}</Td>
                    <Td className="text-fg-muted">
                      <span className="num">{t("siMembersN", { n: g.user_ids.length })}</span>
                      {g.user_ids.length > 0 && <div className="truncate text-xs text-fg-subtle">{g.user_ids.slice(0, 5).map((id) => userName.get(id) ?? `#${id}`).join(", ")}{g.user_ids.length > 5 ? " …" : ""}</div>}
                    </Td>
                    <Td className="hidden text-fg-muted sm:table-cell">{g.hosts > 0 ? t("siHostsN", { n: g.hosts }) : "–"}</Td>
                    <Td className="text-right"><RowMenu items={[
                      { label: t("edit"), icon: Pencil, onSelect: () => openGroup(g) },
                      { label: t("siDeleteGroup"), icon: Trash2, danger: true, separatorBefore: true, disabled: g.hosts > 0, onSelect: () => setDeletingGroup(g) },
                    ]} /></Td>
                  </Tr>
                ))}
              </tbody>
            </Table>
          )}
        </Card>
      )}

      <Sheet open={!!editing} onOpenChange={(o) => !o && setEditing(null)} onSubmit={saveUser} dirty={dirty} error={userSub.error}
        title={editing?.id ? t("siEditUser") : t("siAddUser")} description={t("siUserSheetDesc")}
        footer={<><SheetCancel /><Button type="submit" variant="primary" loading={userSub.busy} disabled={!f?.username.trim() || (!editing?.id && !f?.password)}>{t("save")}</Button></>}>
        {f && (<>
          {userSub.error && !fieldErr(userSub.error, "username", "password", "group_ids", "note") && <div className="px-6 pt-5"><ErrorBox error={userSub.error} /></div>}
          <FormSection title={t("secBasic")}>
            <IdField label={t("username")} required hint={t("siUsernameHint")} error={fieldErr(userSub.error, "username")}>
              {(id) => <Input id={id} autoComplete="off" spellCheck={false} maxLength={64} value={f.username} onChange={(e) => setU({ username: e.target.value })} invalid={!!fieldErr(userSub.error, "username")} />}
            </IdField>
            <IdField label={t("password")} required={!editing?.id} hint={editing?.id ? t("siPasswordKeep") : t("siPasswordHint")} error={fieldErr(userSub.error, "password")}>
              {(id) => (
                <div className="flex gap-2">
                  <Input id={id} type="text" autoComplete="new-password" spellCheck={false} className="font-mono" value={f.password} placeholder={editing?.id ? t("keepPassword") : ""}
                    onChange={(e) => setU({ password: e.target.value })} invalid={!!fieldErr(userSub.error, "password")} />
                  <Button className="shrink-0" onClick={() => setU({ password: genPassword(16) })}><Wand2 className="h-3.5 w-3.5" />{t("generatePassword")}</Button>
                  <Button className="shrink-0" disabled={!f.password} onClick={async () => { if (await copyText(f.password)) toast.success(t("copied")); }} aria-label={t("copyPassword")}><Copy className="h-3.5 w-3.5" /></Button>
                </div>
              )}
            </IdField>
            <IdField label={t("siGroupsCol")} hint={groupOptions.length ? t("siGroupsHint") : t("siNoGroupsHint")} error={fieldErr(userSub.error, "group_ids")}>
              {() => <MultiPicker label={t("siGroupsCol")} placeholder={t("siPickGroups")} options={groupOptions} value={f.group_ids.map(String)} onChange={(v) => setU({ group_ids: v.map(Number) })} disabled={!groupOptions.length} />}
            </IdField>
            <IdField label={t("siNote")} error={fieldErr(userSub.error, "note")}>
              {(id) => <Input id={id} maxLength={500} value={f.note} onChange={(e) => setU({ note: e.target.value })} />}
            </IdField>
          </FormSection>
          <FormSection title={t("siTwoStep")}>
            <div className="divide-y divide-line">
              <SwitchRow label={t("siRequireTwoStep")} description={t("siRequireTwoStepDesc")} checked={f.totp_required} onChange={(v) => setU({ totp_required: v })} />
              <SwitchRow label={t("siUserEnabled")} description={t("siUserEnabledDesc")} checked={f.enabled} onChange={(v) => setU({ enabled: v })} />
            </div>
          </FormSection>
        </>)}
      </Sheet>

      <Sheet open={!!editingGroup} onOpenChange={(o) => !o && setEditingGroup(null)} onSubmit={saveGroup} dirty={groupDirty} error={groupSub.error}
        title={editingGroup?.id ? t("siEditGroup") : t("siAddGroup")} description={t("siGroupSheetDesc")}
        footer={<><SheetCancel /><Button type="submit" variant="primary" loading={groupSub.busy} disabled={!gf?.name.trim()}>{t("save")}</Button></>}>
        {gf && (<>
          {groupSub.error && !fieldErr(groupSub.error, "name", "user_ids") && <div className="px-6 pt-5"><ErrorBox error={groupSub.error} /></div>}
          <FormSection title={t("secBasic")}>
            <IdField label={t("siGroupName")} required hint={t("siGroupNameHint")} error={fieldErr(groupSub.error, "name")}>
              {(id) => <Input id={id} autoComplete="off" maxLength={64} value={gf.name} onChange={(e) => setG({ name: e.target.value })} invalid={!!fieldErr(groupSub.error, "name")} />}
            </IdField>
            <IdField label={t("siMembers")} error={fieldErr(groupSub.error, "user_ids")}>
              {() => <MultiPicker label={t("siMembers")} placeholder={t("siPickMembers")} options={userOptions} value={gf.user_ids.map(String)} onChange={(v) => setG({ user_ids: v.map(Number) })} />}
            </IdField>
          </FormSection>
        </>)}
      </Sheet>

      <ConfirmDialog open={!!ending} onOpenChange={(o) => !o && setEnding(null)} title={t("siEndSessionsTitle")} description={t("siEndSessionsDesc")} confirmLabel={t("siEndSessions")}
        onConfirm={async () => { if (ending) await action(`/api/signin-users/${ending.id}/end-sessions`, t("siEndSessionsDone")); }}>
        {ending && <div className="rounded border border-line bg-surface-2 px-3 py-2 text-sm font-medium">{ending.username}</div>}
      </ConfirmDialog>
      <ConfirmDialog open={!!resetting} onOpenChange={(o) => !o && setResetting(null)} title={t("siResetTwoStep")} description={t("siResetTwoStepDesc")} confirmLabel={t("siResetTwoStep")}
        onConfirm={async () => { if (resetting) await action(`/api/signin-users/${resetting.id}/reset-mfa`, t("siResetTwoStepDone")); }}>
        {resetting && <div className="rounded border border-line bg-surface-2 px-3 py-2 text-sm font-medium">{resetting.username}</div>}
      </ConfirmDialog>
      <ConfirmDialog open={!!deleting} onOpenChange={(o) => !o && setDeleting(null)} title={t("siDeleteUser")} description={t("siDeleteUserDesc")} confirmLabel={t("siDeleteUser")}
        onConfirm={async () => { if (deleting) await action(`/api/signin-users/${deleting.id}`, t("siUserDeleted"), true); }}>
        {deleting && <div className="rounded border border-line bg-surface-2 px-3 py-2 text-sm font-medium">{deleting.username}</div>}
      </ConfirmDialog>
      <ConfirmDialog open={!!deletingGroup} onOpenChange={(o) => !o && setDeletingGroup(null)} title={t("siDeleteGroup")} description={t("siDeleteGroupDesc")} confirmLabel={t("siDeleteGroup")}
        onConfirm={async () => { if (deletingGroup) await action(`/api/signin-groups/${deletingGroup.id}`, t("siGroupDeleted"), true); }}>
        {deletingGroup && <div className="rounded border border-line bg-surface-2 px-3 py-2 text-sm font-medium">{deletingGroup.name}</div>}
      </ConfirmDialog>
    </>
  );
}
