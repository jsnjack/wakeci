# Settings: Wakefile.yaml and the settings page

Read this before adding a setting, or changing the settings file or
`--import-settings`.

## Who owns what

Each setting has one owner; nothing is mirrored between the two.

- **Wakefile.yaml** holds what the server needs to start and what operations
  owns: port, hostname, directories, the secrets file, the time zone, the
  identity provider and `draintimeout`, which systemd's `TimeoutStopSec`
  must match. It is read at startup; a change needs a restart.
- **The settings page** (the database) holds what admins change while
  wakeci runs, recorded in the audit log: the password, concurrent builds,
  builds kept, the API key limits, the single sign-on groups and the rule
  for people in no group, and the API keys.

A new setting goes to the file if wakeci can't start or bind without it,
or if it must match something outside wakeci; otherwise to the page.

`sso.groups` is the one exception, seeded once (see access.md); the
settings file below is the way to keep groups in git.

## The Server panel

The settings page shows what Wakefile.yaml set, read-only
(`ServerConfigData` in `/api/settings`, `ServerConfigCard.vue`): the file's
path, port, hostname, directories, the secrets file with how many secrets
it holds (never their values), the time zone, the drain timeout, and the
single sign-on settings with the identity provider's signing certificates.
A certificate that expires within 30 days or has expired gets a warning,
since the metadata must be replaced before logins stop working. Anyone who
sees the settings sees it: they have `write`, so they can read the file on
the server anyway.

## The settings file

`GET /api/settings/export` writes the settings of the page as YAML
(`settings_file.go`): `concurrentBuilds`, `buildHistorySize`,
`apiKeyRate`, `apiKeyBurst`, `ssoGroups` and `ssoNoGroup` (`null` while it
is off). It never holds the password, API keys or sessions. Only the admin
password exports and imports, since the file has the groups.

Importing sets what the file has and leaves the rest, so a file with only
`ssoGroups` changes only the groups: `ssoGroups` replaces every group,
`ssoNoGroup: null` turns the rule off, and a missing key changes nothing.
The whole file is checked first, with the rules of the page (a grant goes
through `grantFromForm`), unknown keys refused, and saved in one
transaction, so a file with a mistake changes nothing; errors name the
setting, never a value. Importing groups also marks `sso.groups` as seeded,
so its groups aren't added to the imported ones later.

There are two ways in:

- **The page** (`POST /api/settings/import`, audited as `settings.import`),
  which applies it at once like the page's own changes.
- **`wakeci --import-settings file.yaml`** for a stopped server, also one
  that never ran: it opens the database (refusing after 2 seconds if a
  running wakeci holds it), creates what a first start would
  (`bootstrapDB`), imports, writes an audit line and exits. `bootstrapDB`
  writes each default only if it is missing, so the first start keeps the
  imported values.
