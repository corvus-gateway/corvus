import { StickyNote } from "lucide-react";
import { useI18n } from "../i18n";
import { FormSection, Textarea, fieldErr } from "./ui/form";

/** Longest note the API accepts (characters), see validate.MaxNotes. */
export const MAX_NOTES = 1000;

/** Form section with the free-text notes of a host or stream. */
export function NotesSection({ value, onChange, error, disabled }: { value: string; onChange: (v: string) => void; error: unknown; disabled?: boolean }) {
  const { t } = useI18n();
  const err = fieldErr(error, "notes");
  return (
    <FormSection title={t("secNotes")}>
      <div className="space-y-1.5">
        <Textarea aria-label={t("secNotes")} rows={3} maxLength={MAX_NOTES} readOnly={disabled} value={value} onChange={(e) => onChange(e.target.value)} placeholder={t("notesPlaceholder")} invalid={!!err} />
        {err ? <p role="alert" className="text-xs leading-4 text-danger">{err}</p>
          : <p className="text-xs leading-4 text-fg-subtle">{t("notesHint")} <span className="num">({[...value].length}/{MAX_NOTES})</span></p>}
      </div>
    </FormSection>
  );
}

/** First line of a note under a list row; the full note is in the tooltip. */
export function NoteLine({ notes }: { notes?: string }) {
  if (!notes) return null;
  const first = notes.split("\n")[0];
  return (
    <div className="mt-0.5 flex max-w-[18rem] items-center gap-1 text-xs text-fg-muted" title={notes}>
      <StickyNote className="h-3 w-3 shrink-0 text-fg-subtle" aria-hidden />
      <span className="truncate">{first}{notes.includes("\n") ? " …" : ""}</span>
    </div>
  );
}

/** Whether a search string (already lower-cased) matches the notes. */
export const notesMatch = (notes: string | undefined, s: string) => !!notes && notes.toLowerCase().includes(s);
