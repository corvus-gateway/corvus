import type { ProxyProtocolKey } from "../proxyprotocol.i18n";

export const proxyProtocolStrings: Record<ProxyProtocolKey, string> = {
  ppTitle: "Aceptar PROXY protocol",
  ppDesc: "Actívalo cuando un túnel TCP o un balanceador de carga (FRP, HAProxy) pase la IP del cliente en una cabecera PROXY protocol (v1/v2). Los puertos 80/443 no cambian; la cabecera se acepta en puertos aparte.",
  ppHttpPort: "Puerto HTTP",
  ppHttpsPort: "Puerto HTTPS",
  ppPortInvalid: "Introduce un puerto entre 1 y 65535.",
  ppTrusted: "Orígenes de PROXY protocol de confianza",
  ppTrustedHint: "Direcciones de los túneles o balanceadores que envían la cabecera. Solo las conexiones desde ellas usan la IP del cliente de la cabecera. Es una lista aparte de los proxies de confianza de arriba: no añadas allí el túnel.",
  ppHint: "Apunta el túnel a estos puertos y publícalos en docker-compose. Si el túnel está en el mismo host, publícalos solo en loopback (p. ej. '127.0.0.1:8443:8443'); si no, permite solo la dirección del túnel con un cortafuegos.",
};
