# Users & sign-in

## Roles

Add teammates under **Users**.

| Role | Can |
|---|---|
| **Administrator** | Everything, including users, SSO and system settings |
| **Operator** | Change proxy hosts, access queues, certificates and rules, and deploy |
| **Viewer** | Read only |

## Two-step verification

Every user can turn on two-step verification (TOTP) under **Account security**, with an
authenticator app such as Google Authenticator or 1Password. Save the recovery codes shown once
after setup. Administrators can **Reset MFA** for a user who lost their phone.

![Account security](../img/account.png){ .shot }

## Single sign-on (OIDC)

Under **SSO** administrators can let users sign in with an OpenID Connect provider: Okta,
Microsoft Entra ID, Google, Keycloak and others. You can limit sign-in to e-mail domains, map
provider groups to roles, and choose whether unknown users are created automatically.

## API tokens

Scripts and Prometheus use **API tokens** (created on the *API tokens* page) with an
`Authorization: Bearer cvs_…` header. A token has a role and an optional expiry, can't exceed its
creator's role, and is shown only once. See the [HTTP API](../reference/api.md).

## Protect the console

- Allow port 9443 only from trusted networks: with a firewall, or with **Admin console allowed
  networks** on the *Network* page.
- Locked out by that list? Start the container once with `CORVUS_ADMIN_ALLOW_ANY=1`, fix the list,
  and remove the variable again.
- To reach the console under a name or another IP, add them to its certificate with
  `CORVUS_ADMIN_HOSTS` (see [Configuration](../reference/configuration.md)).

## Audit log

**Audit log** records every administrative action: who changed what, from which IP, and when.
