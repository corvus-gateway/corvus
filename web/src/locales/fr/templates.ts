import type { K } from "../templates.i18n";

export const templatesStrings: Record<K, string> = {
  fromTemplate: "Ajouter depuis un modèle",
  templatesTitle: "Modèles de correctifs virtuels",
  templatesDesc: "Règles qui bloquent des CVE connues en attendant le correctif. Appliquez-les d'abord en « Journal », vérifiez l'absence de faux positifs, puis passez en « Bloquer ».",
  tplSevCritical: "Critique", tplSevHigh: "Élevée",
  tplProducts: "Concerne", tplFalsePositives: "Faux positifs",
  tplExemptList: "Liste d'IP exemptées (réseau d'administration)", tplExemptNone: "Aucune exemption",
  tplExemptHint: "Ce modèle peut exempter votre réseau d'administration. Sinon, la règle s'applique à toutes les IP.",
  tplApply: "Créer la règle", tplCreated: "Règle créée depuis le modèle", tplSelect: "Choisissez un modèle",
  managedRules: "Règles gérées",
  managedRulesDesc: "Paquet de signatures d'inspection du corps et de modèles de correctifs virtuels. Seules les versions plus récentes et correctement signées sont appliquées.",
  mrVersion: "Version", mrSource: "Origine", mrBuiltin: "Intégrée", mrInstalled: "Mise à jour", mrSignatures: "Signatures", mrTemplates: "Modèles", mrChecked: "Dernière vérification",
  mrUrl: "URL de mise à jour", mrUrlHint: "La signature est récupérée à la même adresse suivie de .sig (https uniquement).",
  mrAuto: "Vérifier chaque jour", mrCheckNow: "Vérifier maintenant",
  mrUpload: "Téléverser", mrUploadHint: "Pour les installations hors ligne, téléversez bundle.json et bundle.json.sig.",
  mrBundleFile: "Paquet (bundle.json)", mrSigFile: "Signature (bundle.json.sig)",
  mrUploaded: "Paquet de règles appliqué", mrUpToDate: "Vérifié (une version plus récente est appliquée si disponible)", mrSaved: "Paramètres de mise à jour enregistrés",
};
