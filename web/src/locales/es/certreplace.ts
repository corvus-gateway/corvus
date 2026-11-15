import type { CertReplaceKey } from "../certreplace.i18n";

export const certReplaceStrings: Record<CertReplaceKey, string> = {
  replaceCertAction: "Reemplazar",
  replaceCertTitle: "Reemplazar certificado",
  replaceCertDesc: "Pega el certificado renovado y la clave privada. El certificado conserva su ID y sus hosts asignados; los hosts que lo usan se vuelven a desplegar.",
  certReplaced: "Certificado reemplazado",
  renewFailed: "Error al renovar",
  renewFailedTip: "La renovación falló; se usa el certificado actual hasta que caduque. Error: {e}",
};
